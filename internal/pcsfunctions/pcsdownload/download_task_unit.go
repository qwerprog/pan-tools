package pcsdownload

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/downloader"
)

type (
	// DownloadMode 下载模式
	DownloadMode int

	// DownloadTaskUnit 下载的任务单元
	DownloadTaskUnit struct {
		taskInfo *taskframework.TaskInfo // 任务信息

		Cfg                *downloader.Config
		PCS                *baidupcs.BaiduPCS
		PanClientFunc      func() *requester.HTTPClient
		PCSClientFunc      func() *requester.HTTPClient
		EnableHTTPS        *bool
		ParentTaskExecutor *taskframework.TaskExecutor

		DownloadStatistic *DownloadStatistic // 下载统计
		Dashboard         *ui.TransferDashboard

		// 可选项
		VerbosePrinter       *pcsverbose.PCSVerbose
		PrintFormat          string
		IsPrintStatus        bool // 是否输出各个下载线程的详细信息
		IsExecutedPermission bool // 下载成功后是否加上执行权限
		IsOverwrite          bool // 是否覆盖已存在的文件
		NoCheck              bool // 不校验文件
		DlinkPrefer          int  // 使用所有备选下载链接中的第几个链接
		ModifyMTime          bool // 下载的文件 mtime 修改为与网盘一致

		DownloadMode DownloadMode // 下载模式

		PcsPath  string // 要下载的网盘文件路径
		SavePath string // 保存的路径

		FileInfo *baidupcs.FileDirectory // 文件或目录详情
	}
)

const (
	// DefaultPrintFormat 默认的下载进度输出格式
	DefaultPrintFormat = "\r[%s] ↓ %s/%s %s/s in %s, left %s ............"
	// DownloadSuffix 文件下载后缀
	DownloadSuffix = ".pant-downloading"
	// StrDownloadInitError 初始化下载发生错误
	StrDownloadInitError = "初始化下载发生错误"
	// StrDownloadFailed 下载文件失败
	StrDownloadFailed = "下载文件失败"
	// StrDownloadGetDlinkFailed 获取下载链接失败
	StrDownloadGetDlinkFailed = "获取下载链接失败"
	// StrDownloadChecksumFailed 检测文件有效性失败
	StrDownloadChecksumFailed = "检测文件有效性失败"
	// StrDownloadCheckLengthFailed 检测文件大小一致性失败
	StrDownloadCheckLengthFailed = "检测文件大小一致性失败"
	// DefaultDownloadMaxRetry 默认下载失败最大重试次数
	DefaultDownloadMaxRetry = 3
)

const (
	DownloadModeLocate DownloadMode = iota
	DownloadModePCS
	DownloadModeStreaming
)

func (dtu *DownloadTaskUnit) SetTaskInfo(info *taskframework.TaskInfo) {
	dtu.taskInfo = info
}

func (dtu *DownloadTaskUnit) verboseInfof(format string, a ...interface{}) {
	if dtu.VerbosePrinter != nil {
		dtu.VerbosePrinter.Infof(format, a...)
	}
}

func (dtu *DownloadTaskUnit) dashboardActive() bool {
	return dtu.Dashboard != nil && dtu.Dashboard.Enabled()
}

func (dtu *DownloadTaskUnit) formatDoneSummary(note string) string {
	r := ui.NewRenderer(os.Stdout, nil)
	icon := "✓"
	if ui.ASCIIOnly() {
		icon = "[OK]"
	}
	okBadge := r.NewStyle().Foreground(ui.Palette.Success).Bold(true).Render(icon)
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	name := r.NewStyle().Foreground(ui.Palette.VaultCore).Render(filepath.Base(dtu.SavePath))
	sizeStr := converter.ConvertFileSize(dtu.FileInfo.Size, 2)
	return fmt.Sprintf("%s %s %s  %s",
		okBadge,
		muted.Render("["+dtu.taskInfo.Id()+"]"),
		name,
		muted.Render(sizeStr+" · "+note),
	)
}

func (dtu *DownloadTaskUnit) handleError(result *taskframework.TaskUnitRunResult) {
	switch value := result.Err.(type) {
	case pcserror.Error:
		switch value.GetErrType() {
		case pcserror.ErrTypeRemoteError:
			switch value.GetRemoteErrCode() {
			case 31045, 31066, 31297:
				result.NeedRetry = false
			default:
				result.NeedRetry = true
			}
		default:
			result.NeedRetry = true
		}
	case *os.PathError:
		result.NeedRetry = false
	default:
		result.NeedRetry = true
	}
}

// checkFileValid 检测下载落地文件的大小与校验和有效性
func (dtu *DownloadTaskUnit) checkFileValid(result *taskframework.TaskUnitRunResult) (ok bool) {
	if fi, err := os.Stat(dtu.SavePath); err == nil {
		if fi.Size() != dtu.FileInfo.Size {
			result.ResultMessage = StrDownloadCheckLengthFailed
			result.NeedNextDlinkIndex = true
			result.NeedRetry = true
			return false
		}
	}
	if dtu.Cfg.IsTest || dtu.NoCheck {
		return true
	}

	if !dtu.dashboardActive() && dtu.FileInfo.Size >= 128*converter.MB {
		ui.Statusf("[%s] 正在校验文件完整性...\n", dtu.taskInfo.Id())
	}

	err := CheckFileValid(dtu.SavePath, dtu.FileInfo)
	if err != nil {
		result.ResultMessage = StrDownloadChecksumFailed
		result.Err = err
		switch err {
		case ErrDownloadNotSupportChecksum:
			result.ResultMessage = "检验文件有效性"
			return true
		case ErrDownloadFileBanned:
			result.NeedRetry = false
			return false
		case ErrDownloadChecksumFailed:
			result.NeedRetry = true
			dtu.IsOverwrite = true
			return false
		default:
			result.NeedRetry = false
			return false
		}
	}
	return true
}

func (dtu *DownloadTaskUnit) OnRetry(lastRunResult *taskframework.TaskUnitRunResult) {
	var msg string
	if lastRunResult.Err == nil {
		msg = fmt.Sprintf("[%s] %s, 重试 %d/%d", dtu.taskInfo.Id(), lastRunResult.ResultMessage, dtu.taskInfo.Retry(), dtu.taskInfo.MaxRetry())
	} else {
		msg = fmt.Sprintf("[%s] %s, %s, 重试 %d/%d", dtu.taskInfo.Id(), lastRunResult.ResultMessage, lastRunResult.Err, dtu.taskInfo.Retry(), dtu.taskInfo.MaxRetry())
	}
	if dtu.Dashboard != nil {
		dtu.Dashboard.ClearSlot(dtu.taskInfo.Id(), msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func (dtu *DownloadTaskUnit) OnSuccess(lastRunResult *taskframework.TaskUnitRunResult) {
}

func (dtu *DownloadTaskUnit) OnFailed(lastRunResult *taskframework.TaskUnitRunResult) {
	var msg string
	if lastRunResult.Err == nil {
		msg = fmt.Sprintf("[%s] %s", dtu.taskInfo.Id(), lastRunResult.ResultMessage)
	} else {
		msg = fmt.Sprintf("[%s] %s, %s", dtu.taskInfo.Id(), lastRunResult.ResultMessage, lastRunResult.Err)
	}
	if dtu.Dashboard != nil {
		dtu.Dashboard.ClearSlot(dtu.taskInfo.Id(), msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func (dtu *DownloadTaskUnit) OnComplete(lastRunResult *taskframework.TaskUnitRunResult) {
}

func (dtu *DownloadTaskUnit) RetryWait() time.Duration {
	return pcsfunctions.RetryWait(dtu.taskInfo.Retry())
}

func (dtu *DownloadTaskUnit) Run() (result *taskframework.TaskUnitRunResult) {
	result = &taskframework.TaskUnitRunResult{}
	var err error
	if dtu.FileInfo == nil || dtu.taskInfo.Retry() > 0 {
		dtu.FileInfo, err = dtu.PCS.FilesDirectoriesMeta(dtu.PcsPath)
		if err != nil {
			result.ResultMessage = "获取下载路径信息错误"
			result.Err = err
			dtu.handleError(result)
			return
		}
	}

	// 仅在开启 --verbose 时展开 10 行元信息大表，避免日常批量下载刷屏
	if pcsverbose.IsVerbose && !dtu.dashboardActive() {
		ui.Statusf("\n[%s] ----\n%s\n", dtu.taskInfo.Id(), pcsfunctions.FormatFileDirectory(dtu.FileInfo))
	}

	if dtu.FileInfo.Isdir {
		if !dtu.Cfg.IsTest {
			os.MkdirAll(dtu.SavePath, 0777)
		}
		if dtu.Dashboard != nil {
			dtu.Dashboard.CompleteSlot(dtu.taskInfo.Id(), "")
		}
		result.Succeed = true
		return
	}

	if dtu.FileInfo.Size == 0 {
		if !dtu.Cfg.IsTest {
			if mkErr := os.MkdirAll(filepath.Dir(dtu.SavePath), 0777); mkErr != nil {
				result.ResultMessage = StrDownloadInitError
				result.Err = mkErr
				return
			}
			f, createErr := os.Create(dtu.SavePath)
			if createErr != nil {
				result.ResultMessage = StrDownloadInitError
				result.Err = createErr
				return
			}
			_ = f.Close()
		}
		summary := dtu.formatDoneSummary(dtu.SavePath)
		if dtu.Dashboard != nil {
			dtu.Dashboard.CompleteSlot(dtu.taskInfo.Id(), summary)
		} else {
			ui.Statusln(summary)
		}
		result.Succeed = true
		return
	}

	if !dtu.Cfg.IsTest && !dtu.IsOverwrite && FileExist(dtu.SavePath) {
		skipLine := dtu.formatDoneSummary("本地已存在, 跳过")
		if dtu.Dashboard != nil {
			dtu.Dashboard.CompleteSlot(dtu.taskInfo.Id(), skipLine)
		} else {
			ui.Statusln(skipLine)
		}
		result.Succeed = true
		return
	}

	var ok bool
	switch dtu.DownloadMode {
	case DownloadModeLocate:
		ok = dtu.locateDownload(result)
	case DownloadModePCS, DownloadModeStreaming:
		ok = dtu.pcsOrStreamingDownload(dtu.DownloadMode, result)
	}
	if !ok {
		return result
	}

	if !dtu.checkFileValid(result) {
		if result.NeedNextDlinkIndex {
			dtu.DlinkPrefer++
		}
		return result
	}

	if dtu.ModifyMTime {
		_ = os.Chtimes(dtu.SavePath, time.Unix(dtu.FileInfo.Mtime, 0), time.Unix(dtu.FileInfo.Mtime, 0))
	}
	dtu.DownloadStatistic.AddTotalSize(dtu.FileInfo.Size)

	note := dtu.SavePath
	if dtu.Cfg.IsTest {
		note = "测速完成"
	}
	doneLine := dtu.formatDoneSummary(note)
	if dtu.Dashboard != nil {
		dtu.Dashboard.CompleteSlot(dtu.taskInfo.Id(), doneLine)
	} else {
		ui.Statusln(doneLine)
	}
	result.Succeed = true
	return
}
