package pcserror

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPCSErrorNoErrorAndUnwrap(t *testing.T) {
	info := NewPCSErrorInfo("测试操作")
	if got := info.Error(); !strings.Contains(got, StrSuccess) {
		t.Fatalf("expected StrSuccess for ErrorTypeNoError, got %q", got)
	}

	sentinel := errors.New("underlying io failure")
	info.SetNetError(sentinel)
	if !errors.Is(info, sentinel) {
		t.Fatalf("expected errors.Is(info, sentinel) to be true via Unwrap()")
	}

	panInfo := NewPanErrorInfo("网盘操作")
	panInfo.SetNetError(io.ErrUnexpectedEOF)
	if !errors.Is(panInfo, io.ErrUnexpectedEOF) {
		t.Fatalf("expected errors.Is(panInfo, io.ErrUnexpectedEOF) to be true via Unwrap()")
	}
}

func TestHandleJSONParseTypedNilSafety(t *testing.T) {
	var typedNil *PCSErrInfo
	err := HandleJSONParse("测试解析", strings.NewReader(`{"error_code":31045,"error_msg":"invalid session"}`), typedNil)
	if err == nil || err.GetRemoteErrCode() != 31045 {
		t.Fatalf("expected remote error 31045 without panic, got %v", err)
	}

	okErr := HandleJSONParse("测试解析", strings.NewReader(`{"error_code":0}`), typedNil)
	if okErr != nil {
		t.Fatalf("expected nil error on error_code=0, got %v", okErr)
	}
}
