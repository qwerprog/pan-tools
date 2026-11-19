package baidupcs

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	jsoniter "github.com/json-iterator/go"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/multipartreader"
)

// PrepareRecycleList 列出回收站文件列表, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareRecycleList(page int) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()

	panURL := pcs.generatePanURL("recycle/list", map[string]string{
		"num":  "100",
		"page": strconv.Itoa(page),
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationRecycleList, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationRecycleList, http.MethodGet, panURL.String(), nil, nil)
}

// PrepareRecycleRestore 还原回收站文件或目录, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareRecycleRestore(fidList ...int64) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()

	pcsURL := pcs.generatePCSURL("file", "restore")
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationRecycleRestore, pcsURL)

	fsIDList := make([]*FsIDJSON, 0, len(fidList))
	for k := range fidList {
		fsIDList = append(fsIDList, &FsIDJSON{
			FsID: fidList[k],
		})
	}
	fsIDListJSON := FsIDListJSON{
		List: fsIDList,
	}

	sendData, err := jsoniter.Marshal(&fsIDListJSON)
	if err != nil {
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationRecycleRestore,
			ErrType:   pcserror.ErrTypeInternalError,
			Err:       err,
		}
	}

	mr := multipartreader.NewMultipartReader()
	mr.AddFormField("param", bytes.NewReader(sendData))
	mr.CloseMultipart()

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationRecycleRestore, http.MethodPost, pcsURL.String(), mr, nil)
}

// PrepareRecycleDelete 删除回收站文件或目录, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareRecycleDelete(fidList ...int64) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()

	panURL := pcs.generatePanURL("recycle/delete", nil)
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationRecycleDelete, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationRecycleDelete, http.MethodPost, panURL.String(), map[string]string{
		"fidlist": mergeInt64List(fidList...),
	}, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

// PrepareRecycleClear 清空回收站, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareRecycleClear() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()

	pcsURL := pcs.generatePCSURL("file", "delete", map[string]string{
		"type": "recycle",
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationRecycleClear, pcsURL)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationRecycleClear, http.MethodGet, pcsURL.String(), nil, nil)
}
