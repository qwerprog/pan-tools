package baidupcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
)

const (
	// OperationMembershipInfo 获取当前用户会员等级与身份信息
	OperationMembershipInfo = "获取用户会员信息"
)

// MembershipInfo 描述百度网盘账号当前的会员类型、等级与展示标签。
type MembershipInfo struct {
	VIPType       int    `json:"vip_type"`
	Level         int    `json:"level"`
	Cluster       string `json:"cluster"`
	DetailCluster string `json:"detail_cluster"`
	IsVIP         bool   `json:"is_vip"`
	IsSVIP        bool   `json:"is_svip"`
	Label         string `json:"label"`
}

type membershipCurrentProduct struct {
	Cluster       string `json:"cluster"`
	DetailCluster string `json:"detail_cluster"`
}

type membershipLevelInfo struct {
	CurrentLevel int `json:"current_level"`
}

type membershipUserJSON struct {
	*pcserror.PCSErrInfo
	ErrNo          int             `json:"errno"`
	VIPType        int             `json:"vip_type"`
	LevelInfo      json.RawMessage `json:"level_info"`
	CurrentProduct json.RawMessage `json:"current_product"`
	ProductInfos   []struct {
		Cluster       string `json:"cluster"`
		DetailCluster string `json:"detail_cluster"`
		Status        int    `json:"status"`
		EndTime       int64  `json:"end_time"`
	} `json:"product_infos"`
}

// PrepareMembershipInfo 请求百度网盘会员查询接口，只返回响应流与错误信息。
func (pcs *BaiduPCS) PrepareMembershipInfo() (dataReadCloser io.ReadCloser, pcsError pcserror.Error) {
	pcs.lazyInit()
	pcsURL2 := pcs.generatePCSURL2("membership/user", "query", map[string]string{
		"app_id": PanAppID,
		"web":    "5",
	})
	return pcs.sendReqReturnReadCloser(reqTypePan, OperationMembershipInfo, http.MethodGet, pcsURL2.String(), nil, nil)
}

// MembershipInfo 获取当前登录账号的 VIP / SVIP 类型与成长等级。
func (pcs *BaiduPCS) MembershipInfo() (*MembershipInfo, pcserror.Error) {
	dataReadCloser, pcsError := pcs.PrepareMembershipInfo()
	if pcsError != nil {
		return nil, pcsError
	}
	defer dataReadCloser.Close()

	return parseMembershipInfo(dataReadCloser, time.Now().Unix())
}

func parseMembershipInfo(r io.Reader, nowUnix int64) (*MembershipInfo, pcserror.Error) {
	errInfo := pcserror.NewPCSErrorInfo(OperationMembershipInfo)
	resp := &membershipUserJSON{
		PCSErrInfo: errInfo,
	}
	pcsError := pcserror.HandleJSONParse(OperationMembershipInfo, r, resp)
	if pcsError != nil {
		return nil, pcsError
	}
	if resp.ErrNo != 0 {
		errInfo.ErrCode = resp.ErrNo
		errInfo.SetRemoteError()
		return nil, errInfo
	}

	var curProd membershipCurrentProduct
	if trimmed := bytes.TrimSpace(resp.CurrentProduct); len(trimmed) > 0 && trimmed[0] == '{' {
		_ = jsoniter.Unmarshal(trimmed, &curProd)
	}
	var lvlInfo membershipLevelInfo
	if trimmed := bytes.TrimSpace(resp.LevelInfo); len(trimmed) > 0 && trimmed[0] == '{' {
		_ = jsoniter.Unmarshal(trimmed, &lvlInfo)
	}

	cluster := strings.ToLower(strings.TrimSpace(curProd.Cluster))
	detail := strings.ToLower(strings.TrimSpace(curProd.DetailCluster))
	if detail == "" && len(resp.ProductInfos) > 0 {
		for _, p := range resp.ProductInfos {
			if p.Status != 0 || (p.EndTime > 0 && p.EndTime <= nowUnix) {
				continue
			}
			dc := strings.ToLower(strings.TrimSpace(p.DetailCluster))
			if dc == "svip" {
				detail = "svip"
				cluster = strings.ToLower(strings.TrimSpace(p.Cluster))
				break
			}
			if dc == "vip" && detail == "" {
				detail = "vip"
				cluster = strings.ToLower(strings.TrimSpace(p.Cluster))
			}
		}
	}

	isSVIP := detail == "svip" || resp.VIPType == 2
	isVIP := isSVIP || detail == "vip" || resp.VIPType == 1
	level := lvlInfo.CurrentLevel

	label := FormatVIPLabel(isSVIP, isVIP, level)
	return &MembershipInfo{
		VIPType:       resp.VIPType,
		Level:         level,
		Cluster:       cluster,
		DetailCluster: detail,
		IsVIP:         isVIP,
		IsSVIP:        isSVIP,
		Label:         label,
	}, nil
}

// FormatVIPLabel 将会员类型与等级格式化为简短展示标签（例如 "SVIP 10"、"VIP 3"、"FREE"）。
func FormatVIPLabel(isSVIP, isVIP bool, level int) string {
	switch {
	case isSVIP:
		if level > 0 {
			return fmt.Sprintf("SVIP %d", level)
		}
		return "SVIP"
	case isVIP:
		if level > 0 {
			return fmt.Sprintf("VIP %d", level)
		}
		return "VIP"
	default:
		return "FREE"
	}
}
