#!/usr/bin/env bash
# Sprint 3.1 (harden-spec-driven-phase2, task 12.15) — anchor physical-kill verifier
#
# 用法：assert_anchors_pass.sh <testlog> <anchor-test-name> [<anchor-test-name>...]
#
# 为什么需要这个脚本：
#   rollback-drill CI job 的核心 signal 是"runbook anchor 不能被悄悄删除"。
#   裸 `go test -run ...` 对"test 被改名或注释掉"不敏感——Go 的 -run 找不到
#   匹配 test 时会 "PASS [no tests]" 然后 exit 0，看起来绿但其实没跑。
#
#   本脚本的 kill 机制：
#     1. `=== RUN   <TestName>` 必须出现（证明 test 被识别）
#     2. `--- PASS: <TestName>` 必须出现（证明 test 真的过了）
#     3. `--- SKIP:` 任一出现 → fail（SKIP→RED，防 TEST_DATABASE_URL 丢失）
#     4. `--- FAIL:` 任一出现 → fail（兜底，理论上 go test 已非零退出）
#
#   任一 anchor 失守 → 本脚本 exit 1 → step 红 → job 红 → block merge。
#
# 蓝军 mutation 验证（Sprint 3.1 R1）：
#   把任一 anchor test 临时改名（如 TestApplySpecDrivenIntake_LegacyMode_ShortCircuits
#   → TestApplySpecDrivenIntake_LegacyMode_ShortCircuitsRENAMED），重跑本脚本应红。

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <testlog> <anchor1> [<anchor2>...]" >&2
  exit 2
fi

log="$1"; shift

if [[ ! -f "$log" ]]; then
  echo "testlog not found: $log" >&2
  exit 1
fi

# SKIP→RED：任何被静默跳过的 anchor 都视作 gate 失守。
if grep -qE '^--- SKIP:' "$log"; then
  echo "SKIP→RED: anchors skipped in $log:" >&2
  grep -E '^--- SKIP:' "$log" >&2
  exit 1
fi

# FAIL 兜底：go test 非零退出通常已先挡住，但日志里若混进 FAIL 就显式 red。
if grep -qE '^--- FAIL:' "$log"; then
  echo "FAIL in $log:" >&2
  grep -E '^--- FAIL:' "$log" >&2
  exit 1
fi

missing=()
for anchor in "$@"; do
  # 两条必须成对：RUN 证明被识别，PASS 证明真的跑通。
  # 只认以 `  ` 结尾或 `(` 结尾的精确匹配，防子串误中（例如 Foo 匹配到 FooExtended）。
  if ! grep -qE "^=== RUN[[:space:]]+${anchor}\$" "$log" \
    && ! grep -qE "^=== RUN[[:space:]]+${anchor}/" "$log"; then
    missing+=("${anchor} (missing === RUN)")
    continue
  fi
  if ! grep -qE "^--- PASS: ${anchor} " "$log"; then
    missing+=("${anchor} (missing --- PASS:)")
  fi
done

if [[ ${#missing[@]} -gt 0 ]]; then
  echo "anchor physical-kill failed in $log:" >&2
  for m in "${missing[@]}"; do
    echo "  - $m" >&2
  done
  echo "" >&2
  echo "=== testlog tail (last 40 lines) ===" >&2
  tail -n 40 "$log" >&2
  exit 1
fi

echo "all ${#@} anchor(s) verified in $log: $*"
