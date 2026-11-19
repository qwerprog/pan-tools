package baidupcs

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/netdisksign"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// PrepareSharePSet 私密分享文件, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareSharePSet(paths []string, pwd string, period int) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()
	panURL := &url.URL{
		Scheme: "https",
		Host:   PanBaiduCom,
		Path:   "share/pset",
	}
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationShareSet, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationShareSet, http.MethodPost, panURL.String(), map[string]string{
		"path_list":    mergeStringList(paths...),
		"schannel":     "4",
		"channel_list": "[]",
		"period":       strconv.Itoa(period),
		"pwd":          pwd,
		"share_type":   "9",
	}, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

// PrepareShareCancel 取消分享, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareShareCancel(shareIDs []int64) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()
	panURL := &url.URL{
		Scheme: "https",
		Host:   PanBaiduCom,
		Path:   "share/cancel",
	}
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationShareCancel, panURL)

	ss := converter.SliceInt64ToString(shareIDs)
	return pcs.sendReqReturnReadCloser(reqTypePan, OperationShareCancel, http.MethodPost, panURL.String(), map[string]string{
		"shareid_list": "[" + strings.Join(ss, ",") + "]",
	}, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

// PrepareShareList 列出分享列表, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareShareList(page int) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()

	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("desc", "1")
	query.Set("order", "time")

	panURL := &url.URL{
		Scheme:   "https",
		Host:     PanBaiduCom,
		Path:     "share/record",
		RawQuery: query.Encode(),
	}
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationShareList, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationShareList, http.MethodGet, panURL.String(), nil, nil)
}

// PrepareShareSURLInfo 获取分享的详细信息, 包含密码, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareShareSURLInfo(shareID int64) (dataReadCloser io.ReadCloser, panError pcserror.Error) {
	pcs.lazyInit()

	query := url.Values{}
	query.Set("shareid", strconv.FormatInt(shareID, 10))
	query.Set("sign", converter.ToString(netdisksign.ShareSURLInfoSign(shareID)))

	panURL := &url.URL{
		Scheme:   "https",
		Host:     PanBaiduCom,
		Path:     "share/surlinfoinrecord",
		RawQuery: query.Encode(),
	}
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationShareSURLInfo, panURL)

	return pcs.sendReqReturnReadCloser(reqTypePan, OperationShareSURLInfo, http.MethodGet, panURL.String(), nil, nil)
}
