package pcsfunctions

import (
	"path"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// FormatFileDirectory 将单个网盘文件/目录元信息渲染为终端表格字符串。
func FormatFileDirectory(f *baidupcs.FileDirectory) string {
	if f == nil {
		return ""
	}
	builder := &strings.Builder{}
	tb := pcstable.NewTable(builder)
	tb.SetColumnAlignment([]int{pcstable.AlignLeft, pcstable.AlignLeft})

	if f.Isdir {
		tb.AppendBulk([][]string{
			{"类型", "目录"},
			{"目录路径", f.Path},
			{"目录名称", f.Filename},
		})
	} else {
		md5Label := "md5 (截图请打码)"
		if len(f.BlockList) > 1 {
			md5Label = "md5 (可能不正确)"
		}
		tb.AppendBulk([][]string{
			{"类型", "文件"},
			{"文件路径", f.Path},
			{"文件名称", f.Filename},
			{"文件大小", strconv.FormatInt(f.Size, 10) + ", " + converter.ConvertFileSize(f.Size)},
			{md5Label, f.MD5},
		})
	}

	tb.Append([]string{"app_id", strconv.FormatInt(f.AppID, 10)})
	tb.Append([]string{"fs_id", strconv.FormatInt(f.FsID, 10)})
	tb.AppendBulk([][]string{
		{"创建日期", pcsutil.FormatTime(f.Ctime)},
		{"修改日期", pcsutil.FormatTime(f.Mtime)},
	})

	if f.Ifhassubdir {
		tb.Append([]string{"是否含有子目录", "true"})
	}

	tb.Render()
	return builder.String()
}

// FormatCpMvList 将批量拷贝/移动列表渲染为终端表格字符串。
func FormatCpMvList(clj *baidupcs.CpMvListJSON) string {
	if clj == nil {
		return ""
	}
	builder := &strings.Builder{}
	tb := pcstable.NewTable(builder)
	tb.SetHeader([]string{"#", "原路径", "目标路径"})
	for k, item := range clj.List {
		if item == nil {
			continue
		}
		tb.Append([]string{strconv.Itoa(k), item.From, item.To})
	}
	tb.Render()
	return builder.String()
}

// FormatCloudDlTaskList 将离线下载任务列表渲染为终端表格字符串。
func FormatCloudDlTaskList(cl baidupcs.CloudDlTaskList) string {
	builder := &strings.Builder{}
	tb := pcstable.NewTable(builder)
	tb.SetHeader([]string{"#", "任务ID", "任务名称", "文件大小", "创建日期", "保存路径", "资源地址", "状态"})
	for k, v := range cl {
		if v == nil {
			continue
		}
		tb.Append([]string{
			strconv.Itoa(k),
			strconv.FormatInt(v.TaskID, 10),
			v.TaskName,
			converter.ConvertFileSize(v.FileSize),
			pcsutil.FormatTime(v.CreateTime),
			path.Clean(v.SavePath),
			v.SourceURL,
			v.StatusText,
		})
	}
	tb.Render()
	return builder.String()
}
