package baidupcs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/netdisksign"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/multipartreader"
)

// PrepareFilesDirectoriesBatchMeta 获取多个文件/目录的元信息, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareFilesDirectoriesBatchMeta(paths ...string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	sendData, err := (&PathsListJSON{}).JSON(paths...)
	if err != nil {
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationFilesDirectoriesMeta,
			ErrType:   pcserror.ErrTypeInternalError,
			Err:       err,
		}
	}

	pcsURL := pcs.generatePCSURL("file", "meta")
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationFilesDirectoriesMeta, pcsURL)

	mr := multipartreader.NewMultipartReader()
	mr.AddFormField("param", bytes.NewReader(sendData))
	mr.CloseMultipart()

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationFilesDirectoriesMeta, http.MethodPost, pcsURL.String(), mr, nil)
}

// PrepareFilesDirectoriesList 获取目录下的文件和目录列表, 只返回服务器响应数据和错误信息
//
// 走 pan.baidu.com/api/list 接口: PCS 的 file/list 单次最多返回 1000 条且 num/page 参数被忽略,
// 网盘 web 列表接口支持 num/page 翻页 (修复 #511)。
func (pcs *BaiduPCS) PrepareFilesDirectoriesList(path string, options *OrderOptions, num, page int) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	if options == nil {
		options = DefaultOrderOptions
	}
	if path == "" {
		path = PathSeparator
	}
	if num <= 0 || num > maxListNum {
		num = maxListNum
	}
	if page <= 0 {
		page = 1
	}

	desc := "0"
	if options.Order == OrderDesc {
		desc = "1"
	}
	panURL := pcs.generatePanURL("list", map[string]string{
		"dir":        path,
		"order":      string(options.By),
		"desc":       desc,
		"clienttype": "0",
		"num":        strconv.Itoa(num),
		"page":       strconv.Itoa(page),
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationFilesDirectoriesList, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationFilesDirectoriesList, http.MethodGet, panURL.String(), nil, nil)
}

// PrepareSearch 按文件名搜索文件, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareSearch(targetPath, keyword string, recursive bool) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	re := "0"
	if recursive {
		re = "1"
	}
	pcsURL := pcs.generatePCSURL("file", "search", map[string]string{
		"path": targetPath,
		"wd":   keyword,
		"re":   re,
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationSearch, pcsURL)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationSearch, http.MethodGet, pcsURL.String(), nil, nil)
}

// PrepareRemove 批量删除文件/目录, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareRemove(paths ...string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	sendData, err := (&PathsListJSON{}).JSON(paths...)
	if err != nil {
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationRemove,
			ErrType:   pcserror.ErrTypeInternalError,
			Err:       err,
		}
	}

	pcsURL := pcs.generatePCSURL("file", "delete")
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationRemove, pcsURL)

	mr := multipartreader.NewMultipartReader()
	mr.AddFormField("param", bytes.NewReader(sendData))
	mr.CloseMultipart()

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationRemove, http.MethodPost, pcsURL.String(), mr, nil)
}

// PrepareMkdir 创建目录, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareMkdir(pcspath string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL := pcs.generatePCSURL("file", "mkdir", map[string]string{
		"path": pcspath,
	})
	baiduPCSVerbose.Infof("%s URL: %s", OperationMkdir, pcsURL)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationMkdir, http.MethodPost, pcsURL.String(), nil, nil)
}

func (pcs *BaiduPCS) prepareCpMvOp(op string, cpmvJSON ...*CpMvJSON) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	var method string
	switch op {
	case OperationCopy:
		method = "copy"
	case OperationMove, OperationRename:
		method = "move"
	default:
		return nil, &pcserror.PCSErrInfo{
			Operation: op,
			ErrType:   pcserror.ErrTypeInternalError,
			Err:       fmt.Errorf("unknown operation: %s", op),
		}
	}

	sendData, err := (&CpMvListJSON{
		List: cpmvJSON,
	}).JSON()
	if err != nil {
		return nil, &pcserror.PCSErrInfo{
			Operation: op,
			ErrType:   pcserror.ErrTypeInternalError,
			Err:       err,
		}
	}

	pcsURL := pcs.generatePCSURL("file", method)
	baiduPCSVerbose.Infof("%s URL: %s\n", op, pcsURL)

	mr := multipartreader.NewMultipartReader()
	mr.AddFormField("param", bytes.NewReader(sendData))
	mr.CloseMultipart()

	return pcs.sendReqReturnReadCloser(reqTypePCS, op, http.MethodPost, pcsURL.String(), mr, nil)
}

// PrepareLocateDownload 获取下载链接, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareLocateDownload(pcspath string) (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	bduss := pcs.GetBDUSS()
	if pcs.uid == 0 {
		uk, ukErr := pcs.CacheUK()
		if ukErr != nil {
			return nil, ukErr
		}
		if uk <= 0 {
			return nil, &pcserror.PCSErrInfo{
				Operation: OperationLocateDownload,
				ErrType:   pcserror.ErrTypeOthers,
				Err:       errors.New("无法获取有效的用户 UID"),
			}
		}
		pcs.uid = uint64(uk)
	}

	ns := netdisksign.NewLocateDownloadSign(pcs.uid, bduss)
	pcsURL := &url.URL{
		Scheme: GetHTTPScheme(pcs.isHTTPS),
		Host:   pcs.URL().Host,
		Path:   "/rest/2.0/pcs/file",
		RawQuery: (url.Values{
			"ant":        []string{"1"},
			"check_blue": []string{"1"},
			"es":         []string{"1"},
			"esl":        []string{"1"},
			"app_id":     []string{PanAppID},
			"method":     []string{"locatedownload"},
			"path":       []string{pcspath},
			"ver":        []string{"4.0"},
			"clienttype": []string{"17"},
			"channel":    []string{"0"},
			"apn_id":     []string{"1_0"},
			"freeisp":    []string{"0"},
			"queryfree":  []string{"0"},
			"use":        []string{"0"},
		}).Encode() + "&" + ns.URLParam(),
	}
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationLocateDownload, pcsURL)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationLocateDownload, http.MethodPost, pcsURL.String(), nil, pcs.getPanUAHeader())
}

// PrepareLocatePanAPIDownload 从百度网盘首页获取下载链接, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareLocatePanAPIDownload(fidList ...int64) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()
	sign, err := pcs.ph.CacheSignature()
	if err != nil {
		return nil, &pcserror.PanErrorInfo{
			Operation: OperationLocatePanAPIDownload,
			ErrType:   pcserror.ErrTypeOthers,
			Err:       err,
		}
	}

	panURL := pcs.generatePanURL("download", nil)
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationLocatePanAPIDownload, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationLocatePanAPIDownload, http.MethodPost, panURL.String(), map[string]string{
		"sign":      sign.Sign(),
		"timestamp": sign.Timestamp(),
		"fidlist":   mergeInt64List(fidList...),
	}, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}
