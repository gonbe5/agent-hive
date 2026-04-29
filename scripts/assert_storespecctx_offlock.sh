#!/usr/bin/env bash
# Sprint 3.1 (harden-spec-driven-phase2, task 12.15) — Guard 4 OffLock discipline
#
# runbook §3 Guard 4：`StoreSpecCtx` 是 atomic.Pointer 语义，其调用点**禁止**
# 出现在 `session.mu.Lock()` 持锁范围内——持锁内 atomic 写没意义、且违反
# spec-driven Phase 2 的 OffLock 纪律。违规 pattern：
#
#   s.mu.Lock()
#   ...
#   session.StoreSpecCtx(...)   // ← 违规
#   ...
#   s.mu.Unlock()
#
# 扫描策略（per-file 状态机，多行敏感）：
#   - awk 按行扫 internal/master/ 下所有 *.go（排除 _test.go）
#   - 跟踪 lock_depth：`s.mu.Lock()` / `session.mu.Lock()` +1；Unlock() -1
#   - 若遇 `StoreSpecCtx(` 且 lock_depth > 0 → 记录违规行，exit 1
#   - 注释行（// 开头）跳过，避免把 runbook 引文误判
#
# 兜底：`StoreSpecCtxGuarded` 是授权版，本脚本只管 raw `StoreSpecCtx`。
# Definition site（`func (s *SessionState) StoreSpecCtx(...)`) 明显不算调用
# 点，grep `StoreSpecCtx(` 配合函数定义模式可区分。

set -euo pipefail

target_dir="internal/master"

if [[ ! -d "$target_dir" ]]; then
  echo "target dir not found: $target_dir" >&2
  exit 1
fi

violations=$(awk '
FNR == 1 {
  file = FILENAME
  lock_depth = 0
}

# 跳过注释行（简化：单行注释开头），避免 runbook/doc 引文误判
/^[[:space:]]*\/\// { next }

# 跟踪锁深度：s.mu.Lock() / session.mu.Lock()
/\.mu\.Lock\(\)/ { lock_depth++ }
/\.mu\.Unlock\(\)/ { if (lock_depth > 0) lock_depth-- }

# 锁内出现 StoreSpecCtx( 调用（不含 Guarded 版、不含定义）
/StoreSpecCtx\(/ && !/StoreSpecCtxGuarded\(/ && !/func[[:space:]]+\(/ {
  if (lock_depth > 0) {
    printf "%s:%d: StoreSpecCtx called inside lock (lock_depth=%d)\n", file, FNR, lock_depth
  }
}
' "$target_dir"/*.go 2>/dev/null || true)

if [[ -n "$violations" ]]; then
  echo "Guard 4 OffLock discipline violated — StoreSpecCtx must NOT be called inside session.mu.Lock():" >&2
  echo "$violations" >&2
  exit 1
fi

# 正面锚点：至少有一条 StoreSpecCtx 调用（否则 Guard 4 扫描是真空，等于假门）
call_sites=$(grep -rn "StoreSpecCtx(" "$target_dir"/*.go \
  | grep -v _test.go \
  | grep -v 'func ' \
  | grep -v 'StoreSpecCtxGuarded' \
  | grep -v '^[[:space:]]*\/\/' \
  || true)

if [[ -z "$call_sites" ]]; then
  echo "Guard 4 anchor: no StoreSpecCtx() call sites found — scan is vacuous, treating as fail-closed" >&2
  exit 1
fi

echo "Guard 4 OffLock OK — StoreSpecCtx call sites (all outside locks):"
echo "$call_sites"
