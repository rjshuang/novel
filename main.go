package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rjshuang/novel/common"
	"github.com/rjshuang/novel/search"

	goepub "github.com/bmaupin/go-epub"
)

const (
	baseDir   = "download/"
	batchSize = 50
)

var (
	rules     []search.Handler
	outputDir string
)

// input 打印提示信息并读取字符串输入
func input(msg string) string {
	fmt.Print(msg, "\n>> ")
	var s string
	fmt.Scanln(&s)
	return s
}

// inputInt 打印提示信息并读取整数输入
func inputInt(msg string) int {
	fmt.Print(msg, "\n>> ")
	var i int
	fmt.Scanln(&i)
	return i
}

// output 打印输出信息（带空行分隔）
func output(msg string) {
	fmt.Println(msg)
	fmt.Println()
}

// initConf 从 rules.json 加载书源规则
func initConf() error {
	data, err := os.ReadFile("rules.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &rules)
}

func main() {
	// 命令行参数
	flag.StringVar(&outputDir, "o", baseDir, "输出目录")
	flag.Parse()

	// 加载配置
	if err := initConf(); err != nil {
		output("初始化配置失败: " + err.Error())
		return
	}

	// 选择书源
	output("书源ID\t网站")
	for i, h := range rules {
		output(fmt.Sprintf("%d\t%s", i, h.Name))
	}
	i := inputInt("选择书源")
	if i < 0 || i >= len(rules) {
		output("索引无效")
		return
	}
	handler := rules[i]

	// 搜索书籍
	keyword := input("输入书名或作者名")
	bookInfo, err := handler.SearchKeyword(keyword)
	if err != nil {
		output("查找失败: " + err.Error())
		return
	}
	if len(bookInfo) == 0 {
		output("未找到相关信息，请尝试更换关键词或书源")
		return
	}

	// 显示搜索结果
	output("索引\t书名\t作者\t更新时间\t最新章节")
	for i, book := range bookInfo {
		output(fmt.Sprintf("%d\t%s\t%s\t%s\t%s", i, book.Name, book.Author, book.UpdateTime, book.LastChapter))
	}

	// 选择书籍
	index := inputInt("输入书名对应索引")
	if index < 0 || index >= len(bookInfo) {
		output("索引无效")
		return
	}
	book := bookInfo[index]
	if err = handler.SearchChapterList(book); err != nil {
		output("获取章节列表失败: " + err.Error())
		return
	}
	if len(book.ChapterList) == 0 {
		output("章节列表为空，相关链接: " + book.Url)
		return
	}
	output(fmt.Sprintf("你选择了:《%s》, 共 %d 章", book.Name, len(book.ChapterList)))

	// 选择章节范围
	start := inputInt("输入开始章节序号（输入 -1 从头开始）")
	if start <= 0 {
		start = 0
	} else {
		start--
	}
	if start < len(book.ChapterList) {
		output("开始章节: " + book.ChapterList[start].Title)
	}

	end := inputInt("输入结束章节序号（输入 -1 到末尾）")
	if end <= 0 || end > len(book.ChapterList) {
		end = len(book.ChapterList)
	}
	if start >= end {
		output("输入无效：开始章节 >= 结束章节")
		return
	}
	output("结束章节: " + book.ChapterList[end-1].Title)

	// 下载章节
	chapters := book.ChapterList[start:end]
	ch := make(chan any, len(chapters))
	go printProgress(ch, chapters, len(chapters))
	failedChapters := download(handler, chapters, ch)

	time.Sleep(time.Second)
	fmt.Println()
	output(fmt.Sprintf("下载完成，失败章节数: %d", len(failedChapters)))

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		output("创建输出目录失败: " + err.Error())
		return
	}

	// 导出 TXT
	go func() {
		txtPath := filepath.Join(outputDir, book.Name+".txt")
		_ = os.Remove(txtPath)
		f, err := os.OpenFile(txtPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			output("创建 TXT 文件失败: " + err.Error())
			return
		}
		defer f.Close()

		for _, v := range chapters {
			_, _ = f.WriteString(v.Title + "\n")
			_, _ = f.WriteString(strings.Join(v.Content, "\n"))
			_, _ = f.WriteString("\n")
		}
		output("TXT 文件保存成功: " + txtPath)
	}()

	// 导出 EPUB
	e := goepub.NewEpub(book.Name)
	e.SetLang("zh-CN")
	e.SetAuthor(book.Author)
	if cssPath, err := e.AddCSS("static/cover.css", ""); err == nil {
		if coverPath, err := e.AddImage(book.ImageUrl, ""); err == nil {
			e.SetCover(coverPath, cssPath)
		}
	}
	e.SetDescription(book.Description)
	for _, v := range chapters {
		body := fmt.Sprintf("<h2>%s</h2>", v.Title)
		for _, text := range v.Content {
			body += fmt.Sprintf(`<p style="text-indent:2em">%s</p>`, text)
		}
		_, _ = e.AddSection(body, v.Title, "", "")
	}

	epubPath := filepath.Join(outputDir, book.Name+".epub")
	if err := e.Write(epubPath); err != nil {
		output("EPUB 文件保存失败: " + err.Error())
		return
	}
	output("EPUB 文件保存成功: " + epubPath)
}

// download 并发下载章节内容，每批 batchSize 章
func download(handler search.Handler, chapterList []*search.ChapterInfo, ch chan any) map[int]string {
	defer close(ch)

	var (
		wg             sync.WaitGroup
		mu             sync.Mutex
		failedChapters = make(map[int]string)
	)

	for i, chapters := range common.BatchSlice(chapterList, batchSize) {
		wg.Add(1)
		go func(batchIdx int, batch []*search.ChapterInfo) {
			defer wg.Done()
			for j, chapter := range batch {
				ch <- struct{}{}
				if err := handler.SearchContent(chapter); err != nil {
					globalIdx := batchSize*batchIdx + j
					mu.Lock()
					failedChapters[globalIdx] = err.Error()
					mu.Unlock()
					chapterList[globalIdx].Content = []string{err.Error()}
				}
			}
		}(i, chapters)
	}
	wg.Wait()
	return failedChapters
}

// printProgress 实时打印下载进度
func printProgress(ch chan any, chapters []*search.ChapterInfo, total int) {
	if total == 0 {
		return
	}
	var num int
	for {
		data, ok := <-ch
		if !ok {
			fmt.Printf("\r下载完成! 总章节: %d                 \n", total)
			return
		}
		if err, ok := data.(error); ok {
			fmt.Printf("\n下载失败: %s\n", err.Error())
			return
		}
		num++
		percent := float64(num) / float64(total) * 100
		chapterName := ""
		if num-1 < len(chapters) {
			chapterName = chapters[num-1].Title
			if len(chapterName) > 20 {
				chapterName = chapterName[:20] + "..."
			}
		}
		fmt.Printf("\r[%d/%d] %.1f%%  %s", num, total, percent, chapterName)
	}
}
