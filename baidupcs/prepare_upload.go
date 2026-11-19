package baidupcs

import (
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
)

func (pcs *BaiduPCS) prepareRapidUploadV2(targetPath, uploadid, policy, contentMD5, sliceMD5, dataContent, crc32 string, offset, length, totalSize, dataTime int64, blockListMD5 []string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcsURL := pcs.generatePanURL("precreate", nil)
	post := map[string]string{
		"uploadid":     uploadid,
		"path":         targetPath,
		"target_path":  path.Dir(targetPath) + "/",
		"size":         strconv.FormatInt(totalSize, 10),
		"data_offset":  strconv.FormatInt(offset, 10),
		"isdir":        "0",
		"local_mtime":  strconv.FormatInt(dataTime, 10),
		"local_ctime":  strconv.FormatInt(dataTime, 10),
		"rtype":        policy,
		"checkexist":   "0",
		"autoinit":     "1",
		"content-md5":  contentMD5,
		"slice-md5":    sliceMD5,
		"data_time":    strconv.FormatInt(dataTime, 10),
		"data_length":  strconv.FormatInt(length, 10),
		"data_content": dataContent,
		"block_list":   mergeStringList(blockListMD5...),
		"mode":         "1",
	}
	baiduPCSVerbose.Infof("%s URL: %s, Post: %v\n", OperationRapidUpload, pcsURL, post)

	if uploadid == "" {
		delete(post, "uploadid")
	}

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationRapidUpload, http.MethodPost, pcsURL.String(), post, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "*/*",
		"Connection":   "keep-alive",
	})
}

func (pcs *BaiduPCS) prepareFakeRapidUploadV2(targetPath, policy string, dateTime int64, blockListMD5 []string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcsURL := pcs.generatePanURL("precreate", map[string]string{
		"app_id":  PanAppID,
		"channel": "1",
		"web":     "1",
	})
	post := map[string]string{
		"path":        targetPath,
		"target_path": path.Dir(targetPath) + "/",
		"local_mtime": strconv.FormatInt(dateTime, 10),
		"autoinit":    "1",
		"rtype":       policy,
		"block_list":  mergeStringList(blockListMD5...),
	}
	baiduPCSVerbose.Infof("%s URL: %s, Post: %v\n", OperationRapidUpload, pcsURL, post)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationRapidUpload, http.MethodPost, pcsURL.String(), post, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "*/*",
		"Connection":   "keep-alive",
	})
}

// PrepareRapidUploadV2 秒传文件新接口, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareRapidUploadV2(targetPath, policy, uploadid, contentMD5, sliceMD5, dataContent, crc32 string, offset, length, totalSize, dataTime int64, blockListMD5 []string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsError = pcs.CheckIsdir(OperationRapidUpload, targetPath, policy, totalSize)
	if pcsError != nil {
		return nil, pcsError
	}
	rtype := pcs.policyTortype(policy)
	return pcs.prepareRapidUploadV2(targetPath, uploadid, rtype, contentMD5, sliceMD5, dataContent, crc32, offset, length, totalSize, dataTime, blockListMD5)
}

// PrepareFakeRapidUploadV2 仅执行 precreate 握手不进行秒传校验
func (pcs *BaiduPCS) PrepareFakeRapidUploadV2(targetPath, policy string, length, dataTime int64, blockListMD5 []string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsError = pcs.CheckIsdir(OperationRapidUpload, targetPath, policy, length)
	if pcsError != nil {
		return nil, pcsError
	}
	rtype := pcs.policyTortype(policy)
	return pcs.prepareFakeRapidUploadV2(targetPath, rtype, dataTime, blockListMD5)
}

// PrepareUpload 上传单个文件, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareUpload(policy string, targetPath string, uploadFunc UploadFunc) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()

	pcsURL := pcs.generatePCSURL("file", "upload", map[string]string{
		"path":  targetPath,
		"ondup": strings.ReplaceAll(policy, "rsync", "overwrite"),
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationUpload, pcsURL)

	resp, err := uploadFunc(pcsURL.String(), pcs.client.Jar)
	if err != nil {
		handleRespClose(resp)
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationUpload,
			ErrType:   pcserror.ErrTypeNetError,
			Err:       err,
		}
	}

	pcsError = handleRespStatusError(OperationUpload, resp)
	if pcsError != nil {
		return nil, pcsError
	}

	return resp.Body, nil
}

// PrepareUploadTmpFile 分片上传—文件分片及上传, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareUploadTmpFile(uploadFunc UploadFunc) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL := pcs.generatePCSURL("file", "upload", map[string]string{
		"type": "tmpfile",
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationUploadTmpFile, pcsURL)

	resp, err := uploadFunc(pcsURL.String(), pcs.client.Jar)
	if err != nil {
		handleRespClose(resp)
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationUploadTmpFile,
			ErrType:   pcserror.ErrTypeNetError,
			Err:       err,
		}
	}

	pcsError = handleRespStatusError(OperationUploadTmpFile, resp)
	if pcsError != nil {
		return nil, pcsError
	}

	return resp.Body, nil
}

// PrepareUploadCreateSuperFile 分片上传—合并分片文件, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareUploadCreateSuperFile(uploadid, rtype string, fileSize int64, targetPath string, blockList []string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()

	panURL := pcs.generatePanURL("create", nil)
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationUploadCreateSuperFile, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationUploadCreateSuperFile, http.MethodPost, panURL.String(), map[string]string{
		"uploadid":    uploadid,
		"path":        targetPath,
		"size":        strconv.FormatInt(fileSize, 10),
		"isdir":       "0",
		"rtype":       rtype,
		"block_list":  mergeStringList(blockList...),
		"target_path": path.Dir(targetPath),
	}, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

// PrepareUploadPrecreate 分片上传—Precreate, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareUploadPrecreate(targetPath, contentMD5, sliceMD5, crc32 string, size int64, blockList []string) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()
	panURL := &url.URL{
		Scheme: "https",
		Host:   PanBaiduCom,
		Path:   "api/precreate",
	}
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationUploadPrecreate, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationUploadPrecreate, http.MethodPost, panURL.String(), map[string]string{
		"path":         targetPath,
		"size":         strconv.FormatInt(size, 10),
		"isdir":        "0",
		"block_list":   mergeStringList(blockList...),
		"autoinit":     "1",
		"content-md5":  contentMD5,
		"slice-md5":    sliceMD5,
		"contentCrc32": crc32,
		"rtype":        "2",
	}, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

// PrepareUploadSuperfile2 分片上传—Superfile2 接口
func (pcs *BaiduPCS) PrepareUploadSuperfile2(uploadid, targetPath string, partseq int, partOffset int64, uploadFunc UploadFunc) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL := pcs.generatePCSURL("superfile2", "upload", map[string]string{
		"type":       "tmpfile",
		"path":       targetPath,
		"partseq":    strconv.Itoa(partseq),
		"partoffset": strconv.FormatInt(partOffset, 10),
		"uploadid":   uploadid,
		"vip":        "1",
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationUploadSuperfile2, pcsURL)

	resp, err := uploadFunc(pcsURL.String(), pcs.client.Jar)
	if err != nil {
		handleRespClose(resp)
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationUploadSuperfile2,
			ErrType:   pcserror.ErrTypeNetError,
			Err:       err,
		}
	}

	pcsError = handleRespStatusError(OperationUploadSuperfile2, resp)
	if pcsError != nil {
		return nil, pcsError
	}
	return resp.Body, nil
}
