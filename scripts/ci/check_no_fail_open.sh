#!/usr/bin/env bash
# check_no_fail_open.sh — Phase 0 P0-#13 CI gate
#
# 决议：dedup 必须 fail-closed（短超时 + 让飞书重投），禁止任何 config 后门把
# fail-open 重新打开。详见：
#   - docs/渠道对接/feishu-bot/ROADMAP.md     (Phase 0 行 #13)
#   - docs/渠道对接/feishu-bot/09-reliability.md §2.4 / §7
#   - docs/渠道对接/feishu-bot/reviews/M9-redteam.md P0-M9-05
#
# 红队场景：
#   1. 运维误开 fail-open → dedup 故障双发 → LLM token 双扣 + HITL 副作用双触
#   2. 字段保留为 deprecated alias → 配置漂移：故障期望与实际行为不一致
#
# 防御策略：源码 + 配置 example 都不允许出现以下符号（任意大小写形态）：
#   - FailOpenOnDedupError       (Go 字段 PascalCase)
#   - fail_open_on_dedup         (JSON / yaml snake_case 标签)
#   - failOpenOnDedup            (JSON camelCase 兼容形态)
#
# docs/ 路径排除——ROADMAP / 红队 / 09-reliability 的"已删除"说明必然 cite 字面量。
# scripts/ci/ 自身排除——本脚本要 cite 自己要禁的符号。
#
# 退出码：0=clean，1=违例
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT"

violations=0

scan() {
  local label="$1" pattern="$2"
  local hits
  # 扫描范围：源码 (internal/, cmd/) + 仓库根 example 配置 (config*.json)
  # 排除：docs/ (allowed cite)，scripts/ci/ (本脚本自身)，frontend/ (前端 dist 构建产物)
  hits=$(grep -rEn "$pattern" \
    --include='*.go' \
    --include='*.json' \
    --include='*.yaml' \
    --include='*.yml' \
    internal/ cmd/ config.example.json config.json config.test.json 2>/dev/null || true)
  if [ -n "$hits" ]; then
    echo "FAIL: dedup fail-open knob [$label] reintroduced:"
    echo "$hits" | sed 's/^/  - /'
    violations=$((violations+1))
  else
    echo "PASS: $label not found in source / example config"
  fi
}

echo "== gate: forbid dedup fail-open config knob =="
scan "FailOpenOnDedupError (Go field)"        'FailOpenOnDedupError'
scan "fail_open_on_dedup (snake_case tag)"    'fail_open_on_dedup'
scan "failOpenOnDedup (camelCase tag)"        'failOpenOnDedup'

echo
if [ "$violations" -eq 0 ]; then
  echo "ALL_PASS"
  exit 0
else
  echo "FAILED ($violations gates)"
  exit 1
fi
