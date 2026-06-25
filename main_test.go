package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goepub "github.com/bmaupin/go-epub"
)

// TestEpub 测试 EPUB 生成功能（含音频嵌入）
// 使用方式: go test -run TestEpub -v
func TestEpub(t *testing.T) {
	outputDir = "test_output"
	_ = os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	bookName := "音乐簿"
	e := goepub.NewEpub(bookName)
	e.SetLang("zh-CN")
	e.SetAuthor("xxx")

	cssPath, err := e.AddCSS("static/cover.css", "")
	if err != nil {
		t.Fatalf("添加 CSS 失败: %v", err)
	}
	coverPath, err := e.AddImage("static/music.png", "")
	if err != nil {
		t.Fatalf("添加封面图片失败: %v", err)
	}
	e.SetCover(coverPath, cssPath)
	e.SetDescription("我的音乐簿")

	musicDir := `music`
	info, err := os.Stat(musicDir)
	if err != nil {
		t.Skipf("音乐目录不存在，跳过测试: %v", err)
	}
	if !info.IsDir() {
		t.Skipf("音乐路径不是目录，跳过测试")
	}

	ret := make(map[string]string)
	err = filepath.WalkDir(musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		parts := strings.Split(path, string(os.PathSeparator))
		title := parts[len(parts)-2]
		extIdx := strings.LastIndex(d.Name(), ".")
		if extIdx <= 0 {
			return nil
		}
		name := d.Name()[:extIdx]

		audioPath, err := e.AddAudio(path, d.Name())
		if err != nil {
			return fmt.Errorf("添加音频失败 %s: %w", d.Name(), err)
		}
		body := fmt.Sprintf("<h4>%s</h4>", name)
		body += fmt.Sprintf(`<audio src="%s" controls="controls"></audio>`, audioPath)
		body += "<br />"
		ret[title] += body
		return nil
	})
	if err != nil {
		t.Fatalf("遍历音乐目录失败: %v", err)
	}

	for title, body := range ret {
		_, err = e.AddSection(body, title, "", "")
		if err != nil {
			t.Fatalf("添加章节失败 %s: %v", title, err)
		}
	}

	epubPath := filepath.Join(outputDir, bookName+".epub")
	if err := e.Write(epubPath); err != nil {
		t.Fatalf("保存 EPUB 失败: %v", err)
	}
	t.Logf("EPUB 文件保存成功: %s", epubPath)
}
