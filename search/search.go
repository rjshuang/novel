package search

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rjshuang/novel/common"
	"github.com/rjshuang/novel/htmlquery"
	"github.com/rjshuang/novel/http_util"

	"github.com/siongui/gojianfan"
)

// BookInfo 书籍信息
type BookInfo struct {
	Name        string
	Author      string
	LastChapter string
	UpdateTime  string
	Url         string
	ImageUrl    string
	Description string
	ChapterList []*ChapterInfo
}

// ChapterInfo 章节信息
type ChapterInfo struct {
	Title   string
	Url     string
	Content []string
}

// Handler 书源处理器，对应 rules.json 中的一条书源规则
type Handler struct {
	Name     string `json:"name"`
	EndPoint string `json:"endpoint"`
	Search   struct {
		Uri    string            `json:"uri"`
		Method string            `json:"method"`
		Params map[string]string `json:"params"`
		Header map[string]string `json:"header"`
	} `json:"search"`
	Book struct {
		Path   string `json:"path"`
		Detail struct {
			Name        string `json:"name"`
			Uri         string `json:"uri"`
			ImgUri      string `json:"img_uri"`
			Author      string `json:"author"`
			LastChapter string `json:"last_chapter"`
			UpdateTime  string `json:"last_update"`
			Description string `json:"description"`
		} `json:"detail"`
	} `json:"book"`
	Chapter struct {
		Path  string `json:"path"`
		Title string `json:"title"`
		Page  struct {
			Path    string `json:"path"`
			Pattern string `json:"pattern"`
		} `json:"page"`
	} `json:"chapter"`
	Content struct {
		Path string `json:"path"`
		Page struct {
			Path    string `json:"path"`
			Pattern string `json:"pattern"`
		} `json:"page"`
	} `json:"content"`
}

// SearchKeyword 根据关键词搜索书籍
func (h *Handler) SearchKeyword(keyword string) ([]*BookInfo, error) {
	params := url.Values{}
	for k, v := range h.Search.Params {
		if v == "?" {
			params.Set(k, keyword)
		} else {
			params.Set(k, v)
		}
	}

	var resp *http.Response
	var err error
	if h.Search.Method == "GET" {
		reqUrl := h.EndPoint + h.Search.Uri
		if len(params) > 0 {
			reqUrl += "?" + params.Encode()
		}
		resp, err = http_util.Request(reqUrl, "GET", h.Search.Header, nil)
	} else {
		body := strings.NewReader(params.Encode())
		resp, err = http_util.Request(h.EndPoint+h.Search.Uri, "POST", h.Search.Header, body)
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求返回状态码 %d", resp.StatusCode)
	}

	doc := htmlquery.Parse(resp.Body)
	if doc == nil {
		return nil, fmt.Errorf("解析 HTML 失败")
	}

	bookNodes := doc.Find(h.Book.Path)
	bookInfo := make([]*BookInfo, 0, len(bookNodes))
	for _, node := range bookNodes {
		book := &BookInfo{
			Name:        node.FindAndGet(h.Book.Detail.Name),
			Url:         common.FormatUri(h.EndPoint, node.FindAndGet(h.Book.Detail.Uri)),
			Author:      node.FindAndGet(h.Book.Detail.Author),
			LastChapter: node.FindAndGet(h.Book.Detail.LastChapter),
			UpdateTime:  node.FindAndGet(h.Book.Detail.UpdateTime),
			Description: node.FindAndGet(h.Book.Detail.Description),
			ImageUrl:    common.FormatUri(h.EndPoint, node.FindAndGet(h.Book.Detail.ImgUri)),
		}
		if book.Name == "" || book.Url == "" {
			continue
		}
		bookInfo = append(bookInfo, book)
	}
	return bookInfo, nil
}

// SearchChapterList 获取书籍的章节列表（支持分页）
func (h *Handler) SearchChapterList(book *BookInfo) error {
	pageNum := 1
	reqUrl := book.Url

	for i := 1; i <= pageNum && reqUrl != ""; i++ {
		resp, err := http_util.Request(reqUrl, "GET", nil, nil)
		if err != nil {
			return fmt.Errorf("请求章节列表失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("章节列表请求返回状态码 %d", resp.StatusCode)
		}

		doc := htmlquery.Parse(resp.Body)
		resp.Body.Close()

		if doc == nil {
			return fmt.Errorf("解析章节列表 HTML 失败: %s", reqUrl)
		}

		// 尝试从页面元信息获取封面和描述
		if book.ImageUrl == "" {
			if imgList := doc.Find("//meta[@property='og:image']"); len(imgList) > 0 {
				book.ImageUrl = common.FormatUri(h.EndPoint, imgList[0].GetAttr("content"))
			}
		}
		if book.Description == "" {
			if descList := doc.Find("//meta[@property='og:description']"); len(descList) > 0 {
				book.Description = descList[0].GetAttr("content")
			}
		}

		// 解析章节列表
		for _, a := range doc.Find(h.Chapter.Path) {
			c := &ChapterInfo{
				Title: a.GetText(),
				Url:   common.FormatUri(h.EndPoint, a.GetAttr("href")),
			}
			if c.Title == "" && h.Chapter.Title != "" {
				c.Title = a.FindAndGet(h.Chapter.Title)
			}
			book.ChapterList = append(book.ChapterList, c)
		}

		// 处理分页
		reqUrl = ""
		pageNodes := doc.Find(h.Chapter.Page.Path)
		for _, node := range pageNodes {
			if href := node.GetAttr("href"); href != "" {
				if node.GetText() == h.Chapter.Page.Pattern {
					reqUrl = common.FormatUri(h.EndPoint, href)
				} else if !strings.Contains(book.Url, href) {
					reqUrl = h.EndPoint + h.Chapter.Page.Pattern + href
				}
				pageNum++
				break
			} else {
				text := node.GetText()
				if strings.Contains(text, "/") {
					parts := strings.Split(text, "/")
					pageNum, err = strconv.Atoi(parts[1])
					if err == nil {
						reqUrl = book.Url + fmt.Sprintf(h.Chapter.Page.Pattern, i+1)
						break
					}
				}
			}
		}
	}
	return nil
}

// SearchContent 下载章节正文内容（支持翻页）
func (h *Handler) SearchContent(chapter *ChapterInfo) error {
	pre := strings.TrimSuffix(chapter.Url, ".html")
	reqUrl := chapter.Url

	for reqUrl != "" {
		resp, err := http_util.Request(reqUrl, "GET", nil, nil)
		if err != nil {
			return fmt.Errorf("请求章节内容失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("章节内容请求返回状态码 %d", resp.StatusCode)
		}

		doc := htmlquery.Parse(resp.Body)
		resp.Body.Close()

		if doc == nil {
			return fmt.Errorf("解析章节内容 HTML 失败: %s", reqUrl)
		}

		// 提取正文，繁体转简体
		content := doc.FindAndGet(h.Content.Path)
		content = gojianfan.T2S(content)
		content = strings.TrimLeft(content, chapter.Title)
		content = strings.Trim(content, "\n")
		chapter.Content = append(chapter.Content, strings.Split(content, "\n")...)

		// 检查是否有下一页
		reqUrl = ""
		if aList := doc.Find(h.Content.Page.Path); len(aList) > 0 {
			if href := aList[0].GetAttr("href"); href != "" &&
				aList[0].GetText() == h.Content.Page.Pattern &&
				strings.HasPrefix(common.FormatUri(h.EndPoint, href), pre) {
				reqUrl = common.FormatUri(h.EndPoint, href)
			}
		}
	}
	return nil
}
