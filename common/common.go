package common

import (
	"strings"
)

func FormatUri(endpoint, uri string) string {
	if strings.HasPrefix(uri, "http") || uri == "" {
		return uri
	}
	if strings.HasPrefix(uri, "//") {
		return "https:" + uri
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return endpoint + uri
}

func BatchSlice[T any](data []T, batchSize int) [][]T {
	var result [][]T
	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}
		result = append(result, data[i:end])
	}
	return result
}

func DeDuplicate[T comparable](data []T) []T {
	var result []T
	m := make(map[T]struct{})
	for _, t := range data {
		if _, ok := m[t]; !ok {
			m[t] = struct{}{}
			result = append(result, t)
		}
	}
	return result
}

func Contains[T comparable](arr []T, ele T) bool {
	for _, e := range arr {
		if e == ele {
			return true
		}
	}
	return false
}

