package http_util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rjshuang/novel/common"

	"golang.org/x/net/proxy"
)

const (
	RetryTimes   = 3
	Timeout3Sec  = 3
	Timeout10Sec = 10

	CheckURL = "https://www.baidu.com"
)

var (
	unavailableProxy = make(map[string][]string)
)

func Request(url, method string, header map[string]string, body io.Reader, retry bool, proxyList []string) (resp *http.Response, err error) {
	if len(proxyList) == 0 {
		proxyList = []string{""}
	}
	var errList []map[string]string
	for _, proxy := range proxyList {
		if arr, ok := unavailableProxy[url]; ok && common.Contains(arr, proxy) {
			continue
		}
		client, err := GetClient(proxy, Timeout10Sec)
		if err != nil {
			return nil, err
		}
		var req *http.Request
		switch method {
		case http.MethodGet:
			req, err = http.NewRequest("GET", url, nil)
		case http.MethodPost:
			req, err = http.NewRequest("POST", url, body)
		default:
			return nil, errors.New("unsupported method: " + method)
		}
		if err != nil {
			return nil, err
		}
		for k, v := range header {
			req.Header.Set(k, v)
		}
		cnt := 1
		if retry {
			cnt = RetryTimes
		}
		for i := 0; i < cnt; i++ {
			if resp, err = client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
				return resp, err
			}
			if i < cnt-1 {
				time.Sleep(time.Second * Timeout3Sec)
			}
		}
		errMsg := map[string]string{"proxy_url": proxy}
		if err != nil {
			errMsg["err"] = err.Error()
		}
		if resp != nil {
			data, _ := io.ReadAll(resp.Body)
			errMsg["resp_data"] = string(data)
			errMsg["status_code"] = fmt.Sprint(resp.StatusCode)
		}
		errList = append(errList, errMsg)
		unavailableProxy[url] = append(unavailableProxy[url], proxy)
	}
	d, _ := json.Marshal(errList)
	return resp, errors.New(string(d))
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
