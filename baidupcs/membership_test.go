package baidupcs

import (
	"strings"
	"testing"
)

func TestParseMembershipInfo(t *testing.T) {
	svipPayload := `{"error_code":0,"vip_type":2,"level_info":{"current_level":10},"current_product":{"cluster":"vip","detail_cluster":"svip"}}`
	info, err := parseMembershipInfo(strings.NewReader(svipPayload), 1700000000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.IsSVIP || !info.IsVIP || info.Level != 10 || info.Label != "SVIP 10" {
		t.Fatalf("unexpected SVIP parsed info: %+v", info)
	}

	vipPayload := `{"error_code":0,"vip_type":1,"level_info":{"current_level":3},"current_product":{"cluster":"vip","detail_cluster":"vip"}}`
	info, err = parseMembershipInfo(strings.NewReader(vipPayload), 1700000000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IsSVIP || !info.IsVIP || info.Label != "VIP 3" {
		t.Fatalf("unexpected VIP parsed info: %+v", info)
	}

	freePayload := `{"error_code":0,"vip_type":0,"level_info":{"current_level":0},"current_product":{}}`
	info, err = parseMembershipInfo(strings.NewReader(freePayload), 1700000000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IsSVIP || info.IsVIP || info.Label != "FREE" {
		t.Fatalf("unexpected FREE parsed info: %+v", info)
	}

	// Baidu PHP backend returns [] instead of {} for empty associative arrays on free/expired accounts (#17).
	phpEmptyArrayPayload := `{"error_code":0,"vip_type":0,"level_info":[],"current_product":[]}`
	info, err = parseMembershipInfo(strings.NewReader(phpEmptyArrayPayload), 1700000000)
	if err != nil {
		t.Fatalf("expected current_product:[] to parse cleanly without error, got: %v", err)
	}
	if info.IsSVIP || info.IsVIP || info.Label != "FREE" {
		t.Fatalf("unexpected PHP empty array parsed info: %+v", info)
	}

	fallbackProductInfosPayload := `{"error_code":0,"vip_type":0,"level_info":{"current_level":6},"current_product":[],"product_infos":[{"cluster":"vip","detail_cluster":"svip","status":0,"end_time":1800000000}]}`
	info, err = parseMembershipInfo(strings.NewReader(fallbackProductInfosPayload), 1700000000)
	if err != nil {
		t.Fatalf("expected fallback to product_infos when current_product:[], got: %v", err)
	}
	if !info.IsSVIP || info.Label != "SVIP 6" {
		t.Fatalf("expected SVIP 6 from product_infos fallback, got %+v", info)
	}
}
