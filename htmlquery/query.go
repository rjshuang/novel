package htmlquery

import (
	"crypto/rand"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type xpathFilter struct {
	Name    string
	Index   int
	AttrMap map[string]string
}

type htmlNode struct {
	node *html.Node
}

func Parse(reader io.Reader) *htmlNode {
	root, err := html.Parse(reader)
	if err != nil {
		return nil
	}
	return &htmlNode{root}
}

func (h *htmlNode) Find(xpath string) []*htmlNode {
	ret := make([]*htmlNode, 0)
	if h == nil || h.node == nil || xpath == "" {
		return ret
	}

	node := h.node
	if strings.HasPrefix(xpath, "//") {
		pathSegments := SplitXpath(xpath)
		if len(pathSegments) == 0 {
			return ret
		}

		firstFilter := parseFilter(pathSegments[0])

		firstLevelNodes := findElementNodes(node, firstFilter)

		if len(pathSegments) == 1 {
			return wrapNode(firstLevelNodes)
		}

		var results []*html.Node
		for _, firstNode := range firstLevelNodes {
			childResults := findNodesByAbsolutePath(firstNode, pathSegments[1:])
			if len(childResults) > 0 {
				results = append(results, childResults...)
			}
		}
		return wrapNode(results)
	} else if strings.HasPrefix(xpath, "/") {
		path := strings.Split(strings.TrimLeft(xpath, "/"), "/")
		return wrapNode(findNodesByAbsolutePath(node, path))
	} else {
		return ret
	}
}

func (h *htmlNode) IsElement(name string) bool {
	if h == nil || h.node == nil {
		return false
	}

	n := h.node
	return n.Type == html.ElementNode && n.Data == name
}

func (h *htmlNode) FindAndGet(xpath string) string {
	nodeList := h.Find(xpath)
	if len(nodeList) == 0 {
		return ""
	}
	pathNode := SplitXpath(xpath)
	last := pathNode[len(pathNode)-1]
	if strings.Contains(last, "[") && strings.Contains(last, "]") {
		start, end := strings.Index(last, "["), strings.Index(last, "]")
		query := last[start+1 : end]
		for _, ele := range SplitFilter(query) {
			if strings.Contains(ele, "=") {
				continue
			}
			if strings.HasPrefix(query, "@") {
				ele = ele[1:]
				return nodeList[0].GetAttr(ele)
			}
		}
	}
	return nodeList[0].GetText()
}

func (h *htmlNode) GetText() string {
	if h == nil || h.node == nil {
		return ""
	}

	content := ""
	for n := h.node.FirstChild; n != nil; n = n.NextSibling {
		if n.Type == html.TextNode {
			if tmp := strings.TrimSpace(n.Data); tmp != "" {
				content = content + tmp + "\n"
			}
		}
		if n.Type == html.ElementNode && n.Data == "p" && n.FirstChild != nil {
			if tmp := strings.TrimSpace(n.FirstChild.Data); tmp != "" {
				content = content + tmp + "\n"
			}
		}
	}
	content = strings.TrimSuffix(content, "\n")

	return content
}

func (h *htmlNode) GetAttr(attrName string) string {
	if h == nil || h.node == nil {
		return ""
	}

	for _, attr := range h.node.Attr {
		if attr.Key == attrName {
			return attr.Val
		}
	}
	return ""
}

func (h *htmlNode) GetAttrs() map[string]string {
	ret := make(map[string]string)
	if h == nil || h.node == nil {
		return ret
	}

	for _, attr := range h.node.Attr {
		ret[attr.Key] = attr.Val
	}
	return ret
}

func findElementNodes(node *html.Node, filter xpathFilter) []*html.Node {
	results := findDirectChildren(node, filter)

	for n := node.FirstChild; n != nil; n = n.NextSibling {
		childResults := findElementNodes(n, filter)
		if len(childResults) > 0 {
			results = append(results, childResults...)
		}
	}

	return results
}

func parseFilter(filter string) xpathFilter {
	name := filter
	attrMap := make(map[string]string)
	index := -1
	start := strings.Index(filter, "[")
	end := strings.Index(filter, "]")
	if start > 0 && end > start {
		name = filter[:start]
		filterStr := filter[start+1 : end]
		for _, ele := range SplitFilter(filterStr) {
			if strings.HasPrefix(ele, "@") {
				s := ele[1:]
				arr := strings.Split(s, "=")
				var attrVal string
				if len(arr) > 1 {
					attrVal = strings.Trim(arr[1], "'")
				}
				attrMap[arr[0]] = attrVal
			}
			if i, err := strconv.Atoi(ele); err == nil {
				index = i
			}
		}
	}

	return xpathFilter{Name: name, AttrMap: attrMap, Index: index}
}

// findNodesByAbsolutePath 根据绝对路径查找节点
func findNodesByAbsolutePath(node *html.Node, path []string) []*html.Node {
	if len(path) == 0 || node == nil {
		return []*html.Node{}
	}

	filter := parseFilter(path[0])
	var results []*html.Node

	children := findDirectChildren(node, filter)

	if len(path) > 1 {
		nextPath := path[1:]
		for _, child := range children {
			childResults := findNodesByAbsolutePath(child, nextPath)
			if len(childResults) > 0 {
				results = append(results, childResults...)
			}
		}
	} else {
		results = children
	}

	return results
}

// findDirectChildren 查找直接子节点中匹配过滤条件的节点
func findDirectChildren(node *html.Node, filter xpathFilter) []*html.Node {
	results := make([]*html.Node, 0)

	if node == nil || filter.Name == "" {
		return results
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == filter.Name {

			cnt := 0
			if len(filter.AttrMap) > 0 {
				for k, v := range filter.AttrMap {
					for _, attr := range child.Attr {
						if attr.Key == k {
							if v == "" || attr.Val == v {
								cnt++
								break
							}
						}
					}
				}
			}
			if len(filter.AttrMap) == cnt {
				results = append(results, child)
			}
		}
	}

	if filter.Index >= 0 && filter.Index < len(results) {
		return []*html.Node{results[filter.Index]}
	}
	return results
}

func wrapNode(nodeList []*html.Node) []*htmlNode {
	results := make([]*htmlNode, 0, len(nodeList))
	for _, node := range nodeList {
		results = append(results, &htmlNode{node})
	}
	return results
}

func SplitFilter(filter string) []string {
	var ret []string
	filter = strings.TrimSpace(filter)
	for _, ele := range strings.Split(filter, "@") {
		ele = strings.TrimSpace(ele)
		if ele == "" {
			continue
		}
		if _, err := strconv.Atoi(ele); err == nil {
			ret = append(ret, ele)
			continue
		}
		i := strings.LastIndex(ele, "'")
		if i > 0 {
			s := strings.TrimSpace(ele[i+1:])
			ele = ele[:i+1]
			if _, err := strconv.Atoi(s); err == nil {
				ret = append(ret, s)
			}
		}
		ret = append(ret, "@"+ele)
	}
	return ret
}

func SplitXpath(xpath string) []string {
	if strings.HasPrefix(xpath, "//") {
		xpath = strings.TrimPrefix(xpath, `//`)
	}
	if strings.HasPrefix(xpath, "/") {
		xpath = strings.TrimPrefix(xpath, `/`)
	}

	temp := make(map[string]string)
	re := regexp.MustCompile(`\[[^\]]*\]`)
	for _, x := range re.FindAllString(xpath, -1) {
		k := genRandomString(10)
		temp[k] = x
		xpath = strings.ReplaceAll(xpath, x, k)
	}

	ret := strings.Split(xpath, "/")
	for i, str := range ret {
		if len(str) > 10 {
			k := str[len(str)-10:]
			if _, ok := temp[k]; ok {
				ret[i] = strings.ReplaceAll(str, k, temp[k])
			}
		}
	}
	return ret
}

// 生成指定长度的随机字符串
func genRandomString(length int) string {
	if length <= 0 {
		length = 32
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)

	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}

	return string(result)
}

