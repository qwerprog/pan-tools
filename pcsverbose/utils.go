package pcsverbose

import (
	"fmt"
	"time"
)

var cstZone = time.FixedZone("CST", 8*3600)

func TimePrefix() string {
	now := time.Now().In(cstZone)
	year, mon, day := now.Date()
	hour, min, sec := now.Clock()
	millisecond := now.Nanosecond() / 1e6
	return fmt.Sprintf("[%d-%d-%d %02d:%02d:%02d.%03d]", year, mon, day, hour, min, sec, millisecond)
}
