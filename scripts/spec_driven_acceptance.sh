#!/usr/bin/env bash
# spec_driven_acceptance.sh — task 12.8 (a)/(b)/(d)/(e)/(g) 五项一键自检
# (c)/(f) 必须手动（造 CAS 冲突 / 物理关 PG），见 docs/运维手册/spec-driven-phase2-acceptance.md
#
# 用法（staging 节点上跑）：
#   export DATABASE_URL='postgres://hive:***@staging-pg:5432/hive_staging?sslmode=require'
#   export GITHUB_OWNER='your-org'
#   export GITHUB_REPO='agents-hive'
#   export STAGING_LOG=/var/log/hive/server.log
#   bash scripts/spec_driven_acceptance.sh
#
# 退出码：0=ALL PASS，1=任一 FAIL，2=env 缺失
set -euo pipefail

: "${DATABASE_URL:?must set — see runbook §0.1}"
: "${GITHUB_OWNER:?must set — github org}"
: "${GITHUB_REPO:?must set — repo name}"
: "${STAGING_LOG:?must set — path to hive server log}"

fail=0
pass=0
check() {
  local name="$1" cond="$2"
  if eval "$cond" >/dev/null 2>&1; then
    echo "  PASS  $name"
    pass=$((pass+1))
  else
    echo "  FAIL  $name"
    echo "        cmd: $cond"
    fail=$((fail+1))
  fi
}

echo "━━━ task 12.8 acceptance — $(date -u +%Y-%m-%dT%H:%M:%SZ) ━━━"

# ── (a) PG wired (no disabled warn in log) ─────────────────────────────
# 必须先校验 log 文件存在——否则 grep exit=2 被 ! 翻成 success 是纸老虎（蓝军 R-A）
check "(a) PG wired — no 'disabled — PG pool absent' in log" \
  '[ -r "$STAGING_LOG" ] && ! grep -q "spec_change_store disabled" "$STAGING_LOG"'

# ── (b) hive_spec_changes recent rows ──────────────────────────────────
check "(b) hive_spec_changes recent (≥1 row in last 10 min)" \
  '[ "$(psql "$DATABASE_URL" -At -c "SELECT count(*) FROM hive_spec_changes WHERE updated_at > now() - interval '\''10 min'\''")" -ge 1 ]'

# ── (d) fallback_rate ≤ 5% ─────────────────────────────────────────────
check "(d) fallback_rate ≤ 5% (plan_fallback_total / plan_total)" \
  '[ "$(psql "$DATABASE_URL" -At -c "SELECT (coalesce((SELECT sum(value) FROM hive_metrics WHERE name='\''specdriven.plan_fallback_total'\'' AND ts > now() - interval '\''30 min'\''), 0) / NULLIF((SELECT sum(value) FROM hive_metrics WHERE name='\''specdriven.plan_total'\'' AND ts > now() - interval '\''30 min'\''), 0)) <= 0.05")" = "t" ]'

# ── (e) dual ratio ≥ 10% ───────────────────────────────────────────────
check "(e) intake_decision dual ratio ≥ 10%" \
  '[ "$(psql "$DATABASE_URL" -At -c "WITH t AS (SELECT labels->>'\''decision'\'' d, sum(value) v FROM hive_metrics WHERE name='\''specdriven.intake_decision_total'\'' AND ts > now() - interval '\''30 min'\'' GROUP BY 1) SELECT (coalesce((SELECT v FROM t WHERE d='\''dual'\''), 0) / NULLIF(sum(v), 0)) >= 0.1 FROM t")" = "t" ]'

# ── (g) branch protection bound ────────────────────────────────────────
check "(g) branch protection: 'specdriven gate' in required contexts" \
  'gh api "repos/$GITHUB_OWNER/$GITHUB_REPO/branches/main/protection" --jq ".required_status_checks.contexts" | grep -q "specdriven gate"'

echo "━━━ summary: $pass PASS / $fail FAIL ━━━"
if [ "$fail" -eq 0 ]; then
  echo "  → (c) and (f) require manual execution (造 CAS 冲突 / 物理关 PG)"
  echo "  → see runbook §3 and §6"
  echo "  → after manual (c)/(f) green, sign-off table → archive"
  exit 0
else
  echo "  → 不准 promote — 按 spec-driven-rollback.md 降档到 mode=legacy"
  exit 1
fi
