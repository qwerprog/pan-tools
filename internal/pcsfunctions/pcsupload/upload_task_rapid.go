package pcsupload

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
)

// rapidUpload 执行秒传握手阶段；若秒传未命中但获取到 uploadid，返回 isContinue=true 进入分片上传。
func (utu *UploadTaskUnit) rapidUpload() (isContinue bool, result *taskframework.TaskUnitRunResult) {
	utu.Step = StepUploadRapidUpload
	result = &taskframework.TaskUnitRunResult{}

	fdl, pcsError := utu.PCS.CacheFilesDirectoriesList(utu.panDir, baidupcs.DefaultOrderOptions)
	if pcsError != nil {
		switch pcsError.GetErrType() {
		case pcserror.ErrTypeRemoteError:
			switch pcsError.GetRemoteErrCode() {
			case 31066, -9:
			default:
				result.ResultMessage = "获取文件列表错误"
				result.Err = pcsError
				return
			}
		default:
			result.ResultMessage = "获取文件列表错误"
			result.NeedRetry = true
			result.Err = pcsError
			return
		}
	}

	if !utu.dashboardActive() && utu.LocalFileChecksum.Length >= 64*converter.MB {
		ui.Statusf("[%s] 正在计算文件校验和 (%s)...\n", utu.taskInfo.Id(), converter.ConvertFileSize(utu.LocalFileChecksum.Length, 1))
	}

	if utu.LocalFileChecksum.LocalFileMeta.MD5 == nil || utu.LocalFileChecksum.LocalFileMeta.SliceMD5 == nil {
		if err := utu.LocalFileChecksum.Sum(checksum.CHECKSUM_MD5 | checksum.CHECKSUM_SLICE_MD5); err != nil {
			result.ResultMessage = "计算文件秒传信息错误"
			result.Err = err
			return
		}
	}

	for _, fd := range fdl {
		if fd != nil && fd.Filename == utu.panFile {
			decodedMD5, _ := hex.DecodeString(fd.MD5)
			if utu.Policy == baidupcs.SkipPolicy || bytes.Equal(decodedMD5, utu.LocalFileChecksum.MD5) {
				utu.completeSkip("远端已存在, 跳过", fmt.Sprintf("%s 目标已存在, 跳过", utu.SavePath), result)
				return
			}
		}
	}

	uk, pcsError := utu.PCS.CacheUK()
	if pcsError != nil {
		result.ResultMessage = "获取用户uk错误, 请确保登录信息包含了STOKEN"
		result.Err = pcsError
		return
	}
	currentTime := time.Now().Unix()
	offset, err := createDataOffset(hex.EncodeToString(utu.LocalFileChecksum.MD5), uk, currentTime, utu.LocalFileChecksum.Length, DefaultContentSize)
	if err != nil {
		result.ResultMessage = "计算文件偏移量错误"
		result.Err = err
		return
	}
	dataContent, dataLength, err := utu.LocalFileChecksum.GetSliceDataContent(offset, DefaultContentSize)
	if err != nil {
		result.ResultMessage = "读取随机文件子片段错误"
		result.Err = err
		return
	}
	b64Content := strings.TrimRight(base64.StdEncoding.EncodeToString(dataContent), "=")

	blockSize := getBlockSize(utu.LocalFileChecksum.Length)
	if len(utu.LocalFileChecksum.LocalFileMeta.BlocksList) == 0 {
		if err = utu.LocalFileChecksum.CalculateChunkedSum(blockSize); err != nil {
			result.ResultMessage = "计算文件分块md5出错"
			result.Err = err
			return
		}
	}

	info, pcsError := utu.PCS.RapidUpload(
		utu.SavePath,
		utu.Policy,
		utu.state.Uploadid,
		hex.EncodeToString(utu.LocalFileChecksum.MD5),
		hex.EncodeToString(utu.LocalFileChecksum.SliceMD5),
		b64Content,
		fmt.Sprint(utu.LocalFileChecksum.CRC32),
		offset,
		dataLength,
		utu.LocalFileChecksum.Length,
		currentTime,
		utu.LocalFileChecksum.BlocksList,
	)
	if pcsError != nil {
		result.ResultMessage = StrUploadFailed
		result.Err = pcsError
		if pcsError.GetErrType() == pcserror.ErrTypeRemoteError {
			switch pcsError.GetRemoteErrCode() {
			case 31112:
				result.ResultMessage = "秒传失败, 超出配额, 网盘容量已满"
				result.NeedRetry = false
			case baidupcs.ErrCodeSkipExisting:
				if utu.Policy == baidupcs.SkipPolicy {
					utu.completeSkip("远端已存在, 跳过", fmt.Sprintf("%s 目标已存在, 跳过", utu.SavePath), result)
				}
				result.NeedRetry = false
			case baidupcs.ErrCodeRsyncSameSize:
				utu.completeSkip("目标大小未发生改变, 跳过", fmt.Sprintf("%s 目标大小未发生改变, 跳过", utu.SavePath), result)
			}
		}
		return
	}

	if info != nil && info.ReturnType == 2 {
		utu.UploadStatistic.AddTotalSize(utu.LocalFileChecksum.Length)
		doneLine := utu.formatDoneSummary("秒传完成")
		if utu.Dashboard != nil {
			utu.Dashboard.CompleteSlot(utu.taskInfo.Id(), doneLine)
		} else {
			ui.Statusln(doneLine)
		}
		result.Succeed = true
		return
	}

	if utu.state.Uploadid == "" && info != nil {
		utu.state.Uploadid = info.UploadID
	}
	utu.UploadingDatabase.UpdateUploading(&utu.LocalFileChecksum.LocalFileMeta, utu.state)
	utu.UploadingDatabase.Save()
	return true, result
}
