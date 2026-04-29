#!/usr/bin/env bash
# check_pii_safe_sender.sh — Phase 0 P0-#12 CI gate
#
# 红队场景：
#   raw open_id / union_id / sender_id 直接进 logger / metric / error
#   → 进监控聚合 → 三方 SaaS 看到 → 用户身份泄露
#   raw open_id 进 metric label → Prometheus 高基数 + PII 双违规
#
# 核心断言：
#   (a) zap.String/zap.Stringp/zap.Any 等日志字段中以 "open_id" / "sender_id" /
#       "union_id" 命名的 key，第二个参数必须是 SafeSenderID(...) 包装；不允许直接
#       塞 raw 变量（如 senderID / openID / *.OpenId / *.UnionId）。
#   (b) fmt.Errorf / errors.New 的格式串中不得出现 raw open_id / union_id 变量
#       拼接（%s 接 senderID/openID 等）。
#   (c) Prometheus / OTel metric label 不得直接以 raw open_id 为 label value。
#
# 白名单：
#   - 测试文件 (_test.go)
#   - internal/imctx/safe_sender.go（实现本身）
#   - internal/channel/feishu/card_decode.go（包内 alias 转发函数）
#   - 注释行 / 整段注释（grep 后过滤以 // 开头的行 + 多行注释见 inline 处理）
#   - 字段值已包裹在 SafeSenderID(...) 或 imctx.SafeSenderID(...) 中
#
# 退出码：0=clean，1=发现违例，2=env 错误
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT"

if [ ! -d "internal" ]; then
  echo "ERR: internal/ missing" >&2
  exit 2
fi

violations=0

# 公共过滤：去掉测试文件 + imctx/safe_sender.go 与 card_decode.go 的 alias 函数体
# 以及单行注释（// 开头）与文档行
filter_allowed() {
  grep -v '_test\.go:' \
    | grep -v '^internal/imctx/safe_sender\.go:' \
    | grep -v '^internal/channel/feishu/card_decode\.go:.*func SafeSenderID' \
    | grep -vE ':\s*//' || true
}

# (a) zap 日志字段：key 名包含 open_id / sender_id / union_id，但 value 不是 SafeSenderID 包装
echo "== gate (a): zap.* 日志字段中 PII 必须经 SafeSenderID =="
# 匹配 zap.String("...sender_id..." / "...open_id..." / "...union_id...", <value>)
# 排除：value 中已含 SafeSenderID 字符串
hits_a=$(grep -rnE \
  '(zap\.(String|Stringp|Any))\(\s*"[^"]*(sender_id|open_id|union_id)[^"]*"\s*,\s*[^)]*\)' \
  --include='*.go' internal/ 2>/dev/null \
  | grep -vE 'SafeSenderID|safe_sender_id|safe_bot_id|safe_operator_id' \
  | filter_allowed || true)
if [ -n "$hits_a" ]; then
  echo "FAIL: 以下 zap.* 调用把 raw open_id/union_id/sender_id 直接喂给日志："
  echo "$hits_a" | sed 's/^/  - /'
  violations=$((violations+1))
else
  echo "PASS: 无未脱敏 zap 日志字段"
fi

# (b) fmt.Errorf / errors.New 格式串中拼 raw open_id / union_id 变量
echo
echo "== gate (b): fmt.Errorf/errors.New 不得拼接 raw open_id 变量 =="
hits_b=$(grep -rnE \
  '(fmt\.Errorf|errors\.New)\(.*\b(senderID|openID|unionID|openId|unionId|OpenId|UnionId)\b' \
  --include='*.go' internal/ 2>/dev/null \
  | grep -vE 'SafeSenderID' \
  | filter_allowed || true)
if [ -n "$hits_b" ]; then
  echo "FAIL: 以下 error 构造把 raw open_id/union_id 拼进文案："
  echo "$hits_b" | sed 's/^/  - /'
  violations=$((violations+1))
else
  echo "PASS: 无 error 拼接泄露"
fi

# (c) Prometheus / OTel metric label 直接接 raw open_id 变量
echo
echo "== gate (c): metric label 不得使用 raw open_id =="
# WithLabelValues(...) / Labels{...} / attribute.String("open_id"/"sender_id"/"union_id", <raw>)
hits_c1=$(grep -rnE \
  '(WithLabelValues|Labels\{)[^)]*\b(senderID|openID|unionID|openId|unionId|OpenId|UnionId)\b' \
  --include='*.go' internal/ 2>/dev/null \
  | grep -vE 'SafeSenderID' \
  | filter_allowed || true)
hits_c2=$(grep -rnE \
  'attribute\.(String|KeyValue)\(\s*"[^"]*(open_id|sender_id|union_id)[^"]*"\s*,' \
  --include='*.go' internal/ 2>/dev/null \
  | grep -vE 'SafeSenderID|safe_sender_id|safe_bot_id|safe_operator_id' \
  | filter_allowed || true)
# c3:跨行 metric Labels map literal —— 只抓 `Labels: map[string]any{...}` 上下文,
# 防 user_cache.go 缺口 4(2026-04-26 漏抓)那种 metric label 直接塞 raw open_id。
# 收紧 scope 到真 metric 上下文,避免误抓 audit Target / tool schema / case 分支。
# 兼容 BSD/macOS:用 perl(macOS grep 不带 PCRE 多行)。
_pii_c3_files=$(find internal -type f -name '*.go' ! -name '*_test.go' \
  ! -path 'internal/imctx/safe_sender.go' 2>/dev/null)
# 用 m!! 而非 m{} 作为 perl regex delimiter:character class 里的 } 跟 m{} 冲突
hits_c3=$(echo "$_pii_c3_files" | xargs -I{} perl -0777 -ne '
  while (m!Labels:\s*map\[string\]any\{([^}]*)\}!gs) {
    my $block = $1;
    my $block_start = $-[0];
    while ($block =~ m!"(open_id|sender_id|union_id)"\s*:\s*([A-Za-z_]\w*(?:\.\w+)*)!g) {
      my ($key, $val) = ($1, $2);
      next if $val =~ /^Safe/i;
      next if $val =~ /TenantKeyHash/;
      my $abs_offset = $block_start + $-[0];
      my $line = (substr($_, 0, $abs_offset) =~ tr/\n/\n/) + 1;
      print "$ARGV:$line:    Labels{...\"$key\": $val ...}\n";
    }
  }
' {} 2>/dev/null | filter_allowed || true)

hits_c="$(printf '%s\n%s\n%s' "$hits_c1" "$hits_c2" "$hits_c3" | sed '/^$/d')"
if [ -n "$hits_c" ]; then
  echo "FAIL: 以下 metric/trace 标签把 raw open_id 用作 label value："
  echo "$hits_c" | sed 's/^/  - /'
  violations=$((violations+1))
else
  echo "PASS: 无 metric/trace label 泄露(含跨行 map literal)"
fi

echo
if [ "$violations" -eq 0 ]; then
  echo "ALL_PASS"
  exit 0
else
  echo "FAILED ($violations gates)"
  exit 1
fi
