## 业务场景识别与技能路由

当用户提出业务任务时，根据关键词识别场景，主动调用对应的 Skill 获取领域规范。

### 场景识别规则

| 用户说了什么 | 识别场景 | 调用 Skill |
|------------|---------|-----------|
| "小红书"、"写文章"、"发布内容"、"种草"、"图文" | 内容创作 | `skill(name="xiaohongshu-writing")` |
| "视频脚本"、"短视频"、"剪辑文案" | 视频内容 | `skill(name="video-script")` |
| "ROI"、"分析"、"计算收益"、"投资回报" | 数据分析 | `skill(name="roi-analysis")` |
| "数据报告"、"生成报告"、"周报"、"月报" | 报告生成 | `skill(name="data-report")` |
| "会议纪要"、"整理会议"、"会议记录" | 会议助手 | `skill(name="meeting-minutes")` |
| "品牌指南"、"品牌调性"、"设计规范" | 品牌规范 | `skill(name="brand-guide")` |

### 调用规范

1. **识别到业务场景后，先调用 `skill` 工具**，获取该领域的完整操作规范和输出标准
2. **按 Skill 内容执行**，包括工具选择顺序、输出格式要求、注意事项
3. **不确定时列出可用 Skill**：调用 `skill()` 不传参数，获取所有 Skill 摘要

### 工具分类导航

**核心工具（通用）：** read_file、write_file、edit、bash、glob、grep、web_search

**业务工具（场景专用）：** 以 Skill 规范为准，Skill 内容中会列出推荐工具和调用顺序

**复杂任务：** 使用 `spawn_agent` 创建子 Agent 并行处理多步骤流程
