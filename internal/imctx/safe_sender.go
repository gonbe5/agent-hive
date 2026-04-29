package imctx

import (
	"crypto/sha256"
	"encoding/hex"
)

// SafeSenderID 是 Phase 0 P0-#12 的 PII 防御核心：把 IM 平台的 raw 用户标识
// （飞书 open_id / union_id、企微 userid、钉钉 staffid、微信 openid 等）
// 单向折叠为 8 个 hex 字符的稳定指纹（sha256(rawID)[:4]）。
//
// 设计取舍（不可改）：
//   - 单向：sha256 截前 4 字节，不可反推。任何"加 secret 让它可还原"的诉求
//     必须走独立的 audit 表（行级访问控制 + retention），而不是反向函数。
//   - 稳定：相同 raw 输入恒得相同输出，便于跨日志聚合追踪同一用户行为，
//     但聚合体仅暴露 8 hex 指纹，不再泄露真实身份。
//   - 短：8 字符足够日志/metric label 区分用户（4 字节 = 4G 桶），同时把
//     metric label 高基数风险压到可接受水平（同一租户活跃用户级别）。
//   - stdlib-only：本包是 Phase 0 P0-#1 leaf 包，禁止引入 internal/* 依赖；
//     CI gate scripts/ci/check_imctx_leaf.sh 守卫。
//
// 调用约定：
//   - 任何 zap.String / zap.Any / metric label / fmt.Errorf / trace span attribute
//     需要带"用户身份"时，必须先经此函数。CI gate scripts/ci/check_pii_safe_sender.sh
//     扫描违规直接 fail。
//   - 空输入返回空字符串（而非 hash("")），避免上层把"空字段"误判为有效指纹。
func SafeSenderID(rawID string) string {
	if rawID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(rawID))
	return hex.EncodeToString(sum[:4])
}
