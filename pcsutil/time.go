package pcsutil

import (
	"fmt"
	"time"
)

var (
	// CSTLocation 东八区时区
	CSTLocation = time.FixedZone("CST", 8*3600)
)

// BeijingTimeOption 根据给定的 get 返回东八区时间格式.
func BeijingTimeOption(get string) string {
	now := time.Now().In(CSTLocation)
	year, mon, day := now.Date()
	hour, min, sec := now.Clock()
	millisecond := now.Nanosecond() / 1e6
	switch get {
	case "Refer":
		return fmt.Sprintf("%d-%d-%d %02d:%02d:%02d.%03d", year, mon, day, hour, min, sec, millisecond)
	case "printLog":
		return fmt.Sprintf("%d-%d-%d_%02dh%02dm%02ds", year, mon, day, hour, min, sec)
	case "day":
		return fmt.Sprint(day)
	case "ymd":
		return fmt.Sprintf("%d-%d-%d", year, mon, day)
	case "hour":
		return fmt.Sprint(hour)
	default:
		return fmt.Sprint(time.Now().Unix())
	}
}

// FormatTime 将 Unix 时间戳, 转换为字符串
func FormatTime(t int64) string {
	tt := time.Unix(t, 0).In(CSTLocation)
	year, mon, day := tt.Date()
	hour, min, sec := tt.Clock()
	return fmt.Sprintf("%d-%02d-%02d %02d:%02d:%02d", year, mon, day, hour, min, sec)
}
