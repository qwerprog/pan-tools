package downloader

import (
	mathrand "math/rand"
	"regexp"
	"strconv"
	"time"
)

var (
	// ContentRangeRE Content-Range 正则
	ContentRangeRE = regexp.MustCompile(`^.*? \d*?-\d*?/(\d*?)$`)

	// ranSource 随机数种子
	ranSource = mathrand.NewSource(time.Now().UnixNano())

	// ran 一个随机数实例
	ran = mathrand.New(ranSource)
)

// RandomNumber 生成指定区间随机数
func RandomNumber(min, max int) int {
	if min > max {
		min, max = max, min
	}
	return ran.Intn(max-min) + min
}

// ParseContentRange 解析Content-Range
func ParseContentRange(contentRange string) (contentLength int64) {
	raw := ContentRangeRE.FindStringSubmatch(contentRange)
	if len(raw) < 2 {
		return -1
	}

	c, err := strconv.ParseInt(raw[1], 10, 64)
	if err != nil {
		return -1
	}
	return c
}
