package main

import (
	"encoding/json"
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
	BASEDIR   = "download/"
	BATCHSIZE = 50
)

type Config struct {
	ProxyPool []string `json:"proxy_pool"`
	Rules     []search.Handler
}

var conf Config

func input(msg string) {
	fmt.Print(msg, "\n>>")
}
func output(msg string) {
	fmt.Println(msg)
	fmt.Println()
}

func initConf() error {
	data, err := os.ReadFile("conf.json")
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &conf)
	if err != nil {
		return err
	}
	defer func() {
		search.ProxyPool = common.DeDuplicate(conf.ProxyPool)
	}()
	temp := conf.ProxyPool
	conf.ProxyPool = []string{""}
	conf.ProxyPool = append(conf.ProxyPool, temp...)
	data, err = os.ReadFile("proxy.txt")
	if err != nil {
		return nil
	}

	for _, v := range common.BatchSlice(strings.Split(string(data), "\n"), 8) {
		if len(v) == 8 {
			protocol := strings.TrimSpace(v[0])
			ip := strings.TrimSpace(v[1])
			port := strings.TrimSpace(v[2])
			conf.ProxyPool = append(conf.ProxyPool, fmt.Sprintf("%s://%s:%s", protocol, ip, port))
		}
	}

	return nil
}

func main() {
	err := initConf()
	if err != nil {
		output("init conf failed:" + err.Error())
		return
	}

	output("书源id\t网址")
	for i, h := range conf.Rules {
		output(fmt.Sprintf("%d\t%s", i, h.EndPoint))
	}
	input("选择书源")
	var i int
	fmt.Scanln(&i)
	if i >= len(conf.Rules) || i < 0 {
		output("索引无效")
		return
	}
	handler := conf.Rules[i]

	// 查询书籍信息
	input("输入书名或作者名")
	var keyword string
	fmt.Scanln(&keyword)

	var bookInfo []*search.BookInfo
	bookInfo, err = handler.SearchKeyword(keyword)
	if err != nil {
		output("查找失败:" + err.Error())
		return
	}
	if len(bookInfo) == 0 {
		output("未找到相关信息")
		return
	}

	// 显示书籍信息
	output("索引\t书名\t作者\t更新时间\t最新章节")
	for i, book := range bookInfo {
		output(fmt.Sprintf("%d\t%s\t%s\t%s\t%s", i, book.Name, book.Author, book.UpdateTime, book.LastChapter))
	}
	// 获取章节列表
	input("输入书名对应索引")
	var index int
	fmt.Scanln(&index)
	if index >= len(bookInfo) || index < 0 {
		output("索引无效")
		return
	}
	book := bookInfo[index]
	if err = handler.SearchChapterList(book); err != nil {
		output("查找章节列表失败:" + err.Error())
		return
	}
	if len(book.ChapterList) == 0 {
		output("章节列表为空, 相关链接:" + book.Url)
		return
	}
	output(fmt.Sprintf("你选择了:《%s》, 共 %d 章", book.Name, len(book.ChapterList)))

	var start, end int
	input("输入开始章节序号")
	fmt.Scanln(&start)
	if start <= 0 {
		start = 0
	} else {
		start -= 1
	}
	output("开始章节标题: " + book.ChapterList[start].Title)

	input("输入结束章节序号")
	fmt.Scanln(&end)
	if end <= 0 || end > len(book.ChapterList) {
		end = len(book.ChapterList)
	}
	if start > end {
		output("输入无效")
		return
	}
	output("结束章节标题: " + book.ChapterList[end-1].Title)

	list := book.ChapterList[start:end]
	ch := make(chan any, len(list))
	go printProgress(ch, len(list))
	failedChapters := download(handler, list, ch)
	output(fmt.Sprintf("下载失败章节数:%d", len(failedChapters)))

	time.Sleep(time.Second)

	_, err = os.Stat(BASEDIR)
	if err != nil {
		err = os.MkdirAll(BASEDIR, 0755)
		if err != nil {
			output("create dir failed:" + err.Error())
			return
		}
	}

	go func() {
		txt_path := filepath.Join(BASEDIR, book.Name+".txt")
		os.RemoveAll(txt_path)
		f, err := os.OpenFile(txt_path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			output("touch txt file failed:" + err.Error())
			return
		}
		defer f.Close()
		for _, v := range list {
			f.WriteString(v.Title + "\n")
			f.WriteString(strings.Join(v.Content, "\n"))
			f.WriteString("\n")
		}
		output("save txt file success")
	}()

	e := goepub.NewEpub(book.Name)
	e.SetLang("zh-CN")
	e.SetAuthor(book.Author)
	cssPath, _ := e.AddCSS("static/cover.css", "")
	coverPath, _ := e.AddImage(book.ImageUrl, "")
	e.SetCover(coverPath, cssPath)
	e.SetDescription(book.Description)
	for _, v := range list {
		body := fmt.Sprintf("<h2>%s</h2>", v.Title)
		for _, text := range v.Content {
			body += fmt.Sprintf(`<p style="text-indent:2em">%s</p>`, text)
		}
		e.AddSection(body, v.Title, "", "")
	}
	epub_path := filepath.Join(BASEDIR, book.Name+".epub")
	err = e.Write(epub_path)
	if err != nil {
		output("save epub file failed:" + err.Error())
		return
	}
	output("save epub file success")
}

func download(handler search.Handler, chapterList []*search.ChapterInfo, ch chan (any)) map[int]string {
	defer func() {
		close(ch)
	}()
	batch_size := BATCHSIZE
	if handler.RateLimit {
		batch_size = len(chapterList)
	}
	var wg sync.WaitGroup
	failedChapters := make(map[int]string, 0)
	for i, chapters := range common.BatchSlice(chapterList, batch_size) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j, chapter := range chapters {
				ch <- struct{}{}
				err := handler.SearchContent(chapter)
				if err != nil {
					failedChapters[batch_size*i+j] = err.Error()
					chapterList[batch_size*i+j].Content = []string{err.Error()}
					if handler.RateLimit {
						output(err.Error())
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	return failedChapters
}

func printProgress(ch chan any, total int) {
	if total == 0 {
		return
	}
	var num int
	for {
		data, ok := <-ch
		if !ok {
			fmt.Printf("\rdownload finish\n")
			return
		} else {
			if err, ok := data.(error); ok {
				output("\ndownload failed:" + err.Error())
				return
			}
			num++
		}
		percent := float64(num) / float64(total) * 100
		fmt.Printf("\r%.1f%%", percent)
	}
}
