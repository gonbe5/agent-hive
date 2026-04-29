---
name: create-case-skill
description: "从 GitHub 私有仓库 PR 的代码变更中自动分析，生成测试用例，写入 AFlow 测试平台。调用方式: skill create-case-skill，参数: <owner/repo> <pr_number> <team_id> <product_id>"
allowed-tools: bash
metadata:
  author: hive-team
  version: "1.0"
---

# AFlow 测试用例自动生成

从 GitHub 私有仓库 PR 中读取代码变更，由 LLM 分析生成测试用例，写入 AFlow 测试平台。

## 前置环境变量

在 Hive 运行环境中必须配置：

| 变量名 | 说明 | 示例 |
|--------|------|------|
| `GITHUB_TOKEN` | GitHub Personal Access Token（repo read 权限） | `ghp_xxxx` |
| `AFLOW_BASE_URL` | AFlow 服务根地址，不带末尾斜杠 | `http://your-aflow-host` |
| `AFLOW_EMAIL` | AFlow 登录邮箱（用于获取 token） | `bot@company.com` |
| `AFLOW_PASSWORD` | AFlow 登录密码 | `password123` |

---

## Step 1：解析参数

从 Arguments 中按空格分隔解析四个参数：
1. `REPO`：GitHub 仓库，格式 `owner/repo`，如 `org/my-service`
2. `PR_NUM`：PR 编号，如 `42`
3. `TEAM_ID`：AFlow 团队 ID
4. `PRODUCT_ID`：AFlow 项目 ID

---

## Step 2：获取 AFlow 访问 Token

执行以下命令登录 AFlow，获取 token（有效期 365 天，可缓存复用）：

```bash
AFLOW_TOKEN=$(curl -s -X POST "$AFLOW_BASE_URL/brain/api/v1/auth/user_login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$AFLOW_EMAIL\",\"password\":\"$AFLOW_PASSWORD\",\"is_auto_login\":true}" \
  | jq -r '.data.token // .token')

echo "AFlow token 获取: ${AFLOW_TOKEN:0:20}..."
```

如果环境中已有 `AFLOW_TOKEN` 变量，直接跳过登录步骤使用已有 token。

---

## Step 3：拉取 PR 变更文件列表

```bash
PR_FILES=$(curl -s \
  -H "Authorization: Bearer $GITHUB_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "https://api.github.com/repos/$REPO/pulls/$PR_NUM/files")

echo "$PR_FILES" | jq -r '.[] | "\(.status) \(.filename)"'
```

**过滤规则（只保留有价值的源码文件）：**

保留条件（同时满足）：
- `status` != `"removed"`
- 文件扩展名属于：`.go` `.java` `.py` `.ts` `.js` `.kt` `.swift` `.cs` `.cpp` `.php` `.rb`

跳过条件（满足任意一条即跳过）：
- 文件名包含 `_test.go` / `.test.ts` / `.test.js` / `.spec.` / `_spec.`
- 路径包含 `/test/` `/tests/` `/__tests__/` `/mock/` `/fixture/`
- 文件名以 `.md` `.yaml` `.yml` `.json` `.toml` `.lock` `.sum` `.mod` 结尾
- 文件名以 `Dockerfile` `Makefile` `.gitignore` 等配置文件开头/结尾

过滤后，提取每个文件的 `raw_url` 字段。

---

## Step 4：读取每个变更文件的代码内容

对过滤后的每个文件，逐一获取内容：

```bash
# 示例：获取单个文件内容（raw_url 来自上一步）
FILE_CONTENT=$(curl -s \
  -H "Authorization: Bearer $GITHUB_TOKEN" \
  "<raw_url>")

# 只取前 200 行，防止超出 LLM context
echo "$FILE_CONTENT" | head -200
```

记录所有文件内容，准备进行分析。

---

## Step 5：分析代码，生成测试用例

对每个代码文件，分析其中的**公开函数/方法/接口/Handler**，生成测试用例。

### 分析策略

- **Go**：分析 `func` 开头的公开函数（首字母大写），HTTP Handler 函数，及带有 `_test.go` 对应的业务逻辑函数
- **Java/Kotlin**：分析 `public` 方法、`@Controller`/`@Service`/`@RestController` 注解的类
- **Python**：分析 `def` 开头的公开函数（不以 `_` 开头）、Flask/Django 路由函数
- **TypeScript/JavaScript**：分析 `export` 的函数、API 路由 handler

对每个识别到的函数/方法，生成 **至少 2 条** 测试用例：
1. 正常路径（valid input → expected output）
2. 异常/边界路径（nil/空值/非法参数 → 预期错误或边界行为）

对核心业务逻辑函数（如支付、鉴权、数据写入），生成 3 条。

### 生成的 JSON 必须严格符合以下格式

每条测试用例是一个独立 JSON 对象：

```json
{
  "team_id": "<TEAM_ID 参数>",
  "product_id": "<PRODUCT_ID 参数>",
  "folder_parent_id": "0",
  "name": "<函数名>_<场景简述>",
  "description": "<一句话说明本用例验证什么>",
  "pre_condition": "<测试前需要准备的数据或环境，无则填空字符串>",
  "level": 1,
  "test_case_type": 3,
  "test_case_result": 0,
  "sort": 1,
  "status_id": "",
  "head_user_id": "",
  "attaches": [],
  "version_ids": [],
  "test_case_operations": [
    {
      "operation_id": "<uuid-v4 随机生成>",
      "description": "<具体操作描述，包含参数值，不能为空>",
      "expect_result": "<预期的返回值或系统状态>",
      "test_case_result": 0,
      "real_result": ""
    }
  ]
}
```

### 字段填写规范

**`level` 用例优先级：**
- `1` = P0，核心主流程（如登录、支付、数据写入主路径）
- `2` = P1，重要功能
- `3` = P2，普通功能
- `4` = P3，边缘场景（默认）
- `5` = P4，极低优先级

**`test_case_type` 用例类型：**
- `1` = 功能测试（业务逻辑函数）
- `3` = 接口测试（HTTP Handler、RPC 接口）

**`operation_id`：** 每个 operation 生成独立的 UUID v4 格式字符串，如 `"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`

**`test_case_operations` 规范：**
- 每条用例至少 2 步，最多 6 步
- 每步 `description` **不得为空**（服务端有校验，为空会返回错误）
- 步骤描述要具体，包含实际参数值和操作动作
- `expect_result` 要具体，说明预期返回值/状态码/数据变化

**`name` 命名规范：**
- 格式：`函数名_场景描述`
- 示例：`GetUser_用户存在时返回完整信息`、`CreateOrder_金额为负数时返回参数错误`

---

## Step 6：逐条写入 AFlow

对每条生成的测试用例 JSON，调用 AFlow API：

```bash
RESULT=$(curl -s -X POST "$AFLOW_BASE_URL/pm/api/v1/test_case/save" \
  -H "Authorization: $AFLOW_TOKEN" \
  -H "Content-Type: application/json" \
  -H "CurrentTeamID: $TEAM_ID" \
  -d '<单条测试用例 JSON>')

CODE=$(echo "$RESULT" | jq -r '.code')
if [ "$CODE" = "0" ]; then
  echo "✅ 写入成功: <用例名>"
else
  echo "❌ 写入失败: <用例名> | 原因: $(echo $RESULT | jq -r '.msg // .em')"
fi
```

**写入策略：**
- 逐条写入，不要并发（避免 rank_id 冲突）
- 某条失败时记录错误，继续处理下一条（不中断）
- 同一文件的用例顺序：正常路径在前，异常路径在后

---

## Step 7：输出汇总报告

所有用例处理完成后，输出：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ AFlow 测试用例自动生成完成
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
PR:        <owner/repo>#<pr_number>
分析文件:   X 个
生成用例:   Y 条
写入成功:   Z 条
写入失败:   N 条

[如有失败，逐条列出]
❌ <用例名> — <错误原因>

查看地址: <AFLOW_BASE_URL>/project/testcase
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```
