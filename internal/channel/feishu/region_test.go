package feishu

import "testing"

// TestFeishuClientOptionsForRegion 是 Phase 7 缺口 14 蓝军点。
//
// 不变式:Region=intl/lark/international 必须返回 lark.WithOpenBaseUrl 切到
// open.larksuite.com;cn / 空 / 未知 region 必须 fallback 到 SDK 默认(返 nil)。
//
// 蓝军 mutation 点:把 case "intl", "lark", "international" 改成 "intl-disabled"
// 之类,本测试 lark 子用例必红;或把 default 改成返回 lark options,本测试 cn 子用例必红。
func TestFeishuClientOptionsForRegion(t *testing.T) {
	tests := []struct {
		region    string
		wantNil   bool
		wantCount int
	}{
		{"intl", false, 1},
		{"lark", false, 1},
		{"international", false, 1},
		{"cn", true, 0},
		{"", true, 0},
		{"unknown_region", true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.region, func(t *testing.T) {
			opts := feishuClientOptionsForRegion(tc.region)
			if tc.wantNil {
				if opts != nil {
					t.Fatalf("region=%q: expected nil opts, got %d", tc.region, len(opts))
				}
				return
			}
			if len(opts) != tc.wantCount {
				t.Fatalf("region=%q: expected %d opts, got %d", tc.region, tc.wantCount, len(opts))
			}
		})
	}
}
