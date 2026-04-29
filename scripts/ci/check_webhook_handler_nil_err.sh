#!/usr/bin/env bash
# check_webhook_handler_nil_err.sh — Phase 0 P0-#7 CI gate
#
# 决议：飞书 webhook handler 必须永远返回 nil。返回非 nil 会让 SDK 写 5xx 触发
# 飞书无限重试风暴（红队链 A），业务失败必须改走 retry_queue 兜底持久化。详见：
#   - docs/渠道对接/feishu-bot/ROADMAP.md     (Phase 0 行 #7)
#   - docs/渠道对接/feishu-bot/08-security.md
#   - docs/渠道对接/feishu-bot/reviews/M8-redteam.md P0-M8-02
#
# 红队场景：
#   1. 开发者在 handleMessageReceive 函数体内 `return err` / `return fmt.Errorf(...)`
#      → SDK 写回 5xx → 飞书秒级重投 → 业务双处理 / 日志风暴
#   2. panic 未 recover → 同样被 SDK 处理器转成 5xx
#   3. 嵌套 goroutine 里 panic 但没重入队列 → 消息永失
#
# 本脚本的 grep 级拦截（ROADMAP 第 97 行标注的 handlergate AST 实现推迟到 Phase 5）：
#   在 internal/channel/feishu/webhook.go 的 handleMessageReceive 函数体内扫描：
#     (a) 任何 `return err`、`return xxx.Err()` 、`return fmt.Errorf` 返回语句
#         （"return nil" 和 "return" 字面量允许）
#     (b) 函数签名必须是 `func (h *WebhookHandler) handleMessageReceive(...) error`
#         且此函数必须存在
#
# 扫描范围严格限定在 webhook.go 的 handleMessageReceive 函数体，不误伤 enqueueRetry
# 或其他辅助函数（它们可以返回 error）。
#
# 退出码：0=clean，1=违例，2=env/结构错误。
set -euo pipefail

ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT"

TARGET="internal/channel/feishu/webhook.go"

if [ ! -f "$TARGET" ]; then
  echo "ERR: $TARGET missing" >&2
  exit 2
fi

# 定位 handleMessageReceive 函数起止行号。
# 规则：起点是函数签名行（以 "func (h *WebhookHandler) handleMessageReceive" 开头）；
#       终点是接下来第一个纯 "}" 顶格（列 1）结束。
start_line=$(grep -n '^func (h \*WebhookHandler) handleMessageReceive' "$TARGET" | head -1 | cut -d: -f1 || true)

if [ -z "$start_line" ]; then
  echo "FAIL: 在 $TARGET 中找不到 handleMessageReceive 函数签名" >&2
  echo "  期望形如: func (h *WebhookHandler) handleMessageReceive(...) error" >&2
  exit 1
fi

# 从 start_line+1 开始找第一个列 1 的闭合 "}"
end_line=$(awk -v s="$start_line" 'NR>s && /^}[[:space:]]*$/ {print NR; exit}' "$TARGET")

if [ -z "$end_line" ]; then
  echo "FAIL: 无法定位 handleMessageReceive 函数的结束 } " >&2
  exit 2
fi

echo "== gate: handleMessageReceive (行 $start_line - $end_line) 必须永返 nil =="

# 函数体 = (start_line+1) .. (end_line-1)
body=$(sed -n "$((start_line+1)),$((end_line-1))p" "$TARGET")

# 忽略注释行与 "return nil" / 光 "return"
# 允许：return nil、return（bare），return 后带空白 + 注释
# 违规：return <anything that looks like an error value>
violations=$(printf '%s\n' "$body" \
  | grep -nE '^[[:space:]]*return[[:space:]]+' \
  | grep -vE 'return[[:space:]]+nil\b' \
  | grep -vE '^[[:space:]]*//' || true)

if [ -n "$violations" ]; then
  echo "FAIL: handleMessageReceive 函数体内发现返回非 nil 的语句："
  # 把匹配行的相对行号翻译回 webhook.go 的绝对行号
  printf '%s\n' "$violations" | while IFS= read -r line; do
    rel=$(printf '%s' "$line" | cut -d: -f1)
    txt=$(printf '%s' "$line" | cut -d: -f2-)
    abs=$((start_line + rel))
    echo "  - $TARGET:$abs:$txt"
  done
  exit 1
fi

echo "PASS: handleMessageReceive 函数体内无非 nil 返回路径"

# 额外兜底：函数签名的返回类型必须是 error（不是 (X, error) 或 nil）。
sig=$(sed -n "${start_line}p" "$TARGET")
if ! printf '%s' "$sig" | grep -qE '\)[[:space:]]+error[[:space:]]*\{?[[:space:]]*$'; then
  echo "WARN: handleMessageReceive 签名返回类型不是纯 error，请人工确认："
  echo "  $sig"
  # 不作为 fail：未来若签名迁移到无返回值或 (X, error)，gate 需要更新
fi

echo
echo "ALL_PASS"
exit 0
