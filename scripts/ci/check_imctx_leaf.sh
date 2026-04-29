#!/usr/bin/env bash
# check_imctx_leaf.sh — Phase 0 P0-#1 CI gate
#
# 核心断言：
#   (a) internal/imctx 必须是叶子包 —— 不得 import 任何 internal/<其它包>
#   (b) 全树（除 imctx 自己）禁止出现 `"im-feishu-` 字面量自拼 session_id
#       —— P0-#10 唯一入口约束（必须经 imctx.BuildSessionID）
#
# 退出码：0=clean，1=发现违例，2=env 错误
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
PKG="github.com/chef-guo/agents-hive/internal/imctx"

cd "$ROOT"

if [ ! -d "internal/imctx" ]; then
  echo "ERR: internal/imctx missing" >&2
  exit 2
fi

violations=0

# (a) 叶子包断言
echo "== gate (a): internal/imctx leaf-only =="
deps=$(go list -deps ./internal/imctx/... 2>/dev/null \
  | grep 'chef-guo/agents-hive/internal/' \
  | grep -v "^${PKG}$" || true)
if [ -n "$deps" ]; then
  echo "FAIL: internal/imctx must depend on stdlib only, but found:"
  echo "$deps" | sed 's/^/  - /'
  violations=$((violations+1))
else
  echo "PASS: zero internal/* deps"
fi

# (b) session_id 字面量唯一入口
echo
echo "== gate (b): no inline 'im-feishu-' literals outside imctx =="
# 排除：imctx 自身、_test.go 内的断言、docs 与脚本
inline=$(grep -rn '"im-feishu-' \
  --include='*.go' \
  --exclude-dir='imctx' \
  internal/ 2>/dev/null | grep -v '_test\.go:' || true)
if [ -n "$inline" ]; then
  echo "FAIL: session_id literal must go through imctx.BuildSessionID:"
  echo "$inline" | sed 's/^/  - /'
  violations=$((violations+1))
else
  echo "PASS: no inline session_id literals"
fi

echo
if [ "$violations" -eq 0 ]; then
  echo "ALL_PASS"
  exit 0
else
  echo "FAILED ($violations gates)"
  exit 1
fi
