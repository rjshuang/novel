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

var (
	ProxyPool []string
)

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

type ChapterInfo struct {
	Title   string
	Url     string
	Content []string
}

type Handler struct {
	RateLimit bool   `json:"rate_limit"`
	EndPoint  string `json:"endpoint"`
	Search    struct {
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

func (h *Handler) SearchKeyword(keyword string) ([]*BookInfo, error) {
	var resp *http.Response
	var err error

	params := url.Values{}
	for k, v := range h.Search.Params {
		if v == "?" {
			params.Set(k, keyword)
		} else {
			params.Set(k, v)
		}
	}

	bookInfo := make([]*BookInfo, 0)
	if h.Search.Method == "GET" {
		s := h.EndPoint + h.Search.Uri
		if len(params) > 0 {
			s += fmt.Sprintf("?%s", params.Encode())
		}
		resp, err = http_util.Request(s, "GET", h.Search.Header, nil, !h.RateLimit, ProxyPool)
	} else {
		body := strings.NewReader(params.Encode())
		resp, err = http_util.Request(h.EndPoint+h.Search.Uri, "POST", h.Search.Header, body, !h.RateLimit, nil)
	}
	if err != nil {
		return bookInfo, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return bookInfo, fmt.Errorf("request not 200")
	}

	doc := htmlquery.Parse(resp.Body)
	bookNode := doc.Find(h.Book.Path)
	for _, node := range bookNode {
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

func (h *Handler) SearchChapterList(book *BookInfo) error {
	var resp *http.Response
	var err error
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	pageNum := 1
	reqUrl := book.Url
	for i := 1; i <= pageNum && reqUrl != ""; i++ {
		resp, err = http_util.Request(reqUrl, "GET", nil, nil, !h.RateLimit, ProxyPool)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("request not 200")
		}

		doc := htmlquery.Parse(resp.Body)
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

		for _, a := range doc.Find(h.Chapter.Path) {
			c := &ChapterInfo{Title: a.GetText(), Url: common.FormatUri(h.EndPoint, a.GetAttr("href"))}
			if c.Title == "" && h.Chapter.Title != "" {
				c.Title = a.FindAndGet(h.Chapter.Title)
			}
			book.ChapterList = append(book.ChapterList, c)
		}

		reqUrl = ""
		pageNode := doc.Find(h.Chapter.Page.Path)
		for _, node := range pageNode {
			if node.GetAttr("href") != "" {
				if node.GetText() == h.Chapter.Page.Pattern {
					reqUrl = common.FormatUri(h.EndPoint, node.GetAttr("href"))
					pageNum++
					break
				}
			} else {
				s := node.GetText()
				if strings.Contains(s, "/") {
					pageNum, err = strconv.Atoi(strings.Split(s, "/")[1])
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

func (h *Handler) SearchContent(chapter *ChapterInfo) error {
	var resp *http.Response
	var err error
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	pre := strings.TrimSuffix(chapter.Url, ".html")
	for reqUrl := chapter.Url; reqUrl != ""; {
		resp, err = http_util.Request(reqUrl, "GET", nil, nil, h.RateLimit, ProxyPool)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("request not 200")
		}

		doc := htmlquery.Parse(resp.Body)
		content := doc.FindAndGet(h.Content.Path)
		content = gojianfan.T2S(content)
		content = strings.Trim(content, "\n")
		chapter.Content = append(chapter.Content, strings.Split(content, "\n")...)

		reqUrl = ""
		if aList := doc.Find(h.Content.Page.Path); len(aList) > 0 {
			if href := aList[0].GetAttr("href"); href != "" && aList[0].GetText() == h.Content.Page.Pattern &&
				strings.HasPrefix(common.FormatUri(h.EndPoint, href), pre) {
				reqUrl = common.FormatUri(h.EndPoint, href)
			}
		}
	}
	return nil
}
