#!/usr/bin/env bash
# check_session_scope.sh — session-scope-regression-matrix Phase 0 Spike A v0
#
# 核心断言：internal/master/*.go 中禁止出现无白名单注释的
#   (a) eventBus.Broadcast(BroadcastMessage{...})          — spec R-1
#   (b) BroadcastGenericMessage(EventType(AgentProgress|   — spec R-2（窄化）
#        ToolCall|SkillInstallProgress)...)
# 白名单机制：仅接受 `// no session scope by design` 单一注释（spec R-1b / design D1）
# 退出码：0=clean，1=发现违例，2=env 错误
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
SCAN_DIR="$ROOT/internal/master"
MARKER='// no session scope by design'

if [ ! -d "$SCAN_DIR" ]; then
  echo "ERR: scan dir missing: $SCAN_DIR" >&2
  exit 2
fi

violations=0

# 扫描 pattern：返回 `file:line:match` 三元组；白名单检查看 line-1
scan_pattern() {
  local label="$1" pattern="$2"
  # grep -nE 输出 file:line:content；--include 限定 .go
  while IFS= read -r hit; do
    [ -z "$hit" ] && continue
    local file="${hit%%:*}"
    local rest="${hit#*:}"
    local line="${rest%%:*}"
    local prev=$((line - 1))
    # 读取前一行；若含 marker 则豁免
    if [ "$prev" -ge 1 ] && sed -n "${prev}p" "$file" | grep -qF "$MARKER"; then
      continue
    fi
    echo "  VIOLATION [$label] $file:$line" >&2
    violations=$((violations + 1))
  done < <(grep -rnE "$pattern" "$SCAN_DIR" --include='*.go' 2>/dev/null || true)
}

# R-1: 裸 Broadcast(BroadcastMessage...)
scan_pattern "R-1" 'eventBus\.Broadcast\(BroadcastMessage'

# R-2: BroadcastGenericMessage 用于 session-scoped progress 事件（窄化枚举，不误伤 Created/Destroyed/ListChanged 元数据事件）
scan_pattern "R-2" 'BroadcastGenericMessage\(EventType(AgentProgress|ToolCall|SkillInstallProgress)'

if [ "$violations" -gt 0 ]; then
  echo "FAIL: $violations session-scope violation(s) found — add \`${MARKER}\` justification comment on the line above, or migrate to BroadcastSessionMessage" >&2
  exit 1
fi

echo "OK: internal/master/ session-scope contract clean ($(grep -rlE 'eventBus\.Broadcast' "$SCAN_DIR" --include='*.go' 2>/dev/null | wc -l | tr -d ' ') files scanned)"
exit 0
