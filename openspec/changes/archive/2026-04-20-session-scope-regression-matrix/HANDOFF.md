# HANDOFF — session-scope-regression-matrix → frontend-ws-handshake-regression

**Date**: 2026-04-20
**From**: session-scope-regression-matrix
**To**: frontend-ws-handshake-regression Phase 2 owner

## Ready-state 交接

本 change 的 CI harness（`.github/workflows/e2e-session-scope.yml`）已落地，其中 `browser` job 专门为 `frontend-ws-handshake-regression` Phase 2 playwright case 预留接缝：

- **目录**：`frontend/playwright/**/*.spec.ts`（workflow 硬约束：只引用目录 glob，绝不硬编码具体 spec 文件名）
- **空 spec 行为**：当 `frontend/playwright/` 无 `*.spec.ts` 文件时 browser job 走 `no-specs short-circuit` step，报 success，**不 block workflow**
- **有 spec 行为**：自动执行 `npm ci` + `npx playwright install --with-deps chromium` + `npx playwright test playwright/`

## frontend-ws-handshake-regression Phase 2 owner 的 TODO

1. 在 `frontend/playwright/` 下添加 `ws_handshake_*.spec.ts` 等 spec 文件
2. 不需要修改 workflow YAML——目录 glob 自动探测
3. 运行时依赖：spec 里自启本地 dev server 或 mock WS endpoint（本 workflow 不代起 backend）

## 本 change 已落地的后端可观测契约（你的 spec 可以 assert 它们）

- `internal/streaming/websocket.go:358-367` WS filter 语义：envelope SessionID 为空 → 转发；非空仅 sessionID 匹配时转发
- `tests/regression/red_subagent_*_test.go` 已断言 subagent progress / stream 两条路径的 envelope SessionID 正确
- `scripts/ci/check_session_scope.sh` 已 enforcement `internal/master/*.go` 源码层不出现裸 `Broadcast(BroadcastMessage)` / 裸 `BroadcastGenericMessage(EventTypeAgentProgress|ToolCall|SkillInstallProgress)`

## 本 change 明确不做的部分（delegated to you）

- `useChatStore` / `setCurrentSessionId(null)` 等 zustand store spy——`tests/regression/ws_reconnect_race_test.go:16` 文件头注释已显式声明 delegated
- WS handshake URL 必带 `?session_id=<id>` 的 URL 格式断言——你的 playwright spec 覆盖
- 浏览器层重连后首条消息不丢的 e2e 证据——你的 playwright spec 覆盖

## 可否立刻开写？

**是**。workflow + harness ready，只等你在 `frontend/playwright/` 下提交 spec 文件。

---

参考：本 change `specs/session-scope-regression-matrix/spec.md` R-1/R-2 envelope invariant 与你的 FE invariant 接续点在 envelope SessionID——后端保证携带正确，前端保证按此 envelope 过滤/路由。
