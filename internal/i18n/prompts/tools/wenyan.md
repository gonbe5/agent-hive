#### wenyan 发布规范
调用 wenyan__publish_article 时，文章内容（content 参数）必须包含 YAML frontmatter，其中 title 字段为必填，格式为：
```
---
title: 文章标题
---

正文内容...
```
frontmatter 还支持可选字段：description（摘要）、cover（封面图URL）、author（作者）、source_url（原文链接）。
缺少 frontmatter 中的 `title` 字段会导致公众号工具报错「未能找到文章标题」，发布失败。
**重要**：公众号发布功能依赖环境变量 `WECHAT_APP_ID` 和 `WECHAT_APP_SECRET`。如果未配置，工具会静默失败（返回成功但实际未推送）。发布前请确认这两个变量已正确设置。

#### wenyan 封面与插图规范
发布公众号文章时，**必须**完成以下步骤：
1. **封面图**：调用 `generate_image` 生成封面图，尺寸使用 `1792x1024`（横版，适合公众号封面比例）。
   - prompt 应基于文章主题，风格简洁、视觉冲击力强，适合公众号读者。
   - 将返回的图片 URL 填入 frontmatter 的 `cover` 字段。
2. **正文插图**：根据文章内容，在适当段落后调用 `generate_image` 生成 1-3 张插图，尺寸使用 `1024x1024`。
   - 将图片 URL 以 Markdown 图片语法嵌入正文：`![描述](图片URL)`。
3. 图片生成完成后再调用 `wenyan__publish_article` 发布，确保封面和插图都已就位。
