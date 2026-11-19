package baidupcs

import (
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
)

type reqType int

const (
	reqTypePCS reqType = iota
	reqTypePan
)

func handleRespClose(resp *http.Response) error {
	if resp != nil {
		return resp.Body.Close()
	}
	return nil
}

func handleRespStatusError(operation string, resp *http.Response) pcserror.Error {
	switch resp.StatusCode / 100 {
	case 4, 5:
		errInfo := pcserror.DecodePCSJSONError(operation, resp.Body)
		resp.Body.Close()
		if errInfo == nil {
			netErr := pcserror.NewPCSErrorInfo(operation)
			netErr.SetNetError(errors.New("http 响应错误: " + resp.Status))
			return netErr
		}
		errInfo.SetRemoteError()
		return errInfo
	}
	return nil
}

func (pcs *BaiduPCS) sendReqReturnResp(rt reqType, op, method, urlStr string, post interface{}, header map[string]string) (resp *http.Response, pcsError pcserror.Error) {
	if header == nil {
		header = map[string]string{}
	}

	if _, uaok := header["User-Agent"]; !uaok {
		switch rt {
		case reqTypePCS:
			header["User-Agent"] = pcs.pcsUA
		case reqTypePan:
			header["User-Agent"] = pcs.panUA
		}
	}

	resp, err := pcs.client.Req(method, urlStr, post, header)
	if err != nil {
		handleRespClose(resp)
		switch rt {
		case reqTypePCS:
			return nil, &pcserror.PCSErrInfo{
				Operation: op,
				ErrType:   pcserror.ErrTypeNetError,
				Err:       err,
			}
		case reqTypePan:
			return nil, &pcserror.PanErrorInfo{
				Operation: op,
				ErrType:   pcserror.ErrTypeNetError,
				Err:       err,
			}
		default:
			return nil, &pcserror.PCSErrInfo{
				Operation: op,
				ErrType:   pcserror.ErrTypeInternalError,
				Err:       err,
			}
		}
	}
	return resp, nil
}

func (pcs *BaiduPCS) sendReqReturnReadCloser(rt reqType, op, method, urlStr string, post interface{}, header map[string]string) (readCloser io.ReadCloser, pcsError pcserror.Error) {
	resp, pcsError := pcs.sendReqReturnResp(rt, op, method, urlStr, post, header)
	if pcsError != nil {
		return nil, pcsError
	}
	return resp.Body, nil
}

// PrepareUK 获取用户 UK, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareUK() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()

	query := url.Values{}
	query.Set("need_selfinfo", "1")

	panURL := &url.URL{
		Scheme:   "https",
		Host:     PanBaiduCom,
		Path:     "api/user/getinfo",
		RawQuery: query.Encode(),
	}

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationGetUK, http.MethodGet, panURL.String(), nil, nil)
}

// PreparePCSServers 获取推荐的 pcs 服务器 URL
func (pcs *BaiduPCS) PreparePCSServers() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL := pcs.generatePCSURL("file", "locateupload", map[string]string{
		"upload_version": "2.0",
		"app_id":         PanAppID,
	})
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationGetPCSServer, pcsURL)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationGetPCSServer, http.MethodGet, pcsURL.String(), nil, nil)
}

// PrepareQuotaInfo 获取当前用户空间配额信息, 只返回服务器响应数据和错误信息
func (pcs *BaiduPCS) PrepareQuotaInfo() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL := pcs.generatePCSURL("quota", "info")
	baiduPCSVerbose.Infof("%s URL: %s\n", OperationQuotaInfo, pcsURL)

	return pcs.sendReqReturnReadCloser(reqTypePCS, OperationQuotaInfo, http.MethodGet, pcsURL.String(), nil, nil)
}
