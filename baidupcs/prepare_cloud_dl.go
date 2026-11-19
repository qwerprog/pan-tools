package baidupcs

import (
	"io"
	"net/http"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
)

// PrepareCloudDlAddTask 添加离线下载任务, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareCloudDlAddTask(sourceURL, savePath string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL2 := pcs.generatePCSURL2("services/cloud_dl", "add_task", map[string]string{
		"app_id":       PanAppID,
		"task_from":    "0",
		"selected_idx": "1",
		"save_path":    savePath,
		"source_url":   sourceURL,
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationCloudDlAddTask, pcsURL2)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationCloudDlAddTask, http.MethodPost, pcsURL2.String(), nil, nil)
}

// PrepareCloudDlQueryTask 精确查询离线下载任务, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareCloudDlQueryTask(taskIDs string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL2 := pcs.generatePCSURL2("services/cloud_dl", "query_task", map[string]string{
		"app_id":   PanAppID,
		"op_type":  "1",
		"task_ids": taskIDs,
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationCloudDlQueryTask, pcsURL2)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationCloudDlQueryTask, http.MethodGet, pcsURL2.String(), nil, nil)
}

// PrepareCloudDlListTask 查询离线下载任务列表, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareCloudDlListTask() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL2 := pcs.generatePCSURL2("services/cloud_dl", "list_task", map[string]string{
		"need_task_info": "1",
		"status":         "255",
		"start":          "0",
		"limit":          "1000",
		"app_id":         PanAppID,
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationCloudDlListTask, pcsURL2)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationCloudDlListTask, http.MethodPost, pcsURL2.String(), nil, nil)
}

func (pcs *BaiduPCS) prepareCloudDlCDTask(operation, method string, taskID int64) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL2 := pcs.generatePCSURL2("services/cloud_dl", method, map[string]string{
		"app_id":  PanAppID,
		"task_id": strconv.FormatInt(taskID, 10),
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", operation, pcsURL2)

	return pcs.sendReqReturnReadCloser(reqTypePCS, operation, http.MethodPost, pcsURL2.String(), nil, nil)
}

// PrepareCloudDlCancelTask 取消离线下载任务, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareCloudDlCancelTask(taskID int64) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	return pcs.prepareCloudDlCDTask(OperationCloudDlCancelTask, "cancel_task", taskID)
}

// PrepareCloudDlDeleteTask 删除离线下载任务, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareCloudDlDeleteTask(taskID int64) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	return pcs.prepareCloudDlCDTask(OperationCloudDlDeleteTask, "delete_task", taskID)
}

// PrepareCloudDlClearTask 清空离线下载任务记录, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareCloudDlClearTask() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL2 := pcs.generatePCSURL2("services/cloud_dl", "clear_task")
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationCloudDlClearTask, pcsURL2)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationCloudDlClearTask, http.MethodPost, pcsURL2.String(), nil, nil)
}
