package pcscommand

import (
	"fmt"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsupload"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// DefaultUploadMaxRetry 默认上传失败最大重试次数
	DefaultUploadMaxRetry = 3
)

type (
	// UploadOptions 上传可选项
	UploadOptions struct {
		Parallel        int
		MaxRetry        int
		Load            int
		NoRapidUpload   bool
		NoSplitFile     bool   // 禁用分片上传
		Policy          string // 同名文件处理策略
		NoFilenameCheck bool   // 禁用文件名合法性检查
		DisableWildcard bool   // 字面量目标目录模式（跳过通配符匹配，供 TUI 使用）
	}
)

func uploadPrintFormat(load int) string {
	if load <= 1 {
		return pcsupload.DefaultPrintFormat
	}
	return "[%s] ↑ %s/%s %s/s in %s ...\n"
}

func uploadFailureError(failed int) error {
	if failed == 0 {
		return nil
	}
	return fmt.Errorf("上传失败文件数: %d", failed)
}

func normalizeUploadOptions(opt *UploadOptions) *UploadOptions {
	if opt == nil {
		opt = &UploadOptions{}
	}
	if opt.Parallel <= 0 {
		opt.Parallel = pcsconfig.Config.MaxUploadParallel
	}
	if !opt.NoFilenameCheck {
		opt.NoFilenameCheck = pcsconfig.Config.IgnoreIllegal
	}
	if opt.MaxRetry < 0 {
		opt.MaxRetry = DefaultUploadMaxRetry
	}
	if opt.Load <= 0 {
		opt.Load = pcsconfig.Config.MaxUploadLoad
	}
	if opt.Policy != baidupcs.SkipPolicy && opt.Policy != baidupcs.OverWritePolicy && opt.Policy != baidupcs.RsyncPolicy {
		opt.Policy = pcsconfig.Config.UPolicy
	}
	return opt
}

// RunUpload 执行文件上传
func RunUpload(localPaths []string, savePath string, opt *UploadOptions) error {
	opt = normalizeUploadOptions(opt)

	if opt.DisableWildcard {
		savePath = path.Clean(GetActiveUser().PathJoin(savePath))
	} else {
		err := matchPathByShellPatternOnce(&savePath)
		if err != nil {
			return fmt.Errorf("上传文件, 获取网盘路径 %s 错误: %w", savePath, err)
		}
	}

	switch len(localPaths) {
	case 0:
		return fmt.Errorf("本地路径为空")
	}

	// 打开上传状态
	uploadDatabase, err := pcsupload.NewUploadingDatabase()
	if err != nil {
		return fmt.Errorf("打开上传未完成数据库错误: %w", err)
	}
	defer uploadDatabase.Close()

	var (
		pcs = GetBaiduPCS()
		// 使用 task framework
		executor = &taskframework.TaskExecutor{
			IsFailedDeque: true, // 失败统计
		}
		subSavePath string
		// 统计
		statistic = &pcsupload.UploadStatistic{}
	)

	pcsupload.ResetStopping()
	defer pcsupload.ResetStopping()

	// 优雅退出: 第一次 Ctrl+C 停止派发新任务并等待进行中的分片完成后保存进度, 第二次 Ctrl+C 强制退出
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	stopSigDone := make(chan struct{})
	defer func() {
		signal.Stop(sigChan)
		close(stopSigDone)
	}()
	go func() {
		select {
		case <-sigChan:
			fmt.Fprintln(os.Stderr, "\n收到退出信号, 等待进行中的分片完成 (再次 Ctrl+C 立即取消当前上传)...")
			executor.Stop()
			pcsupload.GracefulStopActiveUploaders()
		case <-stopSigDone:
			return
		}
		select {
		case <-sigChan:
			fmt.Fprintln(os.Stderr, "立即取消当前上传任务...")
			executor.Stop()
			pcsupload.CancelActiveUploaders()
		case <-stopSigDone:
			return
		}
	}()

	statistic.StartTimer() // 开始计时

	type pendingUpload struct {
		localFile   string
		subSavePath string
	}
	var pending []pendingUpload

	for k := range localPaths {
		expandedLocal := pcsutil.ExpandLocalPath(localPaths[k])
		walkedFiles, err := pcsutil.WalkDir(expandedLocal, "")
		if err != nil {
			return fmt.Errorf("遍历本地路径 %s 错误: %w", expandedLocal, err)
		}

		for k3 := range walkedFiles {
			var localPathDir string
			if os.PathSeparator == '\\' {
				walkedFiles[k3] = pcsutil.ConvertToUnixPathSeparator(walkedFiles[k3])
				localPathDir = pcsutil.ConvertToUnixPathSeparator(filepath.Dir(expandedLocal))
			} else {
				localPathDir = filepath.Dir(expandedLocal)
			}

			if localPathDir == "." {
				localPathDir = ""
			}
			subSavePath = strings.TrimPrefix(walkedFiles[k3], localPathDir)
			if !opt.NoFilenameCheck && !pcsutil.ChPathLegal(walkedFiles[k3]) {
				return fmt.Errorf("%s 文件路径含有非法字符", walkedFiles[k3])
			}
			pending = append(pending, pendingUpload{
				localFile:   walkedFiles[k3],
				subSavePath: subSavePath,
			})
		}
	}

	if len(pending) == 0 {
		return fmt.Errorf("未检测到上传的文件")
	}
	if len(pending) == 1 {
		opt.Load = 1
	}

	loadCount := len(pending)
	if loadCount > opt.Load {
		loadCount = opt.Load
	}

	dashboard := ui.NewTransferDashboard(os.Stdout, "并发上传", len(pending), loadCount > 1)
	printFmt := pcsupload.DefaultPrintFormat
	if !dashboard.Enabled() {
		printFmt = uploadPrintFormat(loadCount)
	}

	for _, item := range pending {
		executor.Append(&pcsupload.UploadTaskUnit{
			LocalFileChecksum: checksum.NewLocalFileChecksum(item.localFile, int(baidupcs.SliceMD5Size)),
			SavePath:          path.Clean(savePath + baidupcs.PathSeparator + item.subSavePath),
			PCS:               pcs.CopyPCS(),
			PCSClient:         pcsconfig.Config.PCSHTTPClient(),
			UploadingDatabase: uploadDatabase,
			Parallel:          opt.Parallel,
			MaxRate:           pcsconfig.Config.MaxUploadRate,
			PrintFormat:       printFmt,
			NoRapidUpload:     opt.NoRapidUpload,
			NoSplitFile:       opt.NoSplitFile,
			UploadStatistic:   statistic,
			Dashboard:         dashboard,
			Policy:            opt.Policy,
		}, opt.MaxRetry)
	}

	if !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "上传队列就绪: 共 %d 项 · 并发文件 %d · 分片线程 %d · 策略 %s", len(pending), loadCount, opt.Parallel, opt.Policy)
	}

	executor.SetParallel(loadCount)
	executor.Execute()
	dashboard.Close()

	stopped := executor.Stopped()
	failedList := executor.FailedDeque()
	failedCount := failedList.Size()

	if !stopped && failedCount == 0 {
		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "上传完成 · 耗时 %s · 总传输 %s", statistic.Elapsed()/1e6*1e6, converter.ConvertFileSize(statistic.TotalSize()))
		}
		return nil
	}

	if stopped && !ui.IsQuiet() {
		ui.PrintWarn(os.Stderr, "上传已中断 · 耗时 %s · 已传输 %s", statistic.Elapsed()/1e6*1e6, converter.ConvertFileSize(statistic.TotalSize()))
	}

	// 输出上传失败或未完成的文件列表
	if failedCount != 0 {
		if stopped {
			fmt.Fprintf(os.Stderr, "以下文件未完成上传: \n")
		} else {
			fmt.Fprintf(os.Stderr, "以下文件上传失败: \n")
		}
		tb := pcstable.NewTable(os.Stderr)
		for e := failedList.Shift(); e != nil; e = failedList.Shift() {
			item := e.(*taskframework.TaskInfoItem)
			tb.Append([]string{item.Info.Id(), item.Unit.(*pcsupload.UploadTaskUnit).LocalFileChecksum.Path})
		}
		tb.Render()
	}
	if err := uploadFailureError(failedCount); err != nil {
		return err
	}
	return fmt.Errorf("上传已中断")
}
