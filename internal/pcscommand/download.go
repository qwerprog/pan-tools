package pcscommand

import (
	"fmt"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsdownload"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/downloader"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type (
	//DownloadOptions 下载可选参数
	DownloadOptions struct {
		IsTest               bool
		IsPrintStatus        bool
		IsExecutedPermission bool
		IsOverwrite          bool
		DownloadMode         pcsdownload.DownloadMode
		SaveTo               string
		Parallel             int
		Load                 int
		MaxRetry             int
		NoCheck              bool
		ModifyMTime          bool
		FullPath             bool
		LinkPrefer           int
		DisableWildcard      bool // 字面量网盘路径模式（跳过通配符展开，供 TUI / 转存后直接下载使用）
	}

	// LocateDownloadOption 获取下载链接可选参数
	LocateDownloadOption struct {
		FromPan bool
	}
)

func downloadPrintFormat(load int) string {
	if load <= 1 {
		return pcsdownload.DefaultPrintFormat
	}
	return "[%s] ↓ %s/%s %s/s in %s, left %s ...\n"
}

// RunDownload 执行下载网盘内文件
func RunDownload(paths []string, options *DownloadOptions) error {
	if options == nil {
		options = &DownloadOptions{}
	}

	if options.Load <= 0 {
		options.Load = pcsconfig.Config.MaxDownloadLoad
	}

	if options.MaxRetry < 0 {
		options.MaxRetry = pcsdownload.DefaultDownloadMaxRetry
	}

	if !options.NoCheck {
		options.NoCheck = pcsconfig.Config.NoCheck
	}

	if runtime.GOOS == "windows" {
		// windows下不加执行权限
		options.IsExecutedPermission = false
	}

	// 设置下载配置
	cfg := &downloader.Config{
		Mode:                       transfer.RangeGenMode_BlockSize,
		CacheSize:                  pcsconfig.Config.CacheSize,
		BlockSize:                  baidupcs.InitRangeSize,
		MaxRate:                    pcsconfig.Config.MaxDownloadRate,
		InstanceStateStorageFormat: downloader.InstanceStateStorageFormatProto3,
		IsTest:                     options.IsTest,
		TryHTTP:                    !pcsconfig.Config.EnableHTTPS,
	}

	// 设置下载最大并发量
	if options.Parallel < 1 {
		options.Parallel = pcsconfig.Config.MaxParallel
	}

	if len(paths) == 0 {
		return fmt.Errorf("下载路径为空")
	}

	if options.DisableWildcard {
		acUser := GetActiveUser()
		resolved := make([]string, len(paths))
		for i, p := range paths {
			resolved[i] = path.Clean(acUser.PathJoin(p))
		}
		paths = resolved
	} else {
		var err error
		paths, err = matchPathByShellPattern(paths...)
		if err != nil {
			return err
		}
	}

	type scanFailure struct {
		pcsPath string
		err     error
	}

	var (
		pcs          = GetBaiduPCS()
		loadCount    = 0
		scanFailures []scanFailure
	)

	// 预测要下载的文件数量
	file_dir_list := make([]*baidupcs.FileDirectory, 0, 10)
	for k := range paths {
		pcs.FilesDirectoriesRecurseList(paths[k], baidupcs.DefaultOrderOptions, func(depth int, fdPath string, fd *baidupcs.FileDirectory, pcsError pcserror.Error) bool {
			if pcsError != nil {
				pcsCommandVerbose.Warnf("%s\n", pcsError)
				if fdPath == "" {
					fdPath = paths[k]
				}
				scanFailures = append(scanFailures, scanFailure{pcsPath: fdPath, err: pcsError})
				return true
			}
			file_dir_list = append(file_dir_list, fd)
			// 忽略统计文件夹数量
			if !fd.Isdir {
				loadCount++
				if loadCount >= options.Load {
					loadCount = options.Load
				}
			}
			return true
		})
	}

	if len(file_dir_list) == 0 {
		if len(scanFailures) == 1 {
			return fmt.Errorf("获取下载路径 %s 失败: %w", scanFailures[0].pcsPath, scanFailures[0].err)
		}
		if len(scanFailures) > 1 {
			msgs := make([]string, 0, len(scanFailures))
			for _, sf := range scanFailures {
				msgs = append(msgs, fmt.Sprintf("%s (%s)", sf.pcsPath, sf.err))
			}
			return fmt.Errorf("获取下载路径失败 (%d 项): %s: %w", len(scanFailures), strings.Join(msgs, "; "), scanFailures[0].err)
		}
		return fmt.Errorf("未找到可下载的文件或目录")
	}
	// 修改Load, 设置MaxParallel
	if loadCount > 0 {
		options.Load = loadCount
		// 取平均值
		cfg.MaxParallel = pcsconfig.AverageParallel(options.Parallel, loadCount)
	} else {
		cfg.MaxParallel = options.Parallel
	}

	var (
		executor = taskframework.TaskExecutor{
			IsFailedDeque: true, // 统计失败的列表
		}
		statistic = &pcsdownload.DownloadStatistic{}
	)

	// 处理队列, 小文件优先下载
	sort.Slice(file_dir_list, func(i, j int) bool {
		return file_dir_list[i].Size < file_dir_list[j].Size
	})

	dashboard := ui.NewTransferDashboard(os.Stdout, "并发下载", len(file_dir_list), options.Load > 1 && !options.IsPrintStatus)
	printFmt := pcsdownload.DefaultPrintFormat
	if !dashboard.Enabled() {
		printFmt = downloadPrintFormat(options.Load)
	}

	enableHTTPS := pcsconfig.Config.EnableHTTPS
	for _, v := range file_dir_list {
		newCfg := *cfg
		unit := pcsdownload.DownloadTaskUnit{
			Cfg:                  &newCfg,
			PCS:                  pcs,
			PanClientFunc:        pcsconfig.Config.PanHTTPClient,
			PCSClientFunc:        pcsconfig.Config.PCSHTTPClient,
			EnableHTTPS:          &enableHTTPS,
			VerbosePrinter:       pcsCommandVerbose,
			PrintFormat:          printFmt,
			ParentTaskExecutor:   &executor,
			DownloadStatistic:    statistic,
			Dashboard:            dashboard,
			IsPrintStatus:        options.IsPrintStatus,
			IsExecutedPermission: options.IsExecutedPermission,
			IsOverwrite:          options.IsOverwrite,
			NoCheck:              options.NoCheck,
			DlinkPrefer:          options.LinkPrefer,
			DownloadMode:         options.DownloadMode,
			ModifyMTime:          options.ModifyMTime,
			PcsPath:              v.Path,
			FileInfo:             v,
		}
		executor.SetParallel(loadCount)
		vPath := v.Path
		if !options.FullPath {
			vPath = filepath.Join(v.PreBase, filepath.Base(v.Path))
		}
		if options.SaveTo != "" {
			unit.SavePath = filepath.Join(options.SaveTo, vPath)
		} else {
			unit.SavePath = GetActiveUser().GetSavePath(vPath)
		}
		executor.Append(&unit, options.MaxRetry)
	}

	if !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "下载队列就绪: 共 %d 项 · 并发任务 %d · 单任务线程 %d", len(file_dir_list), options.Load, cfg.MaxParallel)
	}

	statistic.StartTimer()
	executor.Execute()
	dashboard.Close()

	// 输出失败的文件列表
	failedList := executor.FailedDeque()
	failedCount := failedList.Size()
	totalFailed := failedCount + len(scanFailures)

	if totalFailed == 0 && !executor.Stopped() {
		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "下载完成 · 耗时 %s · 总传输 %s", statistic.Elapsed()/1e6*1e6, converter.ConvertFileSize(statistic.TotalSize()))
		}
		return nil
	}

	if totalFailed != 0 {
		fmt.Fprintf(os.Stderr, "以下文件下载失败: \n")
		tb := pcstable.NewTable(os.Stderr)
		for idx, sf := range scanFailures {
			tb.Append([]string{fmt.Sprintf("scan-%d", idx+1), fmt.Sprintf("%s (%s)", sf.pcsPath, sf.err)})
		}
		for e := failedList.Shift(); e != nil; e = failedList.Shift() {
			item := e.(*taskframework.TaskInfoItem)
			tb.Append([]string{item.Info.Id(), item.Unit.(*pcsdownload.DownloadTaskUnit).PcsPath})
		}
		tb.Render()
		if len(scanFailures) > 0 && failedCount == 0 {
			return fmt.Errorf("下载失败文件数: %d (%w)", totalFailed, scanFailures[0].err)
		}
		return fmt.Errorf("下载失败文件数: %d", totalFailed)
	}
	if executor.Stopped() {
		return fmt.Errorf("下载已中断")
	}
	return nil
}
