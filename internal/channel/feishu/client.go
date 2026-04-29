package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkapproval "github.com/larksuite/oapi-sdk-go/v3/service/approval/v4"
	larkbitable "github.com/larksuite/oapi-sdk-go/v3/service/bitable/v1"
	larkcalendar "github.com/larksuite/oapi-sdk-go/v3/service/calendar/v4"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larksheets "github.com/larksuite/oapi-sdk-go/v3/service/sheets/v3"
	larkwiki "github.com/larksuite/oapi-sdk-go/v3/service/wiki/v2"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/imctx"
	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larktask "github.com/larksuite/oapi-sdk-go/v3/service/task/v1"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/observability"
)

// Client 飞书 API 客户端（基于官方 SDK）
type Client struct {
	larkClient    *lark.Client
	logger        *zap.Logger
	rateLimiter   *feishuRateLimiter
	maxRetries    int
	health        *clientHealthTracker
	metricsWriter observability.MetricsWriter
	// Phase 3 缺口 6 修复:botOpenID 用 atomic.Pointer 而非 sync.Once。
	// 旧 sync.Once 失败一次永久固化("" 永久缓存)→ 启动期飞书 API 抖动 →
	// botOpenID 永空 → 自反射防御失效 → bot 跟自己对话死循环。
	// 现改成"成功才缓存"模式:失败下次 BotOpenID() 会再试,最大化恢复机会。
	botOpenIDPtr  atomic.Pointer[string]
	botOpenIDMu   sync.Mutex // 保护 fetch in-flight,防 thundering herd
	botOpenIDOnce sync.Once  // 兼容老路径(reloadable_test 等),保留但不再用
	botOpenID     string
	botOpenIDErr  error
}

const (
	maxFeishuImageUploadSize = 10 * 1024 * 1024
	maxFeishuFileUploadSize  = 30 * 1024 * 1024
)

var ErrPermissionDenied = errors.New("feishu permission denied")

// NewClient 创建飞书 API 客户端
func NewClient(appID, appSecret string, logger *zap.Logger, opts ...lark.ClientOptionFunc) *Client {
	client := &Client{
		larkClient:  lark.NewClient(appID, appSecret, opts...),
		logger:      logger,
		rateLimiter: newFeishuRateLimiter(45, 8),
		maxRetries:  3,
		health:      &clientHealthTracker{permissionDegradeThreshold: 5},
	}
	client.ApplyHealthConfig(appID, appSecret, "", "")
	return client
}

func (c *Client) ApplyOutboundConfig(cfg config.FeishuConfig) {
	if c == nil {
		return
	}
	c.rateLimiter = newFeishuRateLimiter(cfg.OutboundGlobalQPSResolved(), cfg.OutboundPerChatQPSResolved())
	c.maxRetries = cfg.OutboundMaxRetriesResolved()
	c.ApplySecurityConfig(cfg.PermissionDegradeThresholdResolved())
	c.ApplyHealthConfig(cfg.AppID, cfg.AppSecret, cfg.VerificationToken, cfg.EncryptKey)
}

func (c *Client) ReloadFromConfig(cfg config.FeishuConfig) error {
	if c == nil {
		return nil
	}
	c.ApplyOutboundConfig(cfg)
	return nil
}

func (c *Client) checkDegradedOutbound() error {
	if c == nil {
		return errs.New(errs.CodeChannelSendFailed, "飞书客户端未初始化")
	}
	if c.degraded(time.Now()) {
		c.emitOutboundRejectedMetric("degraded")
		return errs.New(errs.CodeChannelSendFailed, "飞书客户端已降级，暂停出站发送")
	}
	return nil
}

func (c *Client) SetMetricsWriter(w observability.MetricsWriter) {
	if c == nil {
		return
	}
	c.metricsWriter = w
}

func (c *Client) emitOutboundRejectedMetric(reason string) {
	if c == nil || c.metricsWriter == nil {
		return
	}
	_ = c.metricsWriter.Record(context.Background(), observability.Metric{
		Name:  MetricOutboundRejected,
		Value: 1,
		Labels: map[string]any{
			"reason": reason,
		},
		Ts: time.Now(),
	})
}

// ReplyMessage 回复消息（支持 text / interactive）
func (c *Client) ReplyMessage(ctx context.Context, messageID, msgType, content string) error {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return err
	}
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewReplyMessageReqBuilder().
			MessageId(messageID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(content).
				Build()).
			Build()

		resp, err := c.larkClient.Im.Message.Reply(ctx, req)
		if err != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "回复消息失败", err)
		}
		if !resp.Success() {
			return errors.New(fmt.Sprintf("回复消息失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		return nil
	})
	c.observeAPIError(err, time.Now())
	return err
}

// SendMessage 发送消息到指定聊天（支持 text / interactive）
func (c *Client) SendMessage(ctx context.Context, chatID, msgType, content string) error {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return err
	}
	if err := c.rateLimiter.Wait(ctx, chatID); err != nil {
		c.observeAPIError(err, time.Now())
		return errs.Wrap(errs.CodeChannelSendFailed, "发送消息失败", err)
	}
	// Phase 6 缺口 12 修复:按 chatID 前缀自动切 receive_id_type,
	// push.Service 走 P2P open_id 路径不再被 ReceiveIdType("chat_id") 锁死。
	idType, normalizedID := inferReceiveIDType(chatID)
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(idType).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(normalizedID).
				MsgType(msgType).
				Content(content).
				Build()).
			Build()

		resp, err := c.larkClient.Im.Message.Create(ctx, req)
		if err != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "发送消息失败", err)
		}
		if !resp.Success() {
			return errors.New(fmt.Sprintf("发送消息失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		return nil
	})
	c.observeAPIError(err, time.Now())
	return err
}

// SendTextMessage 发送纯文本消息（便捷方法，供 ToolAdapter 等使用）
func (c *Client) SendTextMessage(ctx context.Context, chatID, text string) error {
	contentJSON, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "序列化消息内容失败", err)
	}
	return c.SendMessage(ctx, chatID, "text", string(contentJSON))
}

func (c *Client) UploadImage(ctx context.Context, data []byte) (string, error) {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return "", err
	}
	if len(data) == 0 {
		return "", errs.New(errs.CodeChannelSendFailed, "上传图片失败: 空图片")
	}
	if len(data) > maxFeishuImageUploadSize {
		return "", errs.New(errs.CodeChannelSendFailed, "上传图片失败: 超过 10MB 限制")
	}
	// Phase 4 缺口 7 修复:核心 API 必须包 rateLimiter.Wait + withRetry,
	// 否则飞书侧 99991400/5xx 时直接报错给 agent,不重试。
	if err := c.rateLimiter.Wait(ctx, ""); err != nil {
		c.observeAPIError(err, time.Now())
		return "", errs.Wrap(errs.CodeChannelSendFailed, "上传图片失败", err)
	}
	var imageKey string
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().
				ImageType("message").
				Image(bytes.NewReader(data)).
				Build()).
			Build()

		resp, innerErr := c.larkClient.Im.Image.Create(ctx, req)
		if innerErr != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "上传图片失败", innerErr)
		}
		if !resp.Success() {
			return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("上传图片失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		if resp.Data == nil || resp.Data.ImageKey == nil || *resp.Data.ImageKey == "" {
			return errs.New(errs.CodeChannelSendFailed, "上传图片成功但未返回 image_key")
		}
		imageKey = *resp.Data.ImageKey
		return nil
	})
	c.observeAPIError(err, time.Now())
	if err != nil {
		return "", err
	}
	return imageKey, nil
}

func (c *Client) UploadFile(ctx context.Context, data []byte, fileName string) (string, error) {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return "", err
	}
	if len(data) == 0 {
		return "", errs.New(errs.CodeChannelSendFailed, "上传文件失败: 空文件")
	}
	if len(data) > maxFeishuFileUploadSize {
		return "", errs.New(errs.CodeChannelSendFailed, "上传文件失败: 超过 30MB 限制")
	}
	if strings.TrimSpace(fileName) == "" {
		return "", errs.New(errs.CodeChannelSendFailed, "上传文件失败: 缺少文件名")
	}
	fileType := "stream"
	if idx := strings.LastIndex(fileName, "."); idx >= 0 && idx < len(fileName)-1 {
		fileType = strings.ToLower(fileName[idx+1:])
	}
	// Phase 4 缺口 7 修复:核心 API 包 rateLimiter.Wait + withRetry。
	if err := c.rateLimiter.Wait(ctx, ""); err != nil {
		c.observeAPIError(err, time.Now())
		return "", errs.Wrap(errs.CodeChannelSendFailed, "上传文件失败", err)
	}
	var fileKey string
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(fileType).
				FileName(fileName).
				File(bytes.NewReader(data)).
				Build()).
			Build()

		resp, innerErr := c.larkClient.Im.File.Create(ctx, req)
		if innerErr != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "上传文件失败", innerErr)
		}
		if !resp.Success() {
			return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("上传文件失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		if resp.Data == nil || resp.Data.FileKey == nil || *resp.Data.FileKey == "" {
			return errs.New(errs.CodeChannelSendFailed, "上传文件成功但未返回 file_key")
		}
		fileKey = *resp.Data.FileKey
		return nil
	})
	c.observeAPIError(err, time.Now())
	if err != nil {
		return "", err
	}
	return fileKey, nil
}

// BuildMarkdownCard 将 Markdown 文本包装为飞书 Interactive 卡片 JSON
// 使用 Card 1.0 格式：div 元素 + lark_md 文本标签。
// 纯 JSON 内容自动包装为 json 代码块。
func BuildMarkdownCard(markdown string) string {
	content := strings.TrimSpace(markdown)

	// 纯 JSON 检测：如果内容本身是完整 JSON，则包装为代码块
	if channel.IsRawJSON(content) {
		content = "```json\n" + content + "\n```"
	}

	card := map[string]any{
		"elements": []any{
			map[string]any{
				"tag": "div",
				"text": map[string]any{
					"tag":     "lark_md",
					"content": content,
				},
			},
		},
	}
	b, _ := json.Marshal(card)
	return string(b)
}

// --- 飞书 Tool API 方法 ---

// SearchDocs 搜索云文档
// 注意：SDK 的 search/v2 需要 user_access_token，此处保留使用旧版 API
// 若后续也出现鉴权问题，需改用 Drive 文件列表接口
func (c *Client) SearchDocs(ctx context.Context, query string, count int) ([]DocItem, error) {
	if count <= 0 {
		count = 10
	}

	// 旧版文档搜索 API 支持 tenant_access_token，SDK 中无对应方法，使用 RawRequest
	body := map[string]interface{}{
		"search_key": query,
		"count":      count,
		"offset":     0,
		"owner_ids":  []string{},
		"docs_types": []string{},
	}

	// SDK-RAWREQUEST-ALLOWED: 飞书 suite docs-api/search/object 是老 API,SDK v3.5.3 未暴露 typed builder。
	apiResp, err := c.larkClient.Post(ctx, "/open-apis/suite/docs-api/search/object",
		body, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "搜索文档失败", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			DocsEntities []struct {
				DocsToken  string `json:"docs_token"`
				DocsType   string `json:"docs_type"`
				Title      string `json:"title"`
				OwnerID    string `json:"owner_id"`
				CreateTime string `json:"create_time"`
				UpdateTime string `json:"update_time"`
				URL        string `json:"docs_url"`
			} `json:"docs_entities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "解析搜索响应失败", err)
	}
	if result.Code != 0 {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("搜索文档失败: code=%d, msg=%s", result.Code, result.Msg))
	}

	items := make([]DocItem, 0, len(result.Data.DocsEntities))
	for _, e := range result.Data.DocsEntities {
		items = append(items, DocItem{
			Title:      e.Title,
			URL:        e.URL,
			DocToken:   e.DocsToken,
			DocType:    e.DocsType,
			OwnerID:    e.OwnerID,
			CreateTime: e.CreateTime,
			UpdateTime: e.UpdateTime,
		})
	}
	return items, nil
}

// GetDocContent 获取文档纯文本内容
func (c *Client) GetDocContent(ctx context.Context, documentID string) (string, error) {
	req := larkdocx.NewRawContentDocumentReqBuilder().
		DocumentId(documentID).
		Build()

	resp, err := c.larkClient.Docx.Document.RawContent(ctx, req)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "获取文档内容失败", err)
	}
	if !resp.Success() {
		return "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取文档失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data != nil && resp.Data.Content != nil {
		return *resp.Data.Content, nil
	}
	return "", nil
}

// SearchContacts 搜索通讯录用户
// 使用 contact/v3 的 FindByDepartment 接口（支持 tenant_access_token），在根部门下按名字过滤
// 自动处理分页，直到收集到足够结果或遍历完所有用户
func (c *Client) SearchContacts(ctx context.Context, query string, pageSize int) ([]ContactItem, error) {
	if pageSize <= 0 {
		pageSize = 10
	}

	const fetchPageSize = 50 // 每次 API 请求拉取 50 条
	queryLower := strings.ToLower(query)
	items := make([]ContactItem, 0, pageSize)
	var pageToken string

	for {
		builder := larkcontact.NewFindByDepartmentUserReqBuilder().
			DepartmentId("0").
			PageSize(fetchPageSize)
		if pageToken != "" {
			builder = builder.PageToken(pageToken)
		}

		resp, err := c.larkClient.Contact.User.FindByDepartment(ctx, builder.Build())
		if err != nil {
			return nil, errs.Wrap(errs.CodeChannelSendFailed, "搜索通讯录失败", err)
		}
		if !resp.Success() {
			return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("搜索通讯录失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}

		if resp.Data != nil {
			for _, u := range resp.Data.Items {
				name := derefStr(u.Name)
				// 按名字模糊匹配
				if query != "" && !strings.Contains(strings.ToLower(name), queryLower) {
					continue
				}
				status := "active"
				if u.Status != nil && u.Status.IsFrozen != nil && *u.Status.IsFrozen {
					status = "frozen"
				}
				avatarURL := ""
				if u.Avatar != nil {
					avatarURL = derefStr(u.Avatar.Avatar72)
				}
				items = append(items, ContactItem{
					UserID: derefStr(u.UserId),
					OpenID: derefStr(u.OpenId),
					Name:   name,
					Email:  derefStr(u.Email),
					Mobile: derefStr(u.Mobile),
					Avatar: avatarURL,
					Status: status,
				})
				if len(items) >= pageSize {
					return items, nil
				}
			}

			// 检查是否还有更多页
			if resp.Data.HasMore == nil || !*resp.Data.HasMore || resp.Data.PageToken == nil {
				break
			}
			pageToken = *resp.Data.PageToken
		} else {
			break
		}
	}
	return items, nil
}

// GetUserInfo 获取用户详细信息
func (c *Client) GetUserInfo(ctx context.Context, userID string) (*UserDetail, error) {
	req := larkcontact.NewGetUserReqBuilder().
		UserId(userID).
		UserIdType("user_id").
		Build()

	resp, err := c.larkClient.Contact.User.Get(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取用户信息失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取用户信息失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || resp.Data.User == nil {
		return nil, errs.New(errs.CodeChannelSendFailed, "用户信息为空")
	}
	u := resp.Data.User
	avatarURL := ""
	if u.Avatar != nil {
		avatarURL = derefStr(u.Avatar.AvatarOrigin)
	}
	employeeType := 0
	if u.EmployeeType != nil {
		employeeType = *u.EmployeeType
	}
	return &UserDetail{
		UserID:        derefStr(u.UserId),
		OpenID:        derefStr(u.OpenId),
		Name:          derefStr(u.Name),
		EnName:        derefStr(u.EnName),
		Email:         derefStr(u.Email),
		Mobile:        derefStr(u.Mobile),
		Avatar:        avatarURL,
		DepartmentIDs: u.DepartmentIds,
		JobTitle:      derefStr(u.JobTitle),
		WorkStation:   derefStr(u.WorkStation),
		City:          derefStr(u.City),
		EmployeeType:  employeeType,
	}, nil
}

// GetUserInfoByOpenID 按 open_id 获取用户详情。
func (c *Client) GetUserInfoByOpenID(ctx context.Context, openID string) (*UserDetail, error) {
	req := larkcontact.NewGetUserReqBuilder().
		UserId(openID).
		UserIdType("open_id").
		Build()

	resp, err := c.larkClient.Contact.User.Get(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取用户信息失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取用户信息失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.User == nil {
		return nil, errs.New(errs.CodeChannelSendFailed, "用户信息为空")
	}
	u := resp.Data.User
	avatarURL := ""
	if u.Avatar != nil {
		avatarURL = derefStr(u.Avatar.AvatarOrigin)
	}
	employeeType := 0
	if u.EmployeeType != nil {
		employeeType = *u.EmployeeType
	}
	return &UserDetail{
		UserID:        derefStr(u.UserId),
		OpenID:        derefStr(u.OpenId),
		Name:          derefStr(u.Name),
		EnName:        derefStr(u.EnName),
		Email:         derefStr(u.Email),
		Mobile:        derefStr(u.Mobile),
		Avatar:        avatarURL,
		DepartmentIDs: u.DepartmentIds,
		JobTitle:      derefStr(u.JobTitle),
		WorkStation:   derefStr(u.WorkStation),
		City:          derefStr(u.City),
		EmployeeType:  employeeType,
	}, nil
}

// GetPrimaryCalendarID 获取当前租户的主日历 ID
func (c *Client) GetPrimaryCalendarID(ctx context.Context) (string, error) {
	req := larkcalendar.NewPrimaryCalendarReqBuilder().Build()

	resp, err := c.larkClient.Calendar.Calendar.Primary(ctx, req)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "获取主日历失败", err)
	}
	if !resp.Success() {
		return "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取主日历失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || len(resp.Data.Calendars) == 0 {
		return "", errs.New(errs.CodeChannelSendFailed, "未找到主日历")
	}
	cal := resp.Data.Calendars[0]
	if cal.Calendar == nil || cal.Calendar.CalendarId == nil {
		return "", errs.New(errs.CodeChannelSendFailed, "主日历 ID 为空")
	}
	return *cal.Calendar.CalendarId, nil
}

// ListCalendarEvents 获取日历事件列表
func (c *Client) ListCalendarEvents(ctx context.Context, calendarID string, startTime, endTime time.Time) ([]CalendarEvent, error) {
	req := larkcalendar.NewListCalendarEventReqBuilder().
		CalendarId(calendarID).
		StartTime(fmt.Sprintf("%d", startTime.Unix())).
		EndTime(fmt.Sprintf("%d", endTime.Unix())).
		PageSize(50).
		Build()

	resp, err := c.larkClient.Calendar.CalendarEvent.List(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取日历事件失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取日历事件失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}

	events := make([]CalendarEvent, 0)
	if resp.Data != nil {
		for _, item := range resp.Data.Items {
			var startTS, endTS, location, organizer, status string
			if item.StartTime != nil {
				startTS = derefStr(item.StartTime.Timestamp)
			}
			if item.EndTime != nil {
				endTS = derefStr(item.EndTime.Timestamp)
			}
			if item.Location != nil {
				location = derefStr(item.Location.Name)
			}
			organizer = derefStr(item.OrganizerCalendarId)
			status = derefStr(item.Status)

			attendees := make([]string, 0)
			if item.Attendees != nil {
				for _, a := range item.Attendees {
					if a.DisplayName != nil {
						attendees = append(attendees, *a.DisplayName)
					}
				}
			}

			events = append(events, CalendarEvent{
				EventID:     derefStr(item.EventId),
				Summary:     derefStr(item.Summary),
				Description: derefStr(item.Description),
				StartTime:   startTS,
				EndTime:     endTS,
				Location:    location,
				Organizer:   organizer,
				Status:      status,
				Attendees:   attendees,
			})
		}
	}
	return events, nil
}

// GetBotOpenID 获取当前应用机器人的 OpenID
// SDK 无对应方法，使用 RawRequest
func (c *Client) GetBotOpenID(ctx context.Context) (string, error) {
	// SDK-RAWREQUEST-ALLOWED: bot/v3/info 在 SDK v3.5.3 application v6 未暴露,只能走 SDK 内置 RawRequest。
	apiResp, err := c.larkClient.Get(ctx, "/open-apis/bot/v3/info",
		nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "获取机器人信息失败", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID string `json:"open_id"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "解析机器人信息响应失败", err)
	}
	if result.Code != 0 {
		return "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取机器人信息失败: code=%d, msg=%s", result.Code, result.Msg))
	}
	return result.Bot.OpenID, nil
}

// BotOpenID 同步获取并缓存 bot 自身 open_id。
// BotOpenID 同步获取并缓存 bot 自身 open_id。
//
// Phase 3 缺口 6 修复:不再用 sync.Once 固化失败状态。
//
// 行为:
//   - 命中缓存(atomic.Pointer 非空)→ 立即返回。
//   - 未命中 → 拿 mutex 拉取 API,成功则 atomic.Store 并返回;失败返空串
//     (logger.Warn 留痕),下次调用会再试。
//   - 5s timeout 防 caller 阻塞。
//   - botOpenIDMu 防多 goroutine 并发触发飞书 API thundering herd。
//
// 启动期 5s 内飞书 API 抖动场景:旧实现 sync.Once 永空 → bot 自反射防御失效。
// 现实现:webhook/longconn handler 每次进来都给一次重试机会,直到飞书恢复。
func (c *Client) BotOpenID() string {
	if cached := c.botOpenIDPtr.Load(); cached != nil && *cached != "" {
		return *cached
	}
	c.botOpenIDMu.Lock()
	defer c.botOpenIDMu.Unlock()
	// double-check:进 critical section 后另一个 goroutine 可能已经写入
	if cached := c.botOpenIDPtr.Load(); cached != nil && *cached != "" {
		return *cached
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := c.GetBotOpenID(ctx)
	if err != nil {
		c.logger.Warn("飞书 BotOpenID 获取失败,下次调用会再试",
			zap.Error(err))
		// 不缓存空值,让下次调用重试
		return ""
	}
	c.botOpenIDPtr.Store(&id)
	return id
}

// MessageDetail 是 GetMessageContent 返回的消息详情。
type MessageDetail struct {
	MessageID    string
	SenderOpenID string
	MessageType  string
	RawContent   string
	Text         string
	Refs         []imctx.DocRef
}

// GetMessageContent 获取指定消息的内容（用于父消息解析）。
// 返回消息正文、发送者 open_id、引用的文档资源。
func (c *Client) GetMessageContent(ctx context.Context, messageID string) (MessageDetail, error) {
	// Phase 4 缺口 7 修复:核心 API 包 rateLimiter.Wait + withRetry。
	// resolver 拉父消息走这条路径,飞书侧 99991400 限流时不重试 → resolver 返空 → prefix 缺父消息上下文。
	if err := c.rateLimiter.Wait(ctx, ""); err != nil {
		c.observeAPIError(err, time.Now())
		return MessageDetail{}, errs.Wrap(errs.CodeChannelSendFailed, "获取消息内容失败", err)
	}
	var detail MessageDetail
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewGetMessageReqBuilder().
			MessageId(messageID).
			Build()
		resp, innerErr := c.larkClient.Im.Message.Get(ctx, req)
		if innerErr != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "获取消息内容失败", innerErr)
		}
		if !resp.Success() {
			return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取消息失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		if resp.Data == nil || resp.Data.Items == nil || len(resp.Data.Items) == 0 {
			return errs.New(errs.CodeChannelSendFailed, "消息不存在或已删除")
		}
		item := resp.Data.Items[0]
		detail = MessageDetail{MessageID: messageID}
		if item.Sender != nil && item.Sender.Id != nil {
			detail.SenderOpenID = derefStr(item.Sender.Id)
		}
		detail.MessageType = derefStr(item.MsgType)
		if item.Body != nil && item.Body.Content != nil {
			detail.RawContent = derefStr(item.Body.Content)
			parsed := ParseInboundMessage(detail.MessageType, detail.RawContent)
			detail.Text = parsed.TextContent
			detail.Refs = parsed.References
		}
		// 某些“引用消息”父消息在 body.content 里只剩摘要；再对整条 Message JSON 扫描一遍，
		// 尽量吃到飞书塞在其它字段里的文档链接。
		if rawItem, marshalErr := json.Marshal(item); marshalErr == nil {
			if extraRefs := extractRefsFromAnyJSON(string(rawItem), "message"); len(extraRefs) > 0 {
				detail.Refs = deduplicateRefs(append(detail.Refs, extraRefs...))
			}
		}
		return nil
	})
	c.observeAPIError(err, time.Now())
	if err != nil {
		return MessageDetail{}, err
	}
	return detail, nil
}

// GetWikiNodeInfo 将 wiki token 转换为实际的 obj_token 和 obj_type。
// 飞书 wiki 是虚拟容器,真实内容是 docx/sheet/bitable 等,需要先查询节点信息获取真实类型。
//
// 修法历史:旧实现用 RawRequest 打 `/wiki/v2/spaces/by_token/{wikiToken}` —— 这条
// API 路径不存在,飞书返 HTML 404 → JSON Unmarshal 报 "invalid character 'p'"。
// 正确做法是 SDK 的 Wiki.Space.GetNode(token=...,obj_type=wiki),token-only 反查
// 节点详情,response 里带 obj_token / obj_type / space_id。
func (c *Client) GetWikiNodeInfo(ctx context.Context, wikiToken string) (objToken, objType string, err error) {
	req := larkwiki.NewGetNodeSpaceReqBuilder().
		Token(wikiToken).
		ObjType("wiki").
		Build()

	resp, apiErr := c.larkClient.Wiki.Space.GetNode(ctx, req)
	if apiErr != nil {
		return "", "", errs.Wrap(errs.CodeChannelSendFailed, "获取 wiki 节点信息失败", apiErr)
	}
	if !resp.Success() {
		return "", "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取 wiki 节点失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.Node == nil {
		return "", "", errs.New(errs.CodeChannelSendFailed, "获取 wiki 节点失败: 返回为空")
	}
	return derefStr(resp.Data.Node.ObjToken), derefStr(resp.Data.Node.ObjType), nil
}

// GetWikiNode 获取指定 wiki 节点详情。
func (c *Client) GetWikiNode(ctx context.Context, spaceID, nodeToken string) (json.RawMessage, error) {
	req := larkwiki.NewGetNodeSpaceReqBuilder().
		Token(nodeToken).
		ObjType("wiki").
		Build()

	resp, err := c.larkClient.Wiki.Space.GetNode(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取 wiki 节点失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取 wiki 节点失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.Node == nil {
		return nil, errs.New(errs.CodeChannelSendFailed, "获取 wiki 节点失败: 返回为空")
	}

	// SDK 的 GetNode(space) 是通过 token 反查节点详情，响应里已经带 space_id；
	// 这里额外校验调用方传入的 space_id，避免跨空间误读被静默接受。
	if resp.Data.Node.SpaceId != nil && *resp.Data.Node.SpaceId != "" && *resp.Data.Node.SpaceId != spaceID {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("wiki 节点不属于指定 space: expect=%s actual=%s", spaceID, *resp.Data.Node.SpaceId))
	}

	return json.Marshal(map[string]any{"node": resp.Data.Node})
}

// ResolveWikiNode 仅通过 wiki node token 反查节点详情。
func (c *Client) ResolveWikiNode(ctx context.Context, nodeToken string) (json.RawMessage, error) {
	req := larkwiki.NewGetNodeSpaceReqBuilder().
		Token(nodeToken).
		ObjType("wiki").
		Build()

	resp, err := c.larkClient.Wiki.Space.GetNode(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取 wiki 节点失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取 wiki 节点失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.Node == nil {
		return nil, errs.New(errs.CodeChannelSendFailed, "获取 wiki 节点失败: 返回为空")
	}
	return json.Marshal(map[string]any{"node": resp.Data.Node})
}

// ListWikiNodes 列出指定知识空间下的 wiki 节点。
func (c *Client) ListWikiNodes(ctx context.Context, spaceID, parentNodeToken string, count int) (json.RawMessage, error) {
	if count <= 0 {
		count = 20
	}
	reqBuilder := larkwiki.NewListSpaceNodeReqBuilder().
		SpaceId(spaceID).
		PageSize(count)
	if parentNodeToken != "" {
		reqBuilder = reqBuilder.ParentNodeToken(parentNodeToken)
	}

	resp, err := c.larkClient.Wiki.SpaceNode.List(ctx, reqBuilder.Build())
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取 wiki 节点列表失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取 wiki 节点列表失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil {
		return nil, errs.New(errs.CodeChannelSendFailed, "获取 wiki 节点列表失败: 返回为空")
	}

	return json.Marshal(map[string]any{
		"items":      resp.Data.Items,
		"page_token": resp.Data.PageToken,
		"has_more":   resp.Data.HasMore,
	})
}

// CreateDoc 创建新文档
func (c *Client) CreateDoc(ctx context.Context, title string, folderToken string) (string, string, error) {
	bodyBuilder := larkdocx.NewCreateDocumentReqBodyBuilder().Title(title)
	if folderToken != "" {
		bodyBuilder.FolderToken(folderToken)
	}

	req := larkdocx.NewCreateDocumentReqBuilder().
		Body(bodyBuilder.Build()).
		Build()

	resp, err := c.larkClient.Docx.Document.Create(ctx, req)
	if err != nil {
		return "", "", errs.Wrap(errs.CodeChannelSendFailed, "创建文档失败", err)
	}
	if !resp.Success() {
		return "", "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("创建文档失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || resp.Data.Document == nil || resp.Data.Document.DocumentId == nil {
		return "", "", errs.New(errs.CodeChannelSendFailed, "创建文档返回为空")
	}

	docID := *resp.Data.Document.DocumentId
	docURL := fmt.Sprintf("https://open.feishu.cn/docx/%s", docID)
	return docID, docURL, nil
}

// AppendDocContent 向文档追加文本内容
// documentID 同时作为根块的 blockID（飞书文档根块 ID == documentID）
func (c *Client) AppendDocContent(ctx context.Context, documentID string, content string) error {
	// 按换行拆分，每段生成一个 Text Block
	paragraphs := strings.Split(content, "\n")
	blocks := make([]*larkdocx.Block, 0, len(paragraphs))

	for _, para := range paragraphs {
		textRun := larkdocx.NewTextRunBuilder().Content(para).Build()
		element := larkdocx.NewTextElementBuilder().TextRun(textRun).Build()
		text := larkdocx.NewTextBuilder().Elements([]*larkdocx.TextElement{element}).Build()
		block := larkdocx.NewBlockBuilder().BlockType(2).Text(text).Build() // BlockType 2 = Text
		blocks = append(blocks, block)
	}

	req := larkdocx.NewCreateDocumentBlockChildrenReqBuilder().
		DocumentId(documentID).
		BlockId(documentID). // 根块 ID == documentID
		Body(larkdocx.NewCreateDocumentBlockChildrenReqBodyBuilder().
			Children(blocks).
			Build()).
		Build()

	resp, err := c.larkClient.Docx.DocumentBlockChildren.Create(ctx, req)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "追加文档内容失败", err)
	}
	if !resp.Success() {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("追加文档内容失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return nil
}

// --- 审批 API ---

// ListApprovalInstances 查询审批实例列表
func (c *Client) ListApprovalInstances(ctx context.Context, approvalCode string, startTime, endTime int64, pageSize int) (json.RawMessage, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	builder := larkapproval.NewListInstanceReqBuilder().
		ApprovalCode(approvalCode).
		PageSize(pageSize)
	if startTime > 0 {
		builder.StartTime(fmt.Sprintf("%d", startTime))
	}
	if endTime > 0 {
		builder.EndTime(fmt.Sprintf("%d", endTime))
	}
	req := builder.Build()

	resp, err := c.larkClient.Approval.Instance.List(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "查询审批实例失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("查询审批实例失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// GetApprovalInstance 获取审批实例详情
func (c *Client) GetApprovalInstance(ctx context.Context, instanceID string) (json.RawMessage, error) {
	req := larkapproval.NewGetInstanceReqBuilder().
		InstanceId(instanceID).
		Build()

	resp, err := c.larkClient.Approval.Instance.Get(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取审批详情失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取审批详情失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// CreateApprovalInstance 创建审批实例
func (c *Client) CreateApprovalInstance(ctx context.Context, approvalCode, openID, form string) (string, error) {
	req := larkapproval.NewCreateInstanceReqBuilder().
		InstanceCreate(larkapproval.NewInstanceCreateBuilder().
			ApprovalCode(approvalCode).
			OpenId(openID).
			Form(form).
			Build()).
		Build()

	resp, err := c.larkClient.Approval.Instance.Create(ctx, req)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "创建审批实例失败", err)
	}
	if !resp.Success() {
		return "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("创建审批实例失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.InstanceCode == nil {
		return "", errs.New(errs.CodeChannelSendFailed, "创建审批返回为空")
	}
	return *resp.Data.InstanceCode, nil
}

// --- 多维表格 API ---

// ListBitableRecords 列出多维表格记录
func (c *Client) ListBitableRecords(ctx context.Context, appToken, tableID string, pageSize int, filter string) (json.RawMessage, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	builder := larkbitable.NewListAppTableRecordReqBuilder().
		AppToken(appToken).
		TableId(tableID).
		PageSize(pageSize)
	if filter != "" {
		builder.Filter(filter)
	}
	req := builder.Build()

	resp, err := c.larkClient.Bitable.AppTableRecord.List(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "查询多维表格记录失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("查询多维表格记录失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// CreateBitableRecord 创建多维表格记录
func (c *Client) CreateBitableRecord(ctx context.Context, appToken, tableID string, fields map[string]interface{}) (json.RawMessage, error) {
	req := larkbitable.NewCreateAppTableRecordReqBuilder().
		AppToken(appToken).
		TableId(tableID).
		AppTableRecord(larkbitable.NewAppTableRecordBuilder().
			Fields(fields).
			Build()).
		Build()

	resp, err := c.larkClient.Bitable.AppTableRecord.Create(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "创建多维表格记录失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("创建多维表格记录失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// UpdateBitableRecord 更新多维表格记录
func (c *Client) UpdateBitableRecord(ctx context.Context, appToken, tableID, recordID string, fields map[string]interface{}) error {
	req := larkbitable.NewUpdateAppTableRecordReqBuilder().
		AppToken(appToken).
		TableId(tableID).
		RecordId(recordID).
		AppTableRecord(larkbitable.NewAppTableRecordBuilder().
			Fields(fields).
			Build()).
		Build()

	resp, err := c.larkClient.Bitable.AppTableRecord.Update(ctx, req)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "更新多维表格记录失败", err)
	}
	if !resp.Success() {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("更新多维表格记录失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return nil
}

// ListBitableTables 列出多维表格的数据表
func (c *Client) ListBitableTables(ctx context.Context, appToken string) (json.RawMessage, error) {
	req := larkbitable.NewListAppTableReqBuilder().
		AppToken(appToken).
		Build()

	resp, err := c.larkClient.Bitable.AppTable.List(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取数据表列表失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取数据表列表失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// --- 任务 API ---

// CreateTask 创建飞书任务
func (c *Client) CreateTask(ctx context.Context, summary string, dueTimestamp string) (json.RawMessage, error) {
	taskBuilder := larktask.NewTaskBuilder().
		Summary(summary)
	if dueTimestamp != "" {
		taskBuilder.Due(larktask.NewDueBuilder().
			Time(dueTimestamp).
			IsAllDay(false).
			Build())
	}

	req := larktask.NewCreateTaskReqBuilder().
		Task(taskBuilder.Build()).
		Build()

	resp, err := c.larkClient.Task.Task.Create(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "创建任务失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("创建任务失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// ListTasks 列出飞书任务
func (c *Client) ListTasks(ctx context.Context, pageSize int) (json.RawMessage, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	req := larktask.NewListTaskReqBuilder().
		PageSize(pageSize).
		Build()

	resp, err := c.larkClient.Task.Task.List(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取任务列表失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取任务列表失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// CompleteTask 完成飞书任务
func (c *Client) CompleteTask(ctx context.Context, taskID string) error {
	// v1 API: 通过 patch 设置 complete_time 来标记完成
	now := fmt.Sprintf("%d", time.Now().Unix())
	req := larktask.NewPatchTaskReqBuilder().
		TaskId(taskID).
		Body(larktask.NewPatchTaskReqBodyBuilder().
			Task(larktask.NewTaskBuilder().
				CompleteTime(now).
				Build()).
			UpdateFields([]string{"complete_time"}).
			Build()).
		Build()

	resp, err := c.larkClient.Task.Task.Patch(ctx, req)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "完成任务失败", err)
	}
	if !resp.Success() {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("完成任务失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return nil
}

// --- 电子表格 API ---

// ReadSheetRange 读取电子表格范围数据
func (c *Client) ReadSheetRange(ctx context.Context, spreadsheetToken, sheetRange string) (json.RawMessage, error) {
	resolvedRange := sheetRange
	if !strings.Contains(sheetRange, "!") {
		sheetID, err := c.firstSheetID(ctx, spreadsheetToken)
		if err != nil {
			return nil, err
		}
		resolvedRange = sheetID + "!" + sheetRange
	}

	// SDK-RAWREQUEST-ALLOWED: sheets v2 values 读 API,SDK v3.5.3 sheets/v3 未暴露 values 操作。
	apiResp, err := c.larkClient.Get(ctx,
		fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values/%s", spreadsheetToken, resolvedRange),
		nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "读取表格数据失败", err)
	}

	var result struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "解析表格响应失败", err)
	}
	if result.Code != 0 {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("读取表格失败: code=%d, msg=%s", result.Code, result.Msg))
	}
	return result.Data, nil
}

func (c *Client) firstSheetID(ctx context.Context, spreadsheetToken string) (string, error) {
	req := larksheets.NewQuerySpreadsheetSheetReqBuilder().
		SpreadsheetToken(spreadsheetToken).
		Build()

	resp, err := c.larkClient.Sheets.SpreadsheetSheet.Query(ctx, req)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "获取工作表列表失败", err)
	}
	if !resp.Success() {
		return "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取工作表列表失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || len(resp.Data.Sheets) == 0 {
		return "", errs.New(errs.CodeChannelSendFailed, "获取工作表列表失败: 返回为空")
	}
	for _, sheet := range resp.Data.Sheets {
		if sheet != nil && sheet.SheetId != nil && *sheet.SheetId != "" {
			return *sheet.SheetId, nil
		}
	}
	return "", errs.New(errs.CodeChannelSendFailed, "获取工作表列表失败: 未找到 sheet_id")
}

// WriteSheetRange 写入电子表格范围数据
func (c *Client) WriteSheetRange(ctx context.Context, spreadsheetToken, sheetRange string, values [][]interface{}) error {
	body := map[string]interface{}{
		"valueRange": map[string]interface{}{
			"range":  sheetRange,
			"values": values,
		},
	}
	// SDK-RAWREQUEST-ALLOWED: sheets v2 values 写 API,SDK v3.5.3 sheets/v3 未暴露 values 操作。
	apiResp, err := c.larkClient.Put(ctx,
		fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/values", spreadsheetToken),
		body, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "写入表格数据失败", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "解析写入响应失败", err)
	}
	if result.Code != 0 {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("写入表格失败: code=%d, msg=%s", result.Code, result.Msg))
	}
	return nil
}

// --- 群管理 API ---

// GetChatInfo 获取群聊信息
func (c *Client) GetChatInfo(ctx context.Context, chatID string) (json.RawMessage, error) {
	req := larkim.NewGetChatReqBuilder().
		ChatId(chatID).
		Build()

	resp, err := c.larkClient.Im.Chat.Get(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取群聊信息失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取群聊信息失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

// ListChatMembers 获取群成员列表
func (c *Client) ListChatMembers(ctx context.Context, chatID string, pageSize int) (json.RawMessage, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	req := larkim.NewGetChatMembersReqBuilder().
		ChatId(chatID).
		PageSize(pageSize).
		Build()

	resp, err := c.larkClient.Im.ChatMembers.Get(ctx, req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeChannelSendFailed, "获取群成员列表失败", err)
	}
	if !resp.Success() {
		return nil, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("获取群成员列表失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return json.Marshal(resp.Data)
}

func (c *Client) IsGroupAdmin(ctx context.Context, tenantKey, chatID, openID string) (bool, error) {
	if chatID == "" || openID == "" {
		return false, nil
	}
	raw, err := c.GetChatInfo(ctx, chatID)
	if err != nil {
		return false, err
	}
	var info struct {
		OwnerID           *string  `json:"owner_id"`
		UserManagerIDList []string `json:"user_manager_id_list"`
		BotManagerIDList  []string `json:"bot_manager_id_list"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return false, err
	}
	if info.OwnerID != nil && *info.OwnerID == openID {
		return true, nil
	}
	for _, id := range info.UserManagerIDList {
		if id == openID {
			return true, nil
		}
	}
	for _, id := range info.BotManagerIDList {
		if id == openID {
			return true, nil
		}
	}
	return false, nil
}

// AddReaction 给指定消息添加表情回复
func (c *Client) AddReaction(ctx context.Context, messageID, emojiType string) error {
	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(emojiType).Build()).
			Build()).
		Build()

	resp, err := c.larkClient.Im.MessageReaction.Create(ctx, req)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "添加表情回复失败", err)
	}
	if !resp.Success() {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("添加表情回复失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}
	return nil
}

// ErrPatchRateLimited 飞书 PatchCard 触发限流（HTTP 429 / code 99991400 等）。
// renderer 上层据此做 300ms 合并节流或退避重试。
var ErrPatchRateLimited = errs.New(errs.CodeChannelSendFailed, "飞书 PatchCard 触发限流")

// PatchCard 增量更新已发送的交互卡片。
// messageID 为首次 SendMessage/ReplyMessage 返回的消息 ID；cardJSON 为完整卡片 JSON（飞书只支持整卡替换，不支持字段级 patch）。
// 429 → 返回 ErrPatchRateLimited 哨兵错误，便于 renderer 层节流判断。
func (c *Client) PatchCard(ctx context.Context, messageID, cardJSON string) error {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return err
	}
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewPatchMessageReqBuilder().
			MessageId(messageID).
			Body(larkim.NewPatchMessageReqBodyBuilder().
				Content(cardJSON).
				Build()).
			Build()

		resp, err := c.larkClient.Im.Message.Patch(ctx, req)
		if err != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "更新卡片失败", err)
		}
		if !resp.Success() {
			if resp.Code == 230020 || resp.Code == 99991400 {
				return ErrPatchRateLimited
			}
			return errors.New(fmt.Sprintf("更新卡片失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		return nil
	})
	c.observeAPIError(err, time.Now())
	return err
}

// ReplyCard 回复一张交互卡片，返回飞书生成的 message_id 供后续 PatchCard 使用。
// 与 ReplyMessage 区别：后者 discard 了 resp.Data.MessageId；renderer 需要这个 ID 做整卡替换。
func (c *Client) ReplyCard(ctx context.Context, replyToID, cardJSON string) (string, error) {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return "", err
	}
	var messageID string
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewReplyMessageReqBuilder().
			MessageId(replyToID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType("interactive").
				Content(cardJSON).
				Build()).
			Build()
		resp, err := c.larkClient.Im.Message.Reply(ctx, req)
		if err != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "回复卡片失败", err)
		}
		if !resp.Success() {
			return errors.New(fmt.Sprintf("回复卡片失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		if resp.Data == nil || resp.Data.MessageId == nil {
			return errs.New(errs.CodeChannelSendFailed, "回复卡片成功但未返回 message_id")
		}
		messageID = *resp.Data.MessageId
		return nil
	})
	if err != nil {
		c.observeAPIError(err, time.Now())
		return "", err
	}
	return messageID, nil
}

// SendCard 向指定 chat 发送一张交互卡片，返回飞书生成的 message_id。
// 当 ReplyCard 因 replyToID 失效（如长任务后消息 ID 过期）而失败时，由 renderer 层 fallback 到本方法。
func (c *Client) SendCard(ctx context.Context, chatID, cardJSON string) (string, error) {
	if err := c.checkDegradedOutbound(); err != nil {
		c.observeAPIError(err, time.Now())
		return "", err
	}
	if err := c.rateLimiter.Wait(ctx, chatID); err != nil {
		c.observeAPIError(err, time.Now())
		return "", errs.Wrap(errs.CodeChannelSendFailed, "发送卡片失败", err)
	}
	// 同 SendMessage:按 chatID 前缀自动切 receive_id_type。
	idType, normalizedID := inferReceiveIDType(chatID)
	var messageID string
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(idType).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(normalizedID).
				MsgType("interactive").
				Content(cardJSON).
				Build()).
			Build()
		resp, err := c.larkClient.Im.Message.Create(ctx, req)
		if err != nil {
			return errs.Wrap(errs.CodeChannelSendFailed, "发送卡片失败", err)
		}
		if !resp.Success() {
			return errors.New(fmt.Sprintf("发送卡片失败: code=%d, msg=%s", resp.Code, resp.Msg))
		}
		if resp.Data == nil || resp.Data.MessageId == nil {
			return errs.New(errs.CodeChannelSendFailed, "发送卡片成功但未返回 message_id")
		}
		messageID = *resp.Data.MessageId
		return nil
	})
	if err != nil {
		c.observeAPIError(err, time.Now())
		return "", err
	}
	return messageID, nil
}

// DownloadMessageResource 下载消息中的资源（图片/文件/音视频）。
// type_ 可选值：image / file / audio / video / media
// 返回 io.Reader 和文件名，调用方负责读取和关闭。
func (c *Client) DownloadMessageResource(ctx context.Context, messageID, fileKey, type_ string) ([]byte, string, error) {
	// Phase 4 缺口 7 修复:核心 API 包 rateLimiter.Wait + withRetry。
	// agent 用 download_message_resource 工具拉资源走这条路径,限流/5xx 时不重试 → agent 拿不到附件。
	if err := c.rateLimiter.Wait(ctx, ""); err != nil {
		c.observeAPIError(err, time.Now())
		return nil, "", fmt.Errorf("download message resource rate limit: %w", err)
	}
	var (
		data     []byte
		fileName string
	)
	err := withRetry(ctx, c.maxRetries, c.logger, func() error {
		req := larkim.NewGetMessageResourceReqBuilder().
			MessageId(messageID).
			FileKey(fileKey).
			Type(type_).
			Build()
		resp, innerErr := c.larkClient.Im.MessageResource.Get(ctx, req)
		if innerErr != nil {
			return fmt.Errorf("download message resource failed: %w", innerErr)
		}
		if !resp.Success() {
			return fmt.Errorf("download message resource API error: code=%d msg=%s", resp.Code, resp.Msg)
		}
		// 读取全部内容到内存（飞书消息资源通常不大，<20MB）
		buf := make([]byte, 32*1024)
		acc := make([]byte, 0, 1024*1024)
		for {
			n, readErr := resp.File.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
			}
			if readErr != nil {
				if readErr.Error() == "EOF" {
					break
				}
				return fmt.Errorf("read resource stream failed: %w", readErr)
			}
		}
		data = acc
		fileName = resp.FileName
		return nil
	})
	c.observeAPIError(err, time.Now())
	if err != nil {
		return nil, "", err
	}
	return data, fileName, nil
}

// ListGapMessages 按显式 container + 时间窗口拉取一页历史消息。
// chat 集合由外部驱动；client 层只负责单 chat 单页请求。
func (c *Client) ListGapMessages(ctx context.Context, req GapFetchRequest) (GapFetchPageResponse, error) {
	apiReq, err := req.BuildListMessageReq()
	if err != nil {
		return GapFetchPageResponse{}, err
	}

	resp, err := c.larkClient.Im.Message.List(ctx, apiReq)
	if err != nil {
		return GapFetchPageResponse{}, errs.Wrap(errs.CodeChannelSendFailed, "gap fetch 拉取消息失败", err)
	}
	if !resp.Success() {
		return GapFetchPageResponse{}, errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("gap fetch 拉取消息失败: code=%d, msg=%s", resp.Code, resp.Msg))
	}

	page := GapFetchPageResponse{}
	if resp.Data == nil {
		return page, nil
	}
	if resp.Data.HasMore != nil {
		page.HasMore = *resp.Data.HasMore
	}
	if resp.Data.PageToken != nil {
		page.NextPageToken = *resp.Data.PageToken
	}
	if len(resp.Data.Items) == 0 {
		return page, nil
	}

	page.Items = make([]GapFetchMessage, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		msg := GapFetchMessage{Raw: item}
		if item != nil && item.MessageId != nil {
			msg.MessageID = *item.MessageId
		}
		page.Items = append(page.Items, msg)
	}
	return page, nil
}

// derefStr 安全解引用字符串指针
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
