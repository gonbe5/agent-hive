package feishu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"
)

// ToolAdapter 将 feishu.Client 适配为 tools.FeishuToolProvider 接口
// 将内部类型序列化为 json.RawMessage 返回，避免 tools 包直接依赖 feishu 类型
type ToolAdapter struct {
	client *Client
}

// NewToolAdapter 创建飞书工具适配器
func NewToolAdapter(client *Client) *ToolAdapter {
	return &ToolAdapter{client: client}
}

// SearchDocs 搜索云文档
func (a *ToolAdapter) SearchDocs(ctx context.Context, query string, count int) (json.RawMessage, error) {
	items, err := a.client.SearchDocs(ctx, query, count)
	if err != nil {
		return nil, err
	}
	return json.Marshal(items)
}

// GetDocContent 获取文档内容
func (a *ToolAdapter) GetDocContent(ctx context.Context, documentID string) (string, error) {
	return a.client.GetDocContent(ctx, documentID)
}

func (a *ToolAdapter) GetWikiNode(ctx context.Context, spaceID, nodeToken string) (json.RawMessage, error) {
	return a.client.GetWikiNode(ctx, spaceID, nodeToken)
}

func (a *ToolAdapter) ResolveWikiNode(ctx context.Context, nodeToken string) (json.RawMessage, error) {
	return a.client.ResolveWikiNode(ctx, nodeToken)
}

func (a *ToolAdapter) ListWikiNodes(ctx context.Context, spaceID, parentNodeToken string, count int) (json.RawMessage, error) {
	return a.client.ListWikiNodes(ctx, spaceID, parentNodeToken, count)
}

// SearchContacts 搜索通讯录
func (a *ToolAdapter) SearchContacts(ctx context.Context, query string, pageSize int) (json.RawMessage, error) {
	items, err := a.client.SearchContacts(ctx, query, pageSize)
	if err != nil {
		return nil, err
	}
	return json.Marshal(items)
}

// GetUserInfo 获取用户详情
func (a *ToolAdapter) GetUserInfo(ctx context.Context, userID string) (json.RawMessage, error) {
	detail, err := a.client.GetUserInfo(ctx, userID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(detail)
}

// ListCalendarEvents 获取日历事件
func (a *ToolAdapter) ListCalendarEvents(ctx context.Context, calendarID string, startTime, endTime time.Time) (json.RawMessage, error) {
	events, err := a.client.ListCalendarEvents(ctx, calendarID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return json.Marshal(events)
}

// GetPrimaryCalendarID 获取主日历 ID
func (a *ToolAdapter) GetPrimaryCalendarID(ctx context.Context) (string, error) {
	return a.client.GetPrimaryCalendarID(ctx)
}

// SendMessage 发送消息（工具调用使用纯文本）
func (a *ToolAdapter) SendMessage(ctx context.Context, chatID, content string) error {
	return a.client.SendTextMessage(ctx, chatID, content)
}

func (a *ToolAdapter) UploadImage(ctx context.Context, imageBase64 string) (json.RawMessage, error) {
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, err
	}
	imageKey, err := a.client.UploadImage(ctx, data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"image_key": imageKey})
}

func (a *ToolAdapter) UploadFile(ctx context.Context, fileBase64, fileName string) (json.RawMessage, error) {
	data, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		return nil, err
	}
	fileKey, err := a.client.UploadFile(ctx, data, fileName)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{
		"file_key":  fileKey,
		"file_name": fileName,
	})
}

func (a *ToolAdapter) SendImage(ctx context.Context, chatID, imageKey string) error {
	content, err := json.Marshal(map[string]string{"image_key": imageKey})
	if err != nil {
		return err
	}
	return a.client.SendMessage(ctx, chatID, "image", string(content))
}

func (a *ToolAdapter) SendFile(ctx context.Context, chatID, fileKey string) error {
	content, err := json.Marshal(map[string]string{"file_key": fileKey})
	if err != nil {
		return err
	}
	return a.client.SendMessage(ctx, chatID, "file", string(content))
}

// CreateDoc 创建文档
func (a *ToolAdapter) CreateDoc(ctx context.Context, title string, folderToken string) (string, string, error) {
	return a.client.CreateDoc(ctx, title, folderToken)
}

// AppendDocContent 向文档追加内容
func (a *ToolAdapter) AppendDocContent(ctx context.Context, documentID string, content string) error {
	return a.client.AppendDocContent(ctx, documentID, content)
}

// --- 审批 ---

// ListApprovalInstances 查询审批实例列表
func (a *ToolAdapter) ListApprovalInstances(ctx context.Context, approvalCode string, startTime, endTime int64, pageSize int) (json.RawMessage, error) {
	return a.client.ListApprovalInstances(ctx, approvalCode, startTime, endTime, pageSize)
}

// GetApprovalInstance 获取审批实例详情
func (a *ToolAdapter) GetApprovalInstance(ctx context.Context, instanceID string) (json.RawMessage, error) {
	return a.client.GetApprovalInstance(ctx, instanceID)
}

// CreateApprovalInstance 创建审批实例
func (a *ToolAdapter) CreateApprovalInstance(ctx context.Context, approvalCode, openID, form string) (string, error) {
	return a.client.CreateApprovalInstance(ctx, approvalCode, openID, form)
}

// --- 多维表格 ---

// ListBitableRecords 列出多维表格记录
func (a *ToolAdapter) ListBitableRecords(ctx context.Context, appToken, tableID string, pageSize int, filter string) (json.RawMessage, error) {
	return a.client.ListBitableRecords(ctx, appToken, tableID, pageSize, filter)
}

// CreateBitableRecord 创建多维表格记录
func (a *ToolAdapter) CreateBitableRecord(ctx context.Context, appToken, tableID string, fields map[string]interface{}) (json.RawMessage, error) {
	return a.client.CreateBitableRecord(ctx, appToken, tableID, fields)
}

// UpdateBitableRecord 更新多维表格记录
func (a *ToolAdapter) UpdateBitableRecord(ctx context.Context, appToken, tableID, recordID string, fields map[string]interface{}) error {
	return a.client.UpdateBitableRecord(ctx, appToken, tableID, recordID, fields)
}

// ListBitableTables 列出多维表格的数据表
func (a *ToolAdapter) ListBitableTables(ctx context.Context, appToken string) (json.RawMessage, error) {
	return a.client.ListBitableTables(ctx, appToken)
}

// --- 任务 ---

// CreateTask 创建飞书任务
func (a *ToolAdapter) CreateTask(ctx context.Context, summary, dueTimestamp string) (json.RawMessage, error) {
	return a.client.CreateTask(ctx, summary, dueTimestamp)
}

// ListTasks 列出飞书任务
func (a *ToolAdapter) ListTasks(ctx context.Context, pageSize int) (json.RawMessage, error) {
	return a.client.ListTasks(ctx, pageSize)
}

// CompleteTask 完成飞书任务
func (a *ToolAdapter) CompleteTask(ctx context.Context, taskID string) error {
	return a.client.CompleteTask(ctx, taskID)
}

// --- 电子表格 ---

// ReadSheetRange 读取电子表格范围数据
func (a *ToolAdapter) ReadSheetRange(ctx context.Context, spreadsheetToken, sheetRange string) (json.RawMessage, error) {
	return a.client.ReadSheetRange(ctx, spreadsheetToken, sheetRange)
}

// WriteSheetRange 写入电子表格范围数据
func (a *ToolAdapter) WriteSheetRange(ctx context.Context, spreadsheetToken, sheetRange string, values [][]interface{}) error {
	return a.client.WriteSheetRange(ctx, spreadsheetToken, sheetRange, values)
}

// --- 群管理 ---

// GetChatInfo 获取群聊信息
func (a *ToolAdapter) GetChatInfo(ctx context.Context, chatID string) (json.RawMessage, error) {
	return a.client.GetChatInfo(ctx, chatID)
}

// ListChatMembers 获取群成员列表
func (a *ToolAdapter) ListChatMembers(ctx context.Context, chatID string, pageSize int) (json.RawMessage, error) {
	return a.client.ListChatMembers(ctx, chatID, pageSize)
}

// DownloadMessageResource 下载消息中的资源（图片/文件/音视频）。
// 返回 base64 编码的内容和文件名，避免 MCP 层传输二进制。
func (a *ToolAdapter) DownloadMessageResource(ctx context.Context, messageID, fileKey, resourceType string) (json.RawMessage, error) {
	data, fileName, err := a.client.DownloadMessageResource(ctx, messageID, fileKey, resourceType)
	if err != nil {
		return nil, err
	}
	result := struct {
		FileName string `json:"file_name"`
		Size     int    `json:"size"`
	}{
		FileName: fileName,
		Size:     len(data),
	}
	return json.Marshal(result)
}
