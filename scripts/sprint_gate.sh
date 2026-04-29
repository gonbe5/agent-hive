#!/usr/bin/env bash
# sprint_gate.sh — Sprint 切换物理验收脚本（harden-spec-driven-phase2 Round 4 产出）
#
# 背景：
#   Round 4 三路评审（CEO / Codex / P9）共识：Sprint 切换不再信 self-report，
#   改走物理闸门。`Sprint 1 → 2`、`Sprint 2 → 3` 起步前必须跑本脚本，全部
#   assertion 绿才算准入。
#
# 用法：
#   scripts/sprint_gate.sh                # 默认 Sprint 1 → 2 准入（4 条 assertion）
#   scripts/sprint_gate.sh --sprint=2     # Sprint 2 → 3 准入（扩 6 条 assertion）
#
# 输出：
#   全过 → "GATE: PASS"；任一失败 → "GATE: FAIL: <具体条目>"。
#
# 设计原则：
#   - 纯物理证据（git log / gh run list / coverage file grep）
#   - 每条 assertion 独立可复现；失败时打印诊断信息不直接退出，跑完所有后再判
#   - 本地 cache（`ls coverage-specdriven.out` 是否存在）允许无 gh CLI 时也能
#     起到部分 gate 作用；gh CLI 缺失时只 skip network-dependent assertion 并告警
#
# 退出码：
#   0 = PASS（全绿）；1 = FAIL（任一红）；2 = 使用错误 / 依赖缺失
set -uo pipefail

# 禁用 -e：我们手动收集每条 assertion 结果，不能因第一条 fail 就退。
set +e

sprint=1
for arg in "$@"; do
  case "$arg" in
    --sprint=1) sprint=1 ;;
    --sprint=2) sprint=2 ;;
    --help|-h)
      grep -E '^#( |$)' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown arg: $arg" >&2
      echo "usage: $0 [--sprint=1|--sprint=2]" >&2
      exit 2
      ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# ─── Precondition: CWD 必须是 git 仓库 ─────────────────────────────────────
# 红线：本脚本的核心 assertion 依赖 `git log main` 和 `gh run list`，二者都要求
# CWD 是一个真正的 git 工作目录。Claude session 或 CI worker 如果在 snapshot /
# flat-copy / sandbox 里跑，`.git` 不存在 → 所有 git 断言永久 FAIL = 假门。
# 早期版本默默让 `git log` 2>/dev/null 吞掉 fatal，然后 assertion 以 "git log failed"
# 的方式 record FAIL，误导使用者以为"commit marker 没加"，实际是环境错了。
# 这里提前硬 fail，把 gate 的使用边界讲清楚：必须在真 repo 里跑。
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  cat >&2 <<'EOF'
sprint_gate.sh: CWD 不是 git 仓库

此脚本的 assertion 1 (git log main) 和 assertion 2 (gh run list) 都依赖真正的
git 工作目录。当前 CWD 没有 .git，这通常意味着你在：
  - Claude session snapshot（代码只是 flat-copy，不是 worktree）
  - CI runner 但没 checkout
  - 错误的目录

正确用法：
  1. 把本地修改 port 回真正的 agents-hive 仓库
  2. commit + push 含 "1.17" / "12.10" / "12.12" 三条 marker
  3. 等 CI workflow `test-specdriven` 跑绿
  4. 在那个真 repo 的 worktree 里再跑 `./scripts/sprint_gate.sh`

GATE: FAIL: precondition (CWD not a git repository)
EOF
  exit 2
fi

declare -a results  # "PASS|FAIL|SKIP:name:detail"
declare -i pass=0 fail=0 skip=0

record() {
  local status="$1" name="$2" detail="$3"
  results+=("${status}|${name}|${detail}")
  case "$status" in
    PASS) pass+=1 ;;
    FAIL) fail+=1 ;;
    SKIP) skip+=1 ;;
  esac
}

# ─── Assertion 1: commit trace ──────────────────────────────────────────
# 要求目标分支（默认 main，可通过 SPRINT_GATE_BRANCH 覆盖）近 20 commit 含
# 1.17（Sprint 1.1 R5-1）、12.10（Sprint 1.2 commit a）、12.12（Sprint 1.2 commit b）
# 三条 marker。用正则宽松匹配 PR title / commit msg。
#
# 纸老虎 #7 修复：早期版本 `git log main || git log HEAD` 静默 fallback 到 HEAD——
# feature 分支上 HEAD 凑巧有 marker 时 assertion 错误 PASS，但 main 到底 merge 没有
# 完全无保障。现在改为：显式检查目标分支（refs/heads 或 refs/remotes/origin）
# 是否存在；不存在则 loud fail 并提示 SPRINT_GATE_BRANCH 覆盖，杜绝"分支不存在
# 时 assertion 靠 HEAD 蒙混"的纸老虎。
branch_ref="${SPRINT_GATE_BRANCH:-main}"
name="${branch_ref} branch has Sprint 1 commits (1.17 / 12.10 / 12.12)"
log=""
if git show-ref --verify --quiet "refs/heads/${branch_ref}" 2>/dev/null; then
  log=$(git log --oneline "${branch_ref}" -20 2>/dev/null)
elif git show-ref --verify --quiet "refs/remotes/origin/${branch_ref}" 2>/dev/null; then
  log=$(git log --oneline "origin/${branch_ref}" -20 2>/dev/null)
fi

if [[ -z "$log" ]]; then
  record FAIL "$name" "branch '${branch_ref}' not found locally or at origin (override with SPRINT_GATE_BRANCH=<name> if your default differs)"
else
  missing=()
  for marker in "1\.17" "12\.10" "12\.12"; do
    if ! grep -qE "$marker" <<<"$log"; then
      missing+=("$marker")
    fi
  done
  if [[ ${#missing[@]} -eq 0 ]]; then
    record PASS "$name" "all 3 markers found in last 20 commits on ${branch_ref}"
  else
    record FAIL "$name" "missing markers on ${branch_ref}: ${missing[*]}"
  fi
fi

# ─── Assertion 2: latest CI run green ───────────────────────────────────
name="test-specdriven workflow latest run on main == success"
if ! command -v gh >/dev/null 2>&1; then
  record SKIP "$name" "gh CLI not installed — install with 'brew install gh' and re-run"
else
  # --json 保证 machine-readable；--limit=1 只看最新一次
  status=$(gh run list --workflow=test-specdriven.yml --branch=main --limit=1 \
    --json conclusion --jq '.[0].conclusion' 2>/dev/null || echo "")
  if [[ "$status" == "success" ]]; then
    record PASS "$name" "latest conclusion = success"
  elif [[ -z "$status" ]]; then
    record FAIL "$name" "no runs found — workflow not triggered yet or gh auth missing"
  else
    record FAIL "$name" "latest conclusion = $status"
  fi
fi

# ─── Assertion 3: store spec_store.go has NON-ZERO coverage ─────────────
# 红线（blue army 发现的纸老虎）：只查 "profile 里有文件名" 过不了关——
# -coverpkg=store 生效后即使没 test 跑过 store 代码，profile 也会列 0-count 行。
# 真正的 gate 是累加 count > 0，证明至少一条语句被真实执行过。
name="internal/store/spec_store.go coverage count > 0 (non-zero executions)"
profile="coverage-specdriven.out"
if [[ ! -f "$profile" ]]; then
  record FAIL "$name" "coverage profile missing — run 'make test-specdriven' first"
else
  store_count=$(awk -F'[ \t]+' '
    /internal\/store\/spec_store\.go/ { sum += $NF }
    END { print (sum ? sum : 0) }
  ' "$profile")
  if [[ "${store_count:-0}" -gt 0 ]]; then
    record PASS "$name" "count=$store_count (>0)"
  else
    record FAIL "$name" "count=0 — store tests not running (check test pkg list + TEST_DATABASE_URL)"
  fi
fi

# ─── Assertion 4: total coverage ≥ 80% ──────────────────────────────────
# Sprint 1.2 蓝军自检红线（denominator 稀释陷阱，与 check_specdriven_coverage.sh 同源修复）：
# filter 必须 narrow 到 `specdriven/` + `store/spec_` prefix——早期通配 `store/`
# 把 5 个非 spec 相关 0% 文件（memory_store/postgres/prompt_store/seed/skill_store）
# 吞进分母，真实 88.6% 被稀释到 36.5%，阈值怎么设都过不了 = 假门。
# Sprint 1 阈值 80%：实测 narrowed 作用域下 spec 全家覆盖率 ~88%，80% 是有效门槛。
name="total coverage (specdriven + store/spec_* narrowed) >= 80%"
if [[ ! -f "$profile" ]]; then
  record FAIL "$name" "coverage profile missing"
elif ! command -v go >/dev/null 2>&1; then
  record SKIP "$name" "go toolchain not in PATH"
else
  # 动态解析 module path（fail-open 到硬编码默认值）——fork / rename 场景下
  # 硬编码 `github.com/chef-guo/agents-hive` 会让 grep 全 miss → tmp_profile 只剩
  # mode 行 → go tool cover 输出 0% → loud fail（错误信息误导为"阈值不过"而非
  # "module path 对不上"）。用 `go list -m` 兜底到 go.mod 声明。
  if module_path=$(go list -m 2>/dev/null); then :; else module_path="github.com/chef-guo/agents-hive"; fi
  module_re="${module_path//./\\.}"  # regex 中点号需转义
  tmp_profile=$(mktemp)
  {
    head -n 1 "$profile"
    grep -E "^${module_re}/internal/(specdriven/|store/spec_)" "$profile" || true
  } > "$tmp_profile"
  pct=$(go tool cover -func="$tmp_profile" 2>/dev/null \
    | awk '/^total:/ { gsub("%", "", $NF); print $NF; exit }')
  rm -f "$tmp_profile"
  if [[ -z "$pct" ]]; then
    record FAIL "$name" "go tool cover could not parse total:"
  elif awk -v p="$pct" 'BEGIN { exit (p+0 < 80) ? 1 : 0 }'; then
    record PASS "$name" "total=${pct}% (>= 80)"
  else
    record FAIL "$name" "total=${pct}% (< 80)"
  fi
fi

# ─── Sprint 2 额外 assertion（Sprint 2 → 3 准入） ───────────────────────
if [[ "$sprint" -ge 2 ]]; then
  # Assertion 5: echo-back 对照实验证据
  name="echo-back counterexample drill evidence in runbook"
  if [[ -f "docs/运维手册/spec-driven-rollout.md" ]] && \
     grep -qE 'echo-back|反例验证' docs/运维手册/spec-driven-rollout.md; then
    record PASS "$name" "evidence link found in runbook"
  else
    record FAIL "$name" "runbook missing echo-back drill evidence section"
  fi

  # Assertion 6: metrics.go 锚点完整（Sprint 2.3 蓝军 R5 退役了原 `enqueueMetric.*specdriven\. ≥ 6`
  # 判据——5 个 counter 的 prod call site 要等 Sprint 3.3.b Runner 落地才出生，
  # 在 Sprint 2 gate 阶段刷这个数 = 纸老虎。改验"锚点三件套"存在且齐全：
  #   (a) 6 个 Metric*Total 常量
  #   (b) AllowedCASConflictScenarios 白名单（3 条 enum）
  #   (c) AllowedPlanFallbackReasons 白名单（4 条 enum）
  # 真实 enqueueMetric 数量验证移到 Sprint 3 gate（见 tasks.md Sprint 3.3.b DONE）。
  name="internal/specdriven/metrics.go anchor: 6 Metric*Total + 2 Allowed* lists"
  anchor="internal/specdriven/metrics.go"
  if [[ ! -f "$anchor" ]]; then
    record FAIL "$name" "anchor file missing: $anchor"
  else
    metric_consts=$(grep -cE 'Metric[A-Z][A-Za-z]+Total[[:space:]]*=' "$anchor" 2>/dev/null | tr -d ' ')
    allowed_cas=$(grep -cE '^var AllowedCASConflictScenarios' "$anchor" 2>/dev/null | tr -d ' ')
    allowed_fb=$(grep -cE '^var AllowedPlanFallbackReasons' "$anchor" 2>/dev/null | tr -d ' ')
    if [[ "${metric_consts:-0}" -ge 6 && "${allowed_cas:-0}" -ge 1 && "${allowed_fb:-0}" -ge 1 ]]; then
      record PASS "$name" "Metric*Total=$metric_consts, AllowedCAS=$allowed_cas, AllowedFallback=$allowed_fb"
    else
      record FAIL "$name" "Metric*Total=$metric_consts (want>=6), AllowedCAS=$allowed_cas, AllowedFallback=$allowed_fb (want>=1 each)"
    fi
  fi

  # Assertion 7: 行为逻辑函数覆盖率（Sprint 2.4 重构后的 per-function floor）
  #
  # 演进说明：原判据"RunFixtures ≥ 85%"在 Sprint 2.4 refactor 之后不再匹配语义——
  # RunFixtures 被抽成薄 orchestrator（h.preflight + loop + recordCaseResult +
  # terminalGate + t.Fatal 终止），其中 t.Fatal 分支被 Go testing 的父-子失败联动
  # 物理阻挡，无法在不 kill 父测试的前提下覆盖。
  #
  # 新判据：把"业务逻辑必须被单测打穿"锁在真正承载逻辑的纯函数上：
  #   - (*Summary).recordCaseResult  —— 三路分支（ok/required-fail/optional-fail）
  #   - (Harness).preflight          —— validate + required-set 完整性
  #   - (Summary).terminalGate       —— RequiredFailed 非空 → block rollout 错误构造
  # 每条 ≥ 95%；任何一条跌破 = Sprint 2.4 引入的抽象退化为新的 paper tiger。
  name="behavior-logic functions coverage >= 95% each"
  if [[ ! -f "$profile" ]]; then
    record FAIL "$name" "coverage profile missing"
  elif ! command -v go >/dev/null 2>&1; then
    record SKIP "$name" "go toolchain not in PATH"
  else
    fail_detail=""
    for fn in recordCaseResult preflight terminalGate; do
      pct=$(go tool cover -func="$profile" 2>/dev/null \
        | awk -v fn="$fn" '$2 == fn { gsub("%", "", $NF); print $NF; exit }')
      if [[ -z "$pct" ]]; then
        fail_detail+=" ${fn}=MISSING"
        continue
      fi
      if ! awk -v p="$pct" 'BEGIN { exit (p+0 < 95) ? 1 : 0 }'; then
        fail_detail+=" ${fn}=${pct}%(<95)"
      fi
    done
    if [[ -z "$fail_detail" ]]; then
      record PASS "$name" "recordCaseResult + preflight + terminalGate all >= 95%"
    else
      record FAIL "$name" "coverage gaps:$fail_detail"
    fi
  fi
fi

# ─── Verdict ────────────────────────────────────────────────────────────
echo ""
echo "──────────────────────────────────────────────────────────────"
echo "Sprint $sprint gate results"
echo "──────────────────────────────────────────────────────────────"
for line in "${results[@]}"; do
  status="${line%%|*}"
  rest="${line#*|}"
  name="${rest%%|*}"
  detail="${rest#*|}"
  printf "  [%4s] %s\n         → %s\n" "$status" "$name" "$detail"
done
echo ""
echo "  PASS=$pass  FAIL=$fail  SKIP=$skip"
echo "──────────────────────────────────────────────────────────────"

if [[ "$fail" -gt 0 ]]; then
  echo "GATE: FAIL: ${fail} assertion(s) red — blocking Sprint $((sprint + 1)) start"
  exit 1
fi

if [[ "$skip" -gt 0 ]]; then
  # SKIP 不阻塞，但要求操作者显式确认（例如 gh CLI 未装时在受控环境仍可放行）。
  echo "GATE: PASS (with $skip SKIP) — review SKIP reasons before starting Sprint $((sprint + 1))"
  exit 0
fi

echo "GATE: PASS"
exit 0
