package pcserror

// PCSErrInfo PCS错误信息
type PCSErrInfo struct {
	Operation string // 正在进行的操作
	ErrType   ErrType
	Err       error
	ErrCode   int    `json:"error_code"` // 错误代码
	ErrMsg    string `json:"error_msg"`  // 错误消息
}

// NewPCSErrorInfo 提供operation操作名称, 返回 *PCSErrInfo
func NewPCSErrorInfo(operation string) *PCSErrInfo {
	return &PCSErrInfo{
		Operation: operation,
		ErrType:   ErrorTypeNoError,
	}
}

func (pcse *PCSErrInfo) SetJSONError(err error) {
	pcse.ErrType = ErrTypeJSONParseError
	pcse.Err = err
}

func (pcse *PCSErrInfo) SetNetError(err error) {
	pcse.ErrType = ErrTypeNetError
	pcse.Err = err
}

func (pcse *PCSErrInfo) SetRemoteError() {
	pcse.ErrType = ErrTypeRemoteError
}

func (pcse *PCSErrInfo) GetOperation() string {
	return pcse.Operation
}

func (pcse *PCSErrInfo) GetErrType() ErrType {
	return pcse.ErrType
}

func (pcse *PCSErrInfo) GetRemoteErrCode() int {
	return pcse.ErrCode
}

func (pcse *PCSErrInfo) GetRemoteErrMsg() string {
	_, msg := findPCSErr(pcse.ErrCode, pcse.ErrMsg)
	return msg
}

func (pcse *PCSErrInfo) GetError() error {
	return pcse.Err
}

// Unwrap 支持标准库 errors.Is / errors.As 解包底层错误。
func (pcse *PCSErrInfo) Unwrap() error {
	return pcse.Err
}

func (pcse *PCSErrInfo) Error() string {
	return formatError(pcse)
}

func findPCSErr(errCode int, errMsg string) (int, string) {
	switch errCode {
	case 0:
		return errCode, ""
	case 31045:
		return errCode, "操作失败, 可能百度帐号登录状态过期, 请尝试重新登录, 消息: " + errMsg
	case 31061:
		return errCode, "文件已存在"
	case 31066:
		return errCode, "文件或目录不存在"
	case 31079:
		return errCode, "秒传文件失败"
	}
	return errCode, errMsg
}
