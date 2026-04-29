#!/usr/bin/env bash
# check_feishu_sdk_only.sh — 飞书 SDK-only 红线 gate
#
# 红线(memory: project_feishu_sdk_only.md):飞书集成必须统一走 larksuite/oapi-sdk-go/v3
# 的 typed builder。直接调 SDK 内置的 RawRequest 入口(c.larkClient.Get/Post/Put/Delete/Patch)
# 仅在飞书 SDK 没暴露 typed builder 时允许,且必须显式声明:
#
#   // SDK-RAWREQUEST-ALLOWED: <理由,说明 SDK 哪个版本/路径未暴露>
#   apiResp, err := c.larkClient.Get(ctx, "/open-apis/...", ...)
#
# 红队场景:
#   - 开发偷懒不查 SDK builder,直接拼 URL → 后续 SDK 升级时漏迁移
#   - 第三方 wrapper 包绕过 SDK auth/retry 机制 → 验签 / token 刷新失效
#
# 扫描范围:
#   - internal/channel/feishu/*.go(主战场)
#   - internal/tools/feishu_tools.go
#   - 排除 _test.go(测试 mock 路径不需 SDK)
#
# 退出码:0=clean,1=违例,2=env 错误。
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT"

if [ ! -d "internal/channel/feishu" ]; then
  echo "ERR: internal/channel/feishu missing" >&2
  exit 2
fi

violations=0

echo "== gate: 飞书 SDK-only 红线(c.larkClient.{Get/Post/Put/Delete/Patch} 必须有 SDK-RAWREQUEST-ALLOWED 注释)=="

candidates=$(grep -rnE 'c\.larkClient\.(Get|Post|Put|Delete|Patch)\(' \
  --include='*.go' \
  internal/channel/feishu/ internal/tools/ 2>/dev/null \
  | grep -v '_test\.go:' || true)

if [ -z "$candidates" ]; then
  echo "PASS: 无 c.larkClient RawRequest 调用"
else
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    file=$(echo "$line" | cut -d: -f1)
    lineno=$(echo "$line" | cut -d: -f2)
    prev_lineno=$((lineno - 1))
    if [ "$prev_lineno" -lt 1 ]; then prev_lineno=1; fi
    prev_line=$(sed -n "${prev_lineno}p" "$file")
    if echo "$prev_line" | grep -q "SDK-RAWREQUEST-ALLOWED"; then
      continue
    fi
    if echo "$line" | grep -q "SDK-RAWREQUEST-ALLOWED"; then
      continue
    fi
    echo "FAIL: 飞书 SDK-only 红线违规(无 SDK-RAWREQUEST-ALLOWED 注释)"
    echo "  - $line"
    violations=$((violations+1))
  done <<< "$candidates"
fi

echo
echo "== gate: 禁止 net/http 直调飞书 open API =="
http_hits=$(grep -rnE 'http\.(Get|Post|NewRequest)\(.*open\.(feishu\.cn|larksuite\.com)' \
  --include='*.go' \
  internal/ 2>/dev/null \
  | grep -v '_test\.go:' || true)
if [ -n "$http_hits" ]; then
  echo "FAIL: 发现 net/http 直调 open.feishu.cn / open.larksuite.com:"
  echo "$http_hits" | sed 's/^/  - /'
  violations=$((violations+1))
else
  echo "PASS: 无 net/http 直调"
fi

echo
if [ "$violations" -eq 0 ]; then
  echo "ALL_PASS"
  exit 0
else
  echo "FAILED ($violations violations)"
  exit 1
fi
