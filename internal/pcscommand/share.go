package pcscommand

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
)

// RunShareSet 执行分享
func RunShareSet(paths []string, option *baidupcs.ShareOption) error {
	if option == nil {
		option = &baidupcs.ShareOption{}
	}
	pcspaths, err := matchPathByShellPattern(paths...)
	if err != nil {
		return err
	}

	shared, err := GetBaiduPCS().ShareSet(pcspaths, option)
	if err != nil {
		return fmt.Errorf("%s失败: %w", baidupcs.OperationShareSet, err)
	}
	if option.IsCombined {
		if ui.IsQuiet() {
			fmt.Printf("%s?pwd=%s\n", shared.Link, shared.Pwd)
		} else {
			ui.PrintSuccess(os.Stdout, "创建分享成功 · ShareID: %d · 链接: %s?pwd=%s", shared.ShareID, shared.Link, shared.Pwd)
		}
	} else {
		if ui.IsQuiet() {
			fmt.Printf("shareID: %d, 链接: %s, 密码: %s\n", shared.ShareID, shared.Link, shared.Pwd)
		} else {
			ui.PrintSuccess(os.Stdout, "创建分享成功 · ShareID: %d · 链接: %s · 提取码: %s", shared.ShareID, shared.Link, shared.Pwd)
		}
	}
	return nil
}

// RunShareCancel 执行取消分享
func RunShareCancel(shareIDs []int64) error {
	if len(shareIDs) == 0 {
		return fmt.Errorf("%s失败, 没有任何 shareid", baidupcs.OperationShareCancel)
	}

	err := GetBaiduPCS().ShareCancel(shareIDs)
	if err != nil {
		return fmt.Errorf("%s失败: %w", baidupcs.OperationShareCancel, err)
	}

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "%s成功 (共 %d 项)", baidupcs.OperationShareCancel, len(shareIDs))
	}
	return nil
}

// RunShareList 执行列出分享列表
func RunShareList(page int) error {
	return RunShareListWithOutput(page, ui.OutputPretty)
}

// RunShareListWithOutput 列出分享列表并支持多种输出模式。
func RunShareListWithOutput(page int, mode ui.OutputMode) error {
	if page < 1 {
		page = 1
	}

	pcs := GetBaiduPCS()
	records, err := pcs.ShareList(page)
	if err != nil {
		return fmt.Errorf("%s失败: %w", baidupcs.OperationShareList, err)
	}

	for k, record := range records {
		if record.ExpireType == -1 {
			record.Valid = "已过期"
		} else if record.ExpireTime == 0 {
			record.Valid = "永久"
		} else {
			tm := time.Unix(time.Now().Unix()+record.ExpireTime, 0)
			record.Valid = tm.Format("2006/01/02 15:04:05")
		}
		if record.Public == 0 && record.ExpireType != -1 {
			info, pcsError := pcs.ShareSURLInfo(record.ShareID)
			if pcsError != nil {
				if mode == ui.OutputPretty {
					ui.PrintWarn(os.Stderr, "[%d] 获取分享提取码失败 (ShareID=%d): %s", k, record.ShareID, pcsError)
				}
			} else {
				record.Passwd = strings.TrimSpace(info.Pwd)
			}
		}
	}

	if mode == ui.OutputJSON {
		type shareJSON struct {
			ShareID   int64  `json:"share_id"`
			Shortlink string `json:"shortlink"`
			Passwd    string `json:"passwd,omitempty"`
			Path      string `json:"typical_path"`
			Valid     string `json:"valid"`
			ViewCount int    `json:"view_count"`
		}
		out := make([]shareJSON, 0, len(records))
		for _, r := range records {
			out = append(out, shareJSON{
				ShareID:   r.ShareID,
				Shortlink: r.Shortlink,
				Passwd:    r.Passwd,
				Path:      r.TypicalPath,
				Valid:     r.Valid,
				ViewCount: r.ViewCount,
			})
		}
		return ui.WriteJSON(os.Stdout, out)
	}

	if mode == ui.OutputPlain {
		rows := make([][]string, 0, len(records))
		for _, r := range records {
			rows = append(rows, []string{
				strconv.FormatInt(r.ShareID, 10),
				r.Shortlink,
				r.Passwd,
				r.TypicalPath,
				r.Valid,
				strconv.Itoa(r.ViewCount),
			})
		}
		return ui.WriteTSV(os.Stdout, rows)
	}

	tb := pcstable.NewTable(os.Stdout)
	tb.SetHeader([]string{"#", "ShareID", "分享链接", "提取密码", "特征目录", "特征路径", "过期时间", "浏览次数"})
	for k, record := range records {
		tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(record.ShareID, 10), record.Shortlink, record.Passwd, path.Clean(path.Dir(record.TypicalPath)), record.TypicalPath, record.Valid, strconv.Itoa(record.ViewCount)})
	}
	tb.Render()
	return nil
}
