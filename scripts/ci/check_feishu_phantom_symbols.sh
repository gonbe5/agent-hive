#!/usr/bin/env bash
# check_feishu_phantom_symbols.sh — Phase 0 P0-#3 CI gate
#
# 红队复核（reviews/M9-redteam.md）确认以下符号在 larksuite/oapi-sdk-go/v3 v3.5.3
# 中并不存在或不属于飞书 webhook/longconn 路径，但历史文档曾误引用。本 gate 防止
# 这些"幽灵符号"重新出现在源码：
#
#   - WithStatusChangeHandler  ：larkws.NewClient 没有此 option（仅有 5 个有效 option）
#   - isEncrypted              ：仅在 apaas/v1/model.go 出现，与飞书 webhook 无关
#   - Im.ChatManagers.List     ：飞书 SDK 仅暴露 AddManagers/DeleteManagers（resource.go:473/501）
#
# docs/ 路径排除——红队 / ROADMAP 的"勘误说明"必然要 cite 这些符号本身。
#
# 退出码：0=clean，1=违例，2=env 错误
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT"

violations=0

scan() {
  local label="$1" pattern="$2"
  local hits
  hits=$(grep -rn "$pattern" \
    --include='*.go' \
    internal/ cmd/ 2>/dev/null || true)
  if [ -n "$hits" ]; then
    echo "FAIL: phantom SDK symbol [$label] reintroduced:"
    echo "$hits" | sed 's/^/  - /'
    violations=$((violations+1))
  else
    echo "PASS: $label not found in source"
  fi
}

echo "== gate: forbid phantom Feishu SDK symbols =="
scan "WithStatusChangeHandler" "WithStatusChangeHandler"
scan "isEncrypted (in feishu paths)" "isEncrypted"
scan "Im.ChatManagers.List" "ChatManagers\.List\b"

echo
if [ "$violations" -eq 0 ]; then
  echo "ALL_PASS"
  exit 0
else
  echo "FAILED ($violations gates)"
  exit 1
fi
