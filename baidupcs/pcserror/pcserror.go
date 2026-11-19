// Package pcserror PCS错误包
package pcserror

import (
	"fmt"
	"io"
	"reflect"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
)

type (
	// ErrType 错误类型
	ErrType int

	// Error 错误信息接口
	Error interface {
		error
		SetJSONError(err error)
		SetNetError(err error)
		SetRemoteError()
		GetOperation() string
		GetErrType() ErrType
		GetRemoteErrCode() int
		GetRemoteErrMsg() string
		GetError() error
	}
)

const (
	// ErrorTypeNoError 无错误
	ErrorTypeNoError ErrType = iota
	// ErrTypeInternalError 内部错误
	ErrTypeInternalError
	// ErrTypeRemoteError 远端服务器返回错误
	ErrTypeRemoteError
	// ErrTypeNetError 网络错误
	ErrTypeNetError
	// ErrTypeJSONParseError json 数据解析失败
	ErrTypeJSONParseError
	// ErrTypeOthers 其他错误
	ErrTypeOthers
)

const (
	// StrSuccess 操作成功
	StrSuccess = "操作成功"
	// StrInternalError 内部错误
	StrInternalError = "内部错误"
	// StrRemoteError 远端服务器返回错误
	StrRemoteError = "远端服务器返回错误"
	// StrNetError 网络错误
	StrNetError = "网络错误"
	// StrJSONParseError json 数据解析失败
	StrJSONParseError = "json 数据解析失败"
)

func formatError(e Error) string {
	op := e.GetOperation()
	rawErr := e.GetError()
	if op == "" {
		if rawErr != nil {
			return rawErr.Error()
		}
		return StrSuccess
	}

	switch e.GetErrType() {
	case ErrorTypeNoError:
		return fmt.Sprintf("%s: %s", op, StrSuccess)
	case ErrTypeInternalError:
		return fmt.Sprintf("%s: %s, %s", op, StrInternalError, rawErr)
	case ErrTypeJSONParseError:
		return fmt.Sprintf("%s: %s, %s", op, StrJSONParseError, rawErr)
	case ErrTypeNetError:
		return fmt.Sprintf("%s: %s, %s", op, StrNetError, rawErr)
	case ErrTypeRemoteError:
		code := e.GetRemoteErrCode()
		if code == 0 {
			return fmt.Sprintf("%s: %s", op, StrSuccess)
		}
		return fmt.Sprintf("%s: 遇到错误, %s, 代码: %d, 消息: %s", op, StrRemoteError, code, e.GetRemoteErrMsg())
	case ErrTypeOthers:
		if rawErr == nil {
			return fmt.Sprintf("%s: %s", op, StrSuccess)
		}
		return fmt.Sprintf("%s, 遇到错误, %s", op, rawErr)
	default:
		return fmt.Sprintf("%s: unknown ErrType (%d)", op, e.GetErrType())
	}
}

// DecodePCSJSONError 解析PCS JSON的错误
func DecodePCSJSONError(operation string, data io.Reader) Error {
	errInfo := NewPCSErrorInfo(operation)
	return HandleJSONParse(operation, data, errInfo)
}

// DecodePanJSONError 解析Pan JSON的错误
func DecodePanJSONError(operation string, data io.Reader) Error {
	errInfo := NewPanErrorInfo(operation)
	return HandleJSONParse(operation, data, errInfo)
}

// HandleJSONParse 处理解析json
func HandleJSONParse(op string, data io.Reader, info Error) (pcsError Error) {
	if info == nil || (reflect.ValueOf(info).Kind() == reflect.Ptr && reflect.ValueOf(info).IsNil()) {
		info = NewPCSErrorInfo(op)
	}

	if err := pcsutil.UnmarshalData(data, info); err != nil {
		info.SetJSONError(err)
		return info
	}

	// 设置出错类型为远程错误
	if info.GetRemoteErrCode() != 0 {
		info.SetRemoteError()
		return info
	}
	return nil
}
