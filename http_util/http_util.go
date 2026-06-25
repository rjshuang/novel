package http_util

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const (
	RetryTimes   = 3
	Timeout3Sec  = 3
	Timeout10Sec = 10

	CheckURL = "https://www.baidu.com"

	// DefaultUA 默认 User-Agent，模拟 Chrome 浏览器
	DefaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	defaultClient     *http.Client
	defaultClientOnce sync.Once
)

// getDefaultClient 获取或创建默认 HTTP 客户端（单例复用）
func getDefaultClient() *http.Client {
	defaultClientOnce.Do(func() {
		defaultClient = &http.Client{
			Transport: &http.Transport{},
			Timeout:   time.Second * Timeout10Sec,
		}
	})
	return defaultClient
}

// Request 发送 HTTP 请求，自动添加默认 User-Agent
func Request(url, method string, header map[string]string, body io.Reader) (resp *http.Response, err error) {
	client := getDefaultClient()

	var req *http.Request
	switch method {
	case http.MethodGet:
		req, err = http.NewRequest("GET", url, nil)
	case http.MethodPost:
		req, err = http.NewRequest("POST", url, body)
	default:
		return nil, errors.New("不支持的请求方法: " + method)
	}
	if err != nil {
		return nil, err
	}

	// 设置默认 User-Agent
	req.Header.Set("User-Agent", DefaultUA)

	// 设置自定义请求头（可覆盖 User-Agent）
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return client.Do(req)
}

func GetClient(proxyUrl string, timeoutSec int) (*http.Client, error) {
	var transport *http.Transport
	if proxyUrl != "" {
		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			return nil, err
		}

		switch proxyURL.Scheme {
		case "socks5", "socks5h", "socks4":
			dialer, err := proxy.FromURL(proxyURL, nil)
			if err != nil {
				return nil, err
			}
			transport = &http.Transport{
				Dial: dialer.Dial,
			}
		case "http", "https":
			transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		default:
			return nil, errors.New("unsupported proxy type: " + proxyURL.Scheme)
		}
	} else {
		transport = &http.Transport{}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   time.Second * time.Duration(timeoutSec),
	}, nil
}

func CheckProxy(proxy string) (bool, error) {
	client, err := GetClient(proxy, Timeout3Sec)
	if err != nil {
		return false, err
	}
	resp, err := client.Get(CheckURL)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return false, errors.New(string(data))
	}
	return true, nil
}
