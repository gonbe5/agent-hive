#!/usr/bin/env bash
# check_session_id_unique_entry.sh — Phase 0 P0-#10 CI gate
#
# 核心断言：
#   imctx 包之外，任何位置出现 IM session_id 字面量自拼（"im-feishu-..." 等）
#   一律视为违规。session_id 必须经 imctx.BuildSessionID 唯一入口构造。
#
# 红队场景：不同入口拼出不同格式 → journal 主键漂移 → 上下文丢失。
#
# 扫描范围：
#   - 限定 internal/、cmd/ 目录
#   - 排除 _test.go（测试断言/fixture 允许出现期望字符串）
#   - 排除 internal/imctx 自身（owner）
#
# 退出码：0=clean，1=发现违例，2=env 错误。
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT"

if [ ! -d "internal/imctx" ]; then
  echo "ERR: internal/imctx missing" >&2
  exit 2
fi

# 已支持平台前缀枚举。新增平台时同步追加，避免漏扫。
# 第一组：字面量自拼（fmt 之外硬编码）
patterns=(
  '"im-feishu-'
  '"im-wechat-'
  '"im-dingtalk-'
  '"im-wecom-'
)

# 第二组：动态拼（fmt.Sprintf / Printf 系列 / + 拼接 / strings.Join）
# 这些用 grep -E 扫，专门抓"绕过 imctx.BuildSessionID 唯一入口"的动态构造。
# 红队场景:`fmt.Sprintf("im-%s-...")` 通过第一组 grep 但绕过 BuildSessionID。
dynamic_patterns=(
  'fmt\.Sprintf\(\s*"im-'
  'fmt\.Fprintf\([^,]+,\s*"im-'
  'fmt\.Printf\(\s*"im-'
  '"im-%s-'
  '"im-"\s*\+'
)

violations=0

for dir in internal cmd; do
  if [ ! -d "$dir" ]; then
    continue
  fi
  # 第一组：字面量
  for pat in "${patterns[@]}"; do
    matches=$(grep -rn "$pat" \
      --include='*.go' \
      --exclude-dir='imctx' \
      "$dir" 2>/dev/null | grep -v '_test\.go:' || true)
    if [ -n "$matches" ]; then
      echo "FAIL: 发现 session_id 字面量自拼 (pattern=$pat) — 必须改用 imctx.BuildSessionID："
      echo "$matches" | sed 's/^/  - /'
      violations=$((violations+1))
    fi
  done
  # 第二组：动态拼接（fmt.Sprintf 等）
  for pat in "${dynamic_patterns[@]}"; do
    matches=$(grep -rnE "$pat" \
      --include='*.go' \
      --exclude-dir='imctx' \
      "$dir" 2>/dev/null | grep -v '_test\.go:' || true)
    if [ -n "$matches" ]; then
      echo "FAIL: 发现 session_id 动态拼接 (pattern=$pat) — 必须改用 imctx.BuildSessionID："
      echo "$matches" | sed 's/^/  - /'
      violations=$((violations+1))
    fi
  done
done

echo
if [ "$violations" -eq 0 ]; then
  echo "ALL_PASS: no inline session_id literals outside imctx"
  exit 0
else
  echo "FAILED ($violations pattern(s) violated)"
  exit 1
fi
