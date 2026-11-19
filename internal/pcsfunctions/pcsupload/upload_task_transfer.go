package pcsupload

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

// upload 执行多线程分片上传与合并
func (utu *UploadTaskUnit) upload() (result *taskframework.TaskUnitRunResult) {
	utu.Step = StepUploadUpload

	blockSize := getBlockSize(utu.LocalFileChecksum.Length)
	fileBaseName := filepath.Base(utu.LocalFileChecksum.Path)

	muer := uploader.NewMultiUploader(
		NewPCSUploadWithClient(utu.PCS, utu.SavePath, utu.PCSClient),
		rio.NewFileReaderAtLen64(utu.LocalFileChecksum.GetFile()),
		&uploader.MultiUploaderConfig{
			Parallel:  utu.Parallel,
			BlockSize: blockSize,
			MaxRate:   utu.MaxRate,
			Policy:    utu.Policy,
		},
		utu.SavePath,
	)

	if utu.state != nil {
		muer.SetInstanceState(utu.state)
	}

	RegisterActiveUploader(muer)
	defer UnregisterActiveUploader(muer)

	muer.OnUploadStatusEvent(func(status uploader.Status, updateChan <-chan struct{}) {
		select {
		case <-updateChan:
			if utu.state.Uploadid != "" {
				utu.UploadingDatabase.UpdateUploading(&utu.LocalFileChecksum.LocalFileMeta, muer.InstanceState())
				utu.UploadingDatabase.Save()
			}
		default:
		}

		if ui.IsQuiet() {
			return
		}

		arrow := "↑"
		if ui.ASCIIOnly() {
			arrow = "^"
		}

		if utu.dashboardActive() {
			utu.Dashboard.UpdateSlot(ui.TransferSlot{
				ID:         utu.taskInfo.Id(),
				Name:       fileBaseName,
				Arrow:      arrow,
				DoneBytes:  status.Uploaded(),
				TotalBytes: status.TotalSize(),
				SpeedBytes: status.SpeedsPerSecond(),
				Elapsed:    status.TimeElapsed(),
			})
			return
		}

		if utu.PrintFormat == "" || utu.PrintFormat == DefaultPrintFormat {
			var ratio float64
			if total := status.TotalSize(); total > 0 {
				ratio = float64(status.Uploaded()) / float64(total)
			}
			fmt.Print(ui.FormatTransferProgress(
				os.Stdout,
				utu.taskInfo.Id(),
				arrow,
				ratio,
				converter.ConvertFileSize(status.Uploaded(), 2),
				converter.ConvertFileSize(status.TotalSize(), 2),
				converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
				(status.TimeElapsed() / 1e7 * 1e7).String(),
				"",
				ui.ColorEnabled(os.Stdout),
				ui.ASCIIOnly(),
				ui.IsTTY(os.Stdout),
				fileBaseName,
			))
			return
		}

		fmt.Printf(utu.PrintFormat, utu.taskInfo.Id(),
			converter.ConvertFileSize(status.Uploaded(), 2),
			converter.ConvertFileSize(status.TotalSize(), 2),
			converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
			status.TimeElapsed(),
		)
	})

	result = &taskframework.TaskUnitRunResult{}
	muer.OnCancel(func() {
		if !utu.dashboardActive() && !ui.IsQuiet() && ui.IsTTY(os.Stdout) {
			fmt.Print("\r\x1b[2K")
		}
		if utu.state.Uploadid != "" {
			utu.UploadingDatabase.UpdateUploading(&utu.LocalFileChecksum.LocalFileMeta, muer.InstanceState())
			utu.UploadingDatabase.Save()
		}
		if utu.Dashboard != nil {
			utu.Dashboard.ClearSlot(utu.taskInfo.Id(), fmt.Sprintf("[%s] 上传已取消，断点已保存", utu.taskInfo.Id()))
		} else {
			ui.Statusf("[%s] 上传已取消，断点进度已保存\n", utu.taskInfo.Id())
		}
		result.ResultMessage = "用户取消上传"
	})
	muer.OnSuccess(func() {
		if !utu.dashboardActive() && !ui.IsQuiet() && ui.IsTTY(os.Stdout) {
			fmt.Print("\r\x1b[2K")
		}
		utu.UploadStatistic.AddTotalSize(utu.LocalFileChecksum.Length)
		utu.UploadingDatabase.Delete(&utu.LocalFileChecksum.LocalFileMeta)
		utu.UploadingDatabase.Save()
		doneLine := utu.formatDoneSummary("分片上传完成")
		if utu.Dashboard != nil {
			utu.Dashboard.CompleteSlot(utu.taskInfo.Id(), doneLine)
		} else {
			ui.Statusln(doneLine)
		}
		result.Succeed = true
	})
	muer.OnError(func(err error) {
		if !utu.dashboardActive() && !ui.IsQuiet() && ui.IsTTY(os.Stdout) {
			fmt.Print("\r\x1b[2K")
		}
		utu.classifyUploadError(err, result)
	})

	muer.Execute()
	return result
}

func (utu *UploadTaskUnit) classifyUploadError(err error, result *taskframework.TaskUnitRunResult) {
	pcsError, ok := err.(pcserror.Error)
	if !ok {
		result.ResultMessage = "上传文件错误"
		result.Err = err
		return
	}

	result.NeedRetry = true

	switch pcsError.GetErrType() {
	case pcserror.ErrTypeRemoteError:
		switch pcsError.GetRemoteErrCode() {
		case baidupcs.ErrCodeSkipExisting, 31061:
			result.ResultMessage = StrUploadFailed
			result.Err = pcsError
			if utu.Policy == baidupcs.SkipPolicy {
				utu.completeSkip("远端已存在, 跳过", fmt.Sprintf("%s 目标已存在, 跳过", utu.SavePath), result)
			}
			result.NeedRetry = false
		case baidupcs.ErrCodeRsyncSameSize:
			utu.completeSkip("目标大小未发生改变, 跳过", fmt.Sprintf("%s 目标大小未发生改变, 跳过", utu.SavePath), result)
		case 31363:
			utu.UploadingDatabase.Delete(&utu.LocalFileChecksum.LocalFileMeta)
			utu.UploadingDatabase.Save()
			result.ResultMessage = StrUploadFailed
			result.Err = errors.New("上传状态过期, 重新上传")
		default:
			result.ResultMessage = StrUploadFailed
			result.Err = pcsError
		}
	case pcserror.ErrTypeNetError:
		result.ResultMessage = StrUploadFailed
		result.Err = pcsError
		if pcsError.GetError() != nil && strings.Contains(pcsError.GetError().Error(), "413 Request Entity Too Large") {
			result.NeedRetry = false
		}
	default:
		result.ResultMessage = StrUploadFailed
		result.NeedRetry = false
		result.Err = pcsError
	}
}
