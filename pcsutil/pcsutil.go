// Package pcsutil 工具包
package pcsutil

import (
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// ErrorColor 保留无 ANSI 转义的纯文本错误格式化函数，交由上层统一着色渲染。
var ErrorColor = fmt.Sprint

// TrimPathPrefix 去除目录的前缀
func TrimPathPrefix(path, prefixPath string) string {
	if prefixPath == "/" {
		return path
	}
	return strings.TrimPrefix(path, prefixPath)
}

func GenerateRandomString(length int) string {
	var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[seededRand.Intn(len(charset))] // 生成随机索引并获取字符
	}
	return string(result)
}

// ContainsString 检测字符串是否在字符串数组里
func ContainsString(ss []string, s string) bool {
	for k := range ss {
		if ss[k] == s {
			return true
		}
	}
	return false
}

// GetURLCookieString 返回指定 URL 匹配的 cookie 字串
func GetURLCookieString(urlString string, jar *cookiejar.Jar) string {
	if jar == nil {
		return ""
	}
	u, err := url.Parse(urlString)
	if err != nil {
		return ""
	}
	cookies := jar.Cookies(u)
	parts := make([]string, 0, len(cookies))
	for _, v := range cookies {
		parts = append(parts, v.String())
	}
	return strings.Join(parts, "; ")
}

// DecompressGZIP 对 io.Reader 数据, 进行 gzip 解压
func DecompressGZIP(r io.Reader) ([]byte, error) {
	gzipReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	return io.ReadAll(gzipReader)
}

// Trigger 用于触发事件
func Trigger(f func()) {
	if f == nil {
		return
	}
	go f()
}

// TriggerOnSync 用于触发事件, 同步触发
func TriggerOnSync(f func()) {
	if f == nil {
		return
	}
	f()
}
