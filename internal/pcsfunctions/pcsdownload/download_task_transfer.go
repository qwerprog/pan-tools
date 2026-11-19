package pcsdownload

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/downloader"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

func removeEmptyDownloadFile(path string) {
	info, statErr := os.Stat(path)
	if statErr == nil && info.Size() == 0 {
		_ = os.Remove(path)
	}
}

func (dtu *DownloadTaskUnit) isHTTPSEnabled() bool {
	if dtu.EnableHTTPS != nil {
		return *dtu.EnableHTTPS
	}
	return pcsconfig.Config.EnableHTTPS
}

func (dtu *DownloadTaskUnit) newPanHTTPClient() *requester.HTTPClient {
	var client *requester.HTTPClient
	if dtu.PanClientFunc != nil {
		client = dtu.PanClientFunc()
	} else {
		client = pcsconfig.Config.PanHTTPClient()
	}
	enableHTTPS := dtu.isHTTPSEnabled()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !enableHTTPS {
			req.Header.Del("Referer")
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	client.SetTimeout(6 * time.Minute)
	client.SetKeepAlive(true)
	return client
}

func (dtu *DownloadTaskUnit) newPCSHTTPClient() *requester.HTTPClient {
	if dtu.PCSClientFunc != nil {
		return dtu.PCSClientFunc()
	}
	return pcsconfig.Config.PCSHTTPClient()
}

// download 执行底层分片下载与进度输出
func (dtu *DownloadTaskUnit) download(downloadURL string, client *requester.HTTPClient) (err error) {
	var (
		writer     downloader.Writer
		file       *os.File
		fileClosed bool
	)

	if !dtu.Cfg.IsTest {
		defer func() {
			if r := recover(); r != nil {
				removeEmptyDownloadFile(dtu.SavePath)
				err = fmt.Errorf("%s: panic: %v", StrDownloadFailed, r)
			}
		}()
		dtu.Cfg.InstanceStatePath = dtu.SavePath + DownloadSuffix

		dir := filepath.Dir(dtu.SavePath)
		fileInfo, statErr := os.Stat(dir)
		if statErr != nil {
			if mkErr := os.MkdirAll(dir, 0777); mkErr != nil {
				return mkErr
			}
		} else if !fileInfo.IsDir() {
			return fmt.Errorf("%s, path %s: not a directory", StrDownloadInitError, dir)
		}

		writer, file, err = downloader.NewDownloaderWriterByFilename(dtu.SavePath, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("%s, %s", StrDownloadInitError, err)
		}
		defer func() {
			if !fileClosed && file != nil {
				_ = file.Close()
			}
		}()
	}

	der := downloader.NewDownloader(downloadURL, writer, dtu.Cfg)
	der.SetClient(client)
	der.SetDURLCheckFunc(BaiduPCSURLCheckFunc)
	der.SetStatusCodeBodyCheckFunc(func(respBody io.Reader) error {
		return pcserror.DecodePCSJSONError(baidupcs.OperationDownloadFile, respBody)
	})

	if dtu.PrintFormat == "" {
		dtu.PrintFormat = DefaultPrintFormat
	}

	fileBaseName := filepath.Base(dtu.SavePath)
	if fileBaseName == "" || fileBaseName == "." {
		fileBaseName = filepath.Base(dtu.PcsPath)
	}

	var isComplete atomic.Bool
	der.OnDownloadStatusEvent(func(status transfer.DownloadStatuser, workersCallback func(downloader.RangeWorkerFunc)) {
		if ui.IsQuiet() || isComplete.Load() {
			return
		}

		leftStr := "-"
		if left := status.TimeLeft(); left >= 0 {
			leftStr = left.Truncate(time.Second).String()
		}

		arrow := "↓"
		if ui.ASCIIOnly() {
			arrow = "v"
		}

		if dtu.dashboardActive() && !dtu.IsPrintStatus {
			dtu.Dashboard.UpdateSlot(ui.TransferSlot{
				ID:         dtu.taskInfo.Id(),
				Name:       fileBaseName,
				Arrow:      arrow,
				DoneBytes:  status.Downloaded(),
				TotalBytes: status.TotalSize(),
				SpeedBytes: status.SpeedsPerSecond(),
				Elapsed:    status.TimeElapsed(),
				ETA:        leftStr,
			})
			return
		}

		builder := &strings.Builder{}
		if dtu.IsPrintStatus {
			tb := pcstable.NewTable(builder)
			tb.SetHeader([]string{"#", "状态", "分片范围", "剩余", "速度/s", "错误"})
			workersCallback(func(key int, worker *downloader.Worker) bool {
				wrange := worker.GetRange()
				tb.Append([]string{
					fmt.Sprint(worker.ID()),
					worker.GetStatus().StatusText(),
					wrange.ShowDetails(),
					strconv.FormatInt(wrange.Len(), 10),
					converter.ConvertFileSize(worker.GetSpeedsPerSecond(), 1),
					fmt.Sprint(worker.Err()),
				})
				return true
			})
			builder.WriteString("\n")
			tb.Render()
		}

		if dtu.PrintFormat == DefaultPrintFormat {
			var ratio float64
			if total := status.TotalSize(); total > 0 {
				ratio = float64(status.Downloaded()) / float64(total)
			}
			builder.WriteString(ui.FormatTransferProgress(
				os.Stdout,
				dtu.taskInfo.Id(),
				arrow,
				ratio,
				converter.ConvertFileSize(status.Downloaded(), 2),
				converter.ConvertFileSize(status.TotalSize(), 2),
				converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
				(status.TimeElapsed() / 1e7 * 1e7).String(),
				leftStr,
				ui.ColorEnabled(os.Stdout),
				ui.ASCIIOnly(),
				ui.IsTTY(os.Stdout),
				fileBaseName,
			))
		} else {
			fmt.Fprintf(builder, dtu.PrintFormat, dtu.taskInfo.Id(),
				converter.ConvertFileSize(status.Downloaded(), 2),
				converter.ConvertFileSize(status.TotalSize(), 2),
				converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
				status.TimeElapsed()/1e7*1e7, leftStr,
			)
		}

		if !isComplete.Load() {
			fmt.Print(builder.String())
		}
	})

	err = der.Execute()
	isComplete.Store(true)
	if !dtu.dashboardActive() && !ui.IsQuiet() && ui.IsTTY(os.Stdout) {
		fmt.Print("\r\x1b[2K")
	}

	if err != nil {
		if !dtu.Cfg.IsTest && file != nil {
			info, infoErr := file.Stat()
			fileClosed = true
			_ = file.Close()
			if infoErr == nil && info.Size() == 0 {
				dtu.verboseInfof("[%s] remove empty file: %s\n", dtu.taskInfo.Id(), dtu.SavePath)
				if removeErr := os.Remove(dtu.SavePath); removeErr != nil {
					dtu.verboseInfof("[%s] remove file error: %s\n", dtu.taskInfo.Id(), removeErr)
				}
			}
		}
		return err
	}

	if !dtu.Cfg.IsTest && dtu.IsExecutedPermission {
		if chmodErr := file.Chmod(0766); chmodErr != nil {
			ui.PrintWarn(os.Stderr, "[%s] 添加可执行权限失败: %s", dtu.taskInfo.Id(), chmodErr)
		}
	}
	return nil
}

func (dtu *DownloadTaskUnit) execPanDownload(dlink string, result *taskframework.TaskUnitRunResult, okPtr *bool) {
	dtu.verboseInfof("[%s] 获取到下载链接: %s\n", dtu.taskInfo.Id(), dlink)

	client := dtu.newPanHTTPClient()
	pcsClient := dtu.PCS
	if pcsClient == nil {
		pcsClient = pcsconfig.Config.ActiveUserBaiduPCS()
	}
	newCookieJar, _ := CloneJarWithDomain(pcsClient.GetClient().Jar, dlink)
	client.SetCookiejar(newCookieJar)
	if err := dtu.download(dlink, client); err != nil {
		result.ResultMessage = StrDownloadFailed
		result.Err = err
		dtu.handleError(result)
		return
	}
	*okPtr = true
}

func (dtu *DownloadTaskUnit) locateDownload(result *taskframework.TaskUnitRunResult) (ok bool) {
	rawDlinks, err := GetLocateDownloadLinks(dtu.PCS, dtu.PcsPath)
	if err != nil {
		result.ResultMessage = StrDownloadGetDlinkFailed
		result.Err = err
		dtu.handleError(result)
		return false
	}

	if len(rawDlinks) < dtu.DlinkPrefer+1 {
		dtu.DlinkPrefer = len(rawDlinks) - 1
	}
	rawDlink := rawDlinks[dtu.DlinkPrefer]
	if strings.HasPrefix(rawDlink.Host, "nb.cache") && len(rawDlinks) > dtu.DlinkPrefer+1 {
		rawDlink = rawDlinks[dtu.DlinkPrefer+1]
	}
	FixHTTPLinkURL(rawDlink)
	dtu.execPanDownload(rawDlink.String(), result, &ok)
	return ok
}

func (dtu *DownloadTaskUnit) pcsOrStreamingDownload(mode DownloadMode, result *taskframework.TaskUnitRunResult) bool {
	dfunc := func(downloadURL string, jar http.CookieJar) error {
		client := dtu.newPCSHTTPClient()
		client.SetCookiejar(jar)
		client.SetKeepAlive(true)
		client.SetTimeout(10 * time.Minute)
		return dtu.download(downloadURL, client)
	}

	var err error
	switch mode {
	case DownloadModePCS:
		err = dtu.PCS.DownloadFile(dtu.PcsPath, dfunc)
	case DownloadModeStreaming:
		err = dtu.PCS.DownloadStreamFile(dtu.PcsPath, dfunc)
	default:
		result.ResultMessage = StrDownloadFailed
		result.Err = fmt.Errorf("unknown download mode: %d", mode)
		return false
	}

	if err != nil {
		result.ResultMessage = StrDownloadFailed
		result.Err = err
		dtu.handleError(result)
		return false
	}
	return true
}
