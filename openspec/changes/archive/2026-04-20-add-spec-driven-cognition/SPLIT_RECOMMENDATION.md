# 认知增强变更的拆分建议（给 owner 参考）

> 本文件不是工作成果，是**拉通建议**——供本变更 owner 采纳或丢弃。
> 底层判断：49 条任务中 9 条已完（全部为一期工程），剩余 40 条分布在二期（20 条）三期（11 条）最终验收（8 条）。

## 一、真实进度（别被 9/49 误导）

| Phase | 条数 | 状态 | 说明 |
|-------|-----|------|------|
| 一期 权限瘦身 | 10 | 工程侧 9/9 完成，剩 1.10 灰度 | 已收尾 |
| 二期 隐藏规范层 | 20 | 0/20 | **最大战场** |
| 三期 子代理+技能路由 | 11 | 0/11 | 依赖二期 |
| 最终验收 | 8 | 0/8 | 灰度+文档，非工程 |

## 二、二期拆成三段（串行不并行）

### 二期 A：新包 + 存储层（零撞车，可立即开干）
- 任务：2.0-2.8（9 条）
- 文件：**全部新建 `internal/specdriven/`**，不碰旧代码
- 产出：schema / store / retention / continuation / complexity / planner / verifier + 单测
- 验收：`go test ./internal/specdriven/...` 绿 + 覆盖率 ≥ 75%

### 二期 B：主控层接入（撞车高危，需排队）
- 任务：2.9-2.14（6 条）
- 文件：
  - `internal/master/session.go`
  - `internal/master/session_compact.go`
  - `internal/master/react_processor.go` ⚠️ 和"会话隔离回归测试矩阵"变更共用
  - `internal/master/prompt_builder.go`
  - `internal/channel/*`
- 前置：二期 A 的 Store 接口必须冻结
- 拉通点：和"会话隔离回归测试矩阵"的 owner 约定 react_processor.go 接入顺序

### 二期 C：配置 + 指标 + 集成（收尾）
- 任务：2.15-2.19（5 条）
- 文件：`internal/config/config.go` / `internal/observability/*` / `internal/master/spec_e2e_test.go`
- 动作：灰度开关 + 9 个指标 + 端到端测试

## 三、三期拆成两段（依赖二期）

### 三期 A：子代理侧接入（强绑定）
- 任务：3.1-3.3（3 条）
- 前置：二期 A 完成
- **强约束**：3.3 明确声明「与 hive-skill-on-demand 同 PR 落地」→ 两个变更必须合并提交

### 三期 B：派活逻辑 + 回退
- 任务：3.4-3.11（8 条）
- 前置：三期 A

## 四、给 owner 的建议顺序

```
立即可做（零前置、零撞车）：
  二期 A（9 条）

二期 A 完成后：
  二期 B（6 条）← 需要和"会话隔离回归测试矩阵" owner 对齐
  二期 C（5 条）

二期完成后：
  三期 A（3 条）← 必须和"按需加载技能"同 PR
  三期 B（8 条）

全部工程完成后：
  一期 1.10 灰度 + 最终验收 F.1-F.8
```

## 五、风险点

| 风险 | 缓解 |
|------|------|
| react_processor.go 双变更撞车 | 二期 B 和回归测试矩阵 owner 对齐接入顺序 |
| 三期 A 和按需加载技能强绑定 | 两 change 必须同 PR；任一 owner 先合都会破坏另一方 |
| 二期新建 `internal/specdriven/` 包大 | 按 2.0-2.8 原顺序开文件，不要跳步 |

## 六、不采纳本建议的后果

平行推 20+ 条任务触十几个文件，二期三期随意接入，结果：
- react_processor.go rebase 地狱
- hive-skill-on-demand 合并后才发现三期 A 的接入点错位
- 验收时发现 Store 接口和主控层对不上，回头改

拆三段 + 强约束，是防撞的**抓手**。
