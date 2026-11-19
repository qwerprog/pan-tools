package downloader

import (
	"sync/atomic"

	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

type (
	// WorkerStatuser 状态接口
	WorkerStatuser interface {
		StatusCode() StatusCode
		StatusText() string
	}

	// StatusCode 状态码
	StatusCode int

	// WorkerStatus 并发安全的 worker 状态封装
	WorkerStatus struct {
		code atomic.Int32
	}

	// DownloadStatusFunc 下载状态处理函数
	DownloadStatusFunc func(status transfer.DownloadStatuser, workersCallback func(RangeWorkerFunc))
)

const (
	// StatusCodeInit 初始化
	StatusCodeInit StatusCode = iota
	// StatusCodeSucceeded 成功
	StatusCodeSucceeded
	// StatusCodePending 等待响应
	StatusCodePending
	// StatusCodeDownloading 下载中
	StatusCodeDownloading
	// StatusCodeWaitToWrite 等待写入数据
	StatusCodeWaitToWrite
	// StatusCodeInternalError 内部错误
	StatusCodeInternalError
	// StatusCodeTooManyConnections 连接数太多
	StatusCodeTooManyConnections
	// StatusCodeNetError 网络错误
	StatusCodeNetError
	// StatusCodeFailed 下载失败
	StatusCodeFailed
	// StatusCodePaused 已暂停
	StatusCodePaused
	// StatusCodeReset 已重设连接
	StatusCodeReset
	// StatusCodeCanceled 已取消
	StatusCodeCanceled
)

// GetStatusText 根据状态码获取状态信息
func GetStatusText(sc StatusCode) string {
	switch sc {
	case StatusCodeInit:
		return "初始化"
	case StatusCodeSucceeded:
		return "成功"
	case StatusCodePending:
		return "等待响应"
	case StatusCodeDownloading:
		return "下载中"
	case StatusCodeWaitToWrite:
		return "等待写入数据"
	case StatusCodeInternalError:
		return "内部错误"
	case StatusCodeTooManyConnections:
		return "连接数太多"
	case StatusCodeNetError:
		return "网络错误"
	case StatusCodeFailed:
		return "下载失败"
	case StatusCodePaused:
		return "已暂停"
	case StatusCodeReset:
		return "已重设连接"
	case StatusCodeCanceled:
		return "已取消"
	default:
		return "未知状态码"
	}
}

// NewWorkerStatus 初始化 WorkerStatus
func NewWorkerStatus() *WorkerStatus {
	ws := &WorkerStatus{}
	ws.SetStatusCode(StatusCodeInit)
	return ws
}

// SetStatusCode 原子更新 worker 状态码
func (ws *WorkerStatus) SetStatusCode(sc StatusCode) {
	ws.code.Store(int32(sc))
}

// StatusCode 原子读取状态码
func (ws *WorkerStatus) StatusCode() StatusCode {
	return StatusCode(ws.code.Load())
}

// StatusText 返回状态文本
func (ws *WorkerStatus) StatusText() string {
	return GetStatusText(ws.StatusCode())
}
