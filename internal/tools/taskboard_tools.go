package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/taskboard"
)

// taskboardInput 是 taskboard 工具的输入参数
type taskboardInput struct {
	Operation   string   `json:"operation"`              // create, get, update, list, delete
	ID          string   `json:"id,omitempty"`           // 任务 ID（get/update/delete）
	Title       string   `json:"title,omitempty"`        // 标题（create/update）
	Description string   `json:"description,omitempty"`  // 描述（create/update）
	Status      string   `json:"status,omitempty"`       // 状态（create/update/list 过滤）
	Priority    string   `json:"priority,omitempty"`     // 优先级（create/update/list 过滤）
	Assignee    string   `json:"assignee,omitempty"`     // 负责人（create/update/list 过滤）
	ParentID    string   `json:"parent_id,omitempty"`    // 父任务 ID（create）
	SessionID   string   `json:"session_id,omitempty"`   // 会话 ID（create/list 过滤）
	Tags        []string `json:"tags,omitempty"`         // 标签（create/update/list 过滤）
	ClearFields []string `json:"clear_fields,omitempty"` // update 时要清空的字段名列表
	Limit       int      `json:"limit,omitempty"`        // 分页限制（list）
	Offset      int      `json:"offset,omitempty"`       // 分页偏移（list）
}

// RegisterTaskBoard 注册 taskboard 工具到 MCP host
func RegisterTaskBoard(host *mcphost.Host, logger *zap.Logger, board taskboard.TaskBoard) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "get", "update", "list", "delete"},
				"description": "操作类型: create=创建任务, get=获取任务, update=更新任务, list=列出任务, delete=删除任务",
			},
			"id": map[string]any{
				"type":        "string",
				"description": "任务 ID（get/update/delete 操作需要）",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "任务标题（create 必填，update 可选）",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "任务描述",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "done", "blocked", "cancelled"},
				"description": "任务状态",
			},
			"priority": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "任务优先级（默认 medium）",
			},
			"assignee": map[string]any{
				"type":        "string",
				"description": "负责人（agent ID 或用户标识）",
			},
			"parent_id": map[string]any{
				"type":        "string",
				"description": "父任务 ID（用于创建子任务）",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "关联的会话 ID",
			},
			"tags": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "标签列表",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "返回结果数量限制（list 操作，默认 50）",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "分页偏移（list 操作）",
			},
			"clear_fields": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "update 时要清空的字段名列表（如 [\"description\",\"assignee\"]）",
			},
		},
		"required": []string{"operation"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "taskboard",
			Description: "持久化工作项管理：创建、查询、更新、删除任务。任务跨会话持久保存，支持状态追踪、优先级、标签和子任务。",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params taskboardInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			switch params.Operation {
			case "create":
				return taskboardCreate(ctx, board, params)
			case "get":
				return taskboardGet(ctx, board, params)
			case "update":
				return taskboardUpdate(ctx, board, params)
			case "list":
				return taskboardList(ctx, board, params)
			case "delete":
				return taskboardDelete(ctx, board, params)
			default:
				return errorResult(fmt.Sprintf("未知操作: %q，支持: create/get/update/list/delete", params.Operation)), nil
			}
		},
	)

	logger.Info("taskboard 工具已注册")
}

func taskboardCreate(ctx context.Context, board taskboard.TaskBoard, params taskboardInput) (*mcphost.ToolResult, error) {
	if params.Title == "" {
		return errorResult("create 操作需要 title"), nil
	}

	task := &taskboard.Task{
		SessionID:   params.SessionID,
		Title:       params.Title,
		Description: params.Description,
		Assignee:    params.Assignee,
		ParentID:    params.ParentID,
		Tags:        params.Tags,
	}
	if params.Status != "" {
		task.Status = taskboard.Status(params.Status)
	}
	if params.Priority != "" {
		task.Priority = taskboard.Priority(params.Priority)
	}

	id, err := board.Create(ctx, task)
	if err != nil {
		return errorResult("创建任务失败: " + err.Error()), nil
	}

	data, _ := json.Marshal(map[string]string{"id": id, "message": "任务已创建"})
	return textResult(string(data)), nil
}

func taskboardGet(ctx context.Context, board taskboard.TaskBoard, params taskboardInput) (*mcphost.ToolResult, error) {
	if params.ID == "" {
		return errorResult("get 操作需要 id"), nil
	}

	task, err := board.Get(ctx, params.ID)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return textResult(string(data)), nil
}

func taskboardUpdate(ctx context.Context, board taskboard.TaskBoard, params taskboardInput) (*mcphost.ToolResult, error) {
	if params.ID == "" {
		return errorResult("update 操作需要 id"), nil
	}

	// 构建要清空的字段集合
	clearSet := make(map[string]bool, len(params.ClearFields))
	for _, f := range params.ClearFields {
		clearSet[f] = true
	}

	patch := taskboard.TaskPatch{}
	hasUpdate := false

	if params.Title != "" {
		patch.Title = &params.Title
		hasUpdate = true
	}
	if params.Description != "" || clearSet["description"] {
		patch.Description = &params.Description
		hasUpdate = true
	}
	if params.Status != "" {
		s := taskboard.Status(params.Status)
		patch.Status = &s
		hasUpdate = true
	}
	if params.Priority != "" {
		p := taskboard.Priority(params.Priority)
		patch.Priority = &p
		hasUpdate = true
	}
	if params.Assignee != "" || clearSet["assignee"] {
		patch.Assignee = &params.Assignee
		hasUpdate = true
	}
	if params.Tags != nil {
		patch.Tags = params.Tags
		hasUpdate = true
	}

	if !hasUpdate {
		return errorResult("update 操作至少需要一个更新字段或 clear_fields"), nil
	}

	if err := board.Update(ctx, params.ID, patch); err != nil {
		return errorResult(err.Error()), nil
	}

	return textResult(fmt.Sprintf("任务 %s 已更新", params.ID)), nil
}

func taskboardList(ctx context.Context, board taskboard.TaskBoard, params taskboardInput) (*mcphost.ToolResult, error) {
	filter := taskboard.TaskFilter{
		SessionID: params.SessionID,
		Assignee:  params.Assignee,
		ParentID:  params.ParentID,
		Tags:      params.Tags,
		Limit:     params.Limit,
		Offset:    params.Offset,
	}
	if params.Status != "" {
		filter.Status = taskboard.Status(params.Status)
	}
	if params.Priority != "" {
		filter.Priority = taskboard.Priority(params.Priority)
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	tasks, err := board.List(ctx, filter)
	if err != nil {
		return errorResult("查询任务失败: " + err.Error()), nil
	}

	if len(tasks) == 0 {
		return textResult("没有匹配的任务"), nil
	}

	// 格式化为可读文本
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个任务:\n\n", len(tasks)))
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("- [%s] %s | %s | %s", t.ID, t.Title, t.Status, t.Priority))
		if t.Assignee != "" {
			sb.WriteString(fmt.Sprintf(" | @%s", t.Assignee))
		}
		if len(t.Tags) > 0 {
			sb.WriteString(fmt.Sprintf(" | tags:%s", strings.Join(t.Tags, ",")))
		}
		sb.WriteString("\n")
	}
	return textResult(sb.String()), nil
}

func taskboardDelete(ctx context.Context, board taskboard.TaskBoard, params taskboardInput) (*mcphost.ToolResult, error) {
	if params.ID == "" {
		return errorResult("delete 操作需要 id"), nil
	}

	if err := board.Delete(ctx, params.ID); err != nil {
		return errorResult(err.Error()), nil
	}

	return textResult(fmt.Sprintf("任务 %s 已删除", params.ID)), nil
}
