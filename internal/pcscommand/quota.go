package pcscommand

import (
	"fmt"
	"os"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

type quotaJSON struct {
	Username    string  `json:"username"`
	QuotaBytes  int64   `json:"quota_bytes"`
	UsedBytes   int64   `json:"used_bytes"`
	FreeBytes   int64   `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// RunGetQuota 执行 获取当前用户空间配额信息, 并输出
func RunGetQuota() error {
	return RunGetQuotaWithOutput(ui.OutputPretty)
}

// RunGetQuotaWithOutput 获取当前用户空间配额信息并按指定输出模式打印。
func RunGetQuotaWithOutput(mode ui.OutputMode) error {
	quota, used, err := GetBaiduPCS().QuotaInfo()
	if err != nil {
		return err
	}

	var ratio float64
	if quota > 0 {
		ratio = float64(used) / float64(quota)
	}
	userName := GetActiveUser().Name

	switch mode {
	case ui.OutputJSON:
		return ui.WriteJSON(os.Stdout, quotaJSON{
			Username:    userName,
			QuotaBytes:  quota,
			UsedBytes:   used,
			FreeBytes:   quota - used,
			UsedPercent: 100 * ratio,
		})
	case ui.OutputPlain:
		return ui.WriteTSV(os.Stdout, [][]string{{
			userName,
			strconv.FormatInt(quota, 10),
			strconv.FormatInt(used, 10),
			fmt.Sprintf("%.2f%%", 100*ratio),
		}})
	default:
		bar := ui.RenderQuotaBar(os.Stdout, ratio, 24, ui.ColorEnabled(os.Stdout), ui.ASCIIOnly())
		ui.PrintInfo(os.Stdout, "用户: %s  已用: %s / %s  %s",
			userName,
			converter.ConvertFileSize(used, 2),
			converter.ConvertFileSize(quota, 2),
			bar,
		)
		return nil
	}
}
