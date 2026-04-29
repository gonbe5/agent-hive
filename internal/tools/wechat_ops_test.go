package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// ---------------------------------------------------------------------------
// mockWechatOpsProvider 模拟微信操作提供者
// ---------------------------------------------------------------------------

type mockWechatOpsProvider struct {
	mu       sync.Mutex
	calls    []string // 记录被调用的方法名
	loggedIn bool     // 模拟登录状态
}

func (m *mockWechatOpsProvider) record(method string) {
	m.mu.Lock()
	m.calls = append(m.calls, method)
	m.mu.Unlock()
}

func (m *mockWechatOpsProvider) called(method string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.calls {
		if c == method {
			return true
		}
	}
	return false
}

// --- 状态 ---

func (m *mockWechatOpsProvider) IsLoggedIn() bool {
	m.record("IsLoggedIn")
	return m.loggedIn
}

// --- 消息 ---

func (m *mockWechatOpsProvider) SendImageMessage(_ context.Context, _, _ string) error {
	m.record("SendImageMessage")
	return nil
}

func (m *mockWechatOpsProvider) SendFileMessage(_ context.Context, _, _, _ string) error {
	m.record("SendFileMessage")
	return nil
}

func (m *mockWechatOpsProvider) SendEmojiMessage(_ context.Context, _ string, _ string, _ int) error {
	m.record("SendEmojiMessage")
	return nil
}

func (m *mockWechatOpsProvider) SendCardMessage(_ context.Context, _, _ string) error {
	m.record("SendCardMessage")
	return nil
}

func (m *mockWechatOpsProvider) RevokeMessage(_ context.Context, _, _ string) error {
	m.record("RevokeMessage")
	return nil
}

func (m *mockWechatOpsProvider) ForwardImage(_ context.Context, _, _ string) error {
	m.record("ForwardImage")
	return nil
}

func (m *mockWechatOpsProvider) ForwardVideo(_ context.Context, _, _ string) error {
	m.record("ForwardVideo")
	return nil
}

// --- 联系人 ---

func (m *mockWechatOpsProvider) GetFriendList(_ context.Context) (interface{}, error) {
	m.record("GetFriendList")
	return []map[string]string{
		{"wxid": "test_friend_1", "nickname": "好友A"},
		{"wxid": "test_friend_2", "nickname": "好友B"},
	}, nil
}

func (m *mockWechatOpsProvider) GetContactDetails(_ context.Context, _ []string) (interface{}, error) {
	m.record("GetContactDetails")
	return map[string]string{"wxid": "test_user", "nickname": "测试用户"}, nil
}

func (m *mockWechatOpsProvider) SearchContact(_ context.Context, keyword string) (interface{}, error) {
	m.record("SearchContact")
	return map[string]string{"keyword": keyword, "wxid": "found_user"}, nil
}

func (m *mockWechatOpsProvider) AddFriend(_ context.Context, _, _ string) error {
	m.record("AddFriend")
	return nil
}

func (m *mockWechatOpsProvider) AcceptFriend(_ context.Context, _, _ string) error {
	m.record("AcceptFriend")
	return nil
}

func (m *mockWechatOpsProvider) DeleteFriend(_ context.Context, _ string) error {
	m.record("DeleteFriend")
	return nil
}

// --- 群管理 ---

func (m *mockWechatOpsProvider) CreateGroup(_ context.Context, _ []string) error {
	m.record("CreateGroup")
	return nil
}

func (m *mockWechatOpsProvider) GetGroupList(_ context.Context) (interface{}, error) {
	m.record("GetGroupList")
	return []map[string]string{
		{"group_wxid": "group_1", "name": "测试群1"},
	}, nil
}

func (m *mockWechatOpsProvider) GetGroupDetail(_ context.Context, _ string) (interface{}, error) {
	m.record("GetGroupDetail")
	return map[string]string{"group_wxid": "group_1", "name": "测试群1"}, nil
}

func (m *mockWechatOpsProvider) GetGroupMembers(_ context.Context, _ string) (interface{}, error) {
	m.record("GetGroupMembers")
	return []map[string]string{{"wxid": "member_1"}}, nil
}

func (m *mockWechatOpsProvider) InviteToGroup(_ context.Context, _ string, _ []string) error {
	m.record("InviteToGroup")
	return nil
}

func (m *mockWechatOpsProvider) RemoveFromGroup(_ context.Context, _ string, _ []string) error {
	m.record("RemoveFromGroup")
	return nil
}

func (m *mockWechatOpsProvider) SetGroupName(_ context.Context, _, _ string) error {
	m.record("SetGroupName")
	return nil
}

func (m *mockWechatOpsProvider) SetGroupAnnouncement(_ context.Context, _, _ string) error {
	m.record("SetGroupAnnouncement")
	return nil
}

func (m *mockWechatOpsProvider) GetGroupQRCode(_ context.Context, _ string) (string, error) {
	m.record("GetGroupQRCode")
	return "https://qr.example.com/group_1", nil
}

func (m *mockWechatOpsProvider) QuitGroup(_ context.Context, _ string) error {
	m.record("QuitGroup")
	return nil
}

// --- 用户 ---

func (m *mockWechatOpsProvider) GetProfile(_ context.Context) (interface{}, error) {
	m.record("GetProfile")
	return map[string]string{"wxid": "my_wxid", "nickname": "我的昵称"}, nil
}

func (m *mockWechatOpsProvider) SetNickname(_ context.Context, _ string) error {
	m.record("SetNickname")
	return nil
}

func (m *mockWechatOpsProvider) SetSignature(_ context.Context, _ string) error {
	m.record("SetSignature")
	return nil
}

func (m *mockWechatOpsProvider) ModifyRemark(_ context.Context, _, _ string) error {
	m.record("ModifyRemark")
	return nil
}

// --- 朋友圈 ---

func (m *mockWechatOpsProvider) PostMoment(_ context.Context, _ string, _ []string) error {
	m.record("PostMoment")
	return nil
}

func (m *mockWechatOpsProvider) GetTimeline(_ context.Context) (interface{}, error) {
	m.record("GetTimeline")
	return []map[string]string{{"id": "sns_1", "content": "朋友圈内容"}}, nil
}

func (m *mockWechatOpsProvider) GetUserMoments(_ context.Context, _ string) (interface{}, error) {
	m.record("GetUserMoments")
	return []map[string]string{{"id": "sns_2"}}, nil
}

func (m *mockWechatOpsProvider) LikeMoment(_ context.Context, _ string) error {
	m.record("LikeMoment")
	return nil
}

func (m *mockWechatOpsProvider) CommentMoment(_ context.Context, _, _ string) error {
	m.record("CommentMoment")
	return nil
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// newWechatTestHost 创建注册了微信操作工具的测试 Host
func newWechatTestHost(t *testing.T, provider *mockWechatOpsProvider) *mcphost.Host {
	t.Helper()
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	RegisterWechatOpsTools(host, logger, provider)
	return host
}

// execTool 执行工具并返回结果文本，失败时调用 t.Fatal
func execTool(t *testing.T, host *mcphost.Host, toolName string, params interface{}) (string, bool) {
	t.Helper()
	input, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("序列化参数失败: %v", err)
	}
	result, err := host.ExecuteTool(context.Background(), toolName, input)
	if err != nil {
		t.Fatalf("ExecuteTool(%s) 失败: %v", toolName, err)
	}
	var content string
	json.Unmarshal(result.Content, &content)
	return content, result.IsError
}

// ---------------------------------------------------------------------------
// TestRegisterWechatOpsTools 注册相关测试
// ---------------------------------------------------------------------------

func TestRegisterWechatOpsTools_NilProvider(t *testing.T) {
	// 传入 nil 应返回 0
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	count := RegisterWechatOpsTools(host, logger, nil)
	if count != 0 {
		t.Errorf("传入 nil 期望返回 0, 实际 %d", count)
	}
}

func TestRegisterWechatOpsTools_InvalidProvider(t *testing.T) {
	// 传入非 WechatOpsProvider 类型应返回 0
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	count := RegisterWechatOpsTools(host, logger, "not_a_provider")
	if count != 0 {
		t.Errorf("传入无效类型期望返回 0, 实际 %d", count)
	}
}

func TestRegisterWechatOpsTools_ValidProvider(t *testing.T) {
	// 传入有效 mock 应注册 6 个工具
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	provider := &mockWechatOpsProvider{loggedIn: true}
	count := RegisterWechatOpsTools(host, logger, provider)
	if count != 6 {
		t.Errorf("传入有效 provider 期望返回 6, 实际 %d", count)
	}

	// 验证 6 个工具都已注册
	expectedTools := []string{
		"wechat_send_rich_message",
		"wechat_contacts",
		"wechat_groups",
		"wechat_profile",
		"wechat_moments",
		"wechat_status",
	}
	tools := host.ListTools()
	registered := make(map[string]bool)
	for _, tool := range tools {
		registered[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !registered[name] {
			t.Errorf("工具 %q 未注册", name)
		}
	}
}

// ---------------------------------------------------------------------------
// TestWechatContacts 联系人工具测试
// ---------------------------------------------------------------------------

func TestWechatContacts_ListAction(t *testing.T) {
	// 调用 wechat_contacts 的 list 操作，验证调用了 GetFriendList
	provider := &mockWechatOpsProvider{loggedIn: true}
	host := newWechatTestHost(t, provider)

	content, isErr := execTool(t, host, "wechat_contacts", map[string]string{
		"action": "list",
	})
	if isErr {
		t.Fatalf("list 操作不应返回错误: %s", content)
	}
	if !provider.called("GetFriendList") {
		t.Error("list 操作应调用 GetFriendList")
	}
	// 验证返回数据包含好友信息
	if !strings.Contains(content, "test_friend_1") {
		t.Errorf("返回数据应包含好友 wxid, 实际: %s", content)
	}
}

func TestWechatContacts_SearchAction(t *testing.T) {
	// 调用 search 操作，验证 keyword 被正确传递
	provider := &mockWechatOpsProvider{loggedIn: true}
	host := newWechatTestHost(t, provider)

	content, isErr := execTool(t, host, "wechat_contacts", map[string]string{
		"action":  "search",
		"keyword": "测试关键词",
	})
	if isErr {
		t.Fatalf("search 操作不应返回错误: %s", content)
	}
	if !provider.called("SearchContact") {
		t.Error("search 操作应调用 SearchContact")
	}
	// 验证关键词出现在响应中（mock 会把 keyword 放入返回值）
	if !strings.Contains(content, "测试关键词") {
		t.Errorf("返回数据应包含搜索关键词, 实际: %s", content)
	}
}

func TestWechatContacts_Actions(t *testing.T) {
	// 表驱动测试：联系人工具的多种操作
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantMethod string
		wantErr    bool
		wantText   string // 结果应包含的文本
	}{
		{
			name:     "detail_缺少wxids参数",
			params:   map[string]interface{}{"action": "detail"},
			wantErr:  true,
			wantText: "wxids 参数不能为空",
		},
		{
			name:       "detail_正常返回",
			params:     map[string]interface{}{"action": "detail", "wxids": []string{"user_1"}},
			wantMethod: "GetContactDetails",
			wantText:   "test_user",
		},
		{
			name:     "search_缺少keyword",
			params:   map[string]interface{}{"action": "search"},
			wantErr:  true,
			wantText: "keyword 参数不能为空",
		},
		{
			name:       "add_正常",
			params:     map[string]interface{}{"action": "add", "wxid": "new_friend", "verify_msg": "你好"},
			wantMethod: "AddFriend",
			wantText:   "好友请求已发送",
		},
		{
			name:     "add_缺少wxid",
			params:   map[string]interface{}{"action": "add"},
			wantErr:  true,
			wantText: "wxid 参数不能为空",
		},
		{
			name:       "accept_正常",
			params:     map[string]interface{}{"action": "accept", "encrypt_user": "enc_123", "ticket": "ticket_456"},
			wantMethod: "AcceptFriend",
			wantText:   "已同意好友请求",
		},
		{
			name:     "accept_缺少参数",
			params:   map[string]interface{}{"action": "accept", "encrypt_user": "enc_123"},
			wantErr:  true,
			wantText: "encrypt_user 和 ticket 参数不能为空",
		},
		{
			name:       "delete_正常",
			params:     map[string]interface{}{"action": "delete", "wxid": "del_user"},
			wantMethod: "DeleteFriend",
			wantText:   "联系人已删除",
		},
		{
			name:     "未知操作",
			params:   map[string]interface{}{"action": "unknown"},
			wantErr:  true,
			wantText: "未知操作",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockWechatOpsProvider{loggedIn: true}
			host := newWechatTestHost(t, provider)

			content, isErr := execTool(t, host, "wechat_contacts", tt.params)
			if isErr != tt.wantErr {
				t.Errorf("错误状态 = %v, 期望 %v, 内容: %s", isErr, tt.wantErr, content)
			}
			if tt.wantMethod != "" && !provider.called(tt.wantMethod) {
				t.Errorf("期望调用 %s 但未被调用", tt.wantMethod)
			}
			if tt.wantText != "" && !strings.Contains(content, tt.wantText) {
				t.Errorf("返回内容应包含 %q, 实际: %s", tt.wantText, content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWechatGroups 群管理工具测试
// ---------------------------------------------------------------------------

func TestWechatGroups_ListAction(t *testing.T) {
	// 调用 wechat_groups 的 list 操作
	provider := &mockWechatOpsProvider{loggedIn: true}
	host := newWechatTestHost(t, provider)

	content, isErr := execTool(t, host, "wechat_groups", map[string]string{
		"action": "list",
	})
	if isErr {
		t.Fatalf("list 操作不应返回错误: %s", content)
	}
	if !provider.called("GetGroupList") {
		t.Error("list 操作应调用 GetGroupList")
	}
	if !strings.Contains(content, "group_1") {
		t.Errorf("返回数据应包含群 ID, 实际: %s", content)
	}
}

func TestWechatGroups_Actions(t *testing.T) {
	// 表驱动测试：群管理工具的多种操作
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantMethod string
		wantErr    bool
		wantText   string
	}{
		{
			name:       "detail_正常",
			params:     map[string]interface{}{"action": "detail", "group_wxid": "group_1"},
			wantMethod: "GetGroupDetail",
			wantText:   "测试群1",
		},
		{
			name:     "detail_缺少group_wxid",
			params:   map[string]interface{}{"action": "detail"},
			wantErr:  true,
			wantText: "group_wxid 参数不能为空",
		},
		{
			name:       "members_正常",
			params:     map[string]interface{}{"action": "members", "group_wxid": "group_1"},
			wantMethod: "GetGroupMembers",
			wantText:   "member_1",
		},
		{
			name:       "create_正常",
			params:     map[string]interface{}{"action": "create", "wxids": []string{"user_1", "user_2"}},
			wantMethod: "CreateGroup",
			wantText:   "群聊创建成功",
		},
		{
			name:     "create_成员不足",
			params:   map[string]interface{}{"action": "create", "wxids": []string{"user_1"}},
			wantErr:  true,
			wantText: "至少需要 2 个成员",
		},
		{
			name:       "invite_正常",
			params:     map[string]interface{}{"action": "invite", "group_wxid": "group_1", "wxids": []string{"user_3"}},
			wantMethod: "InviteToGroup",
			wantText:   "邀请已发送",
		},
		{
			name:       "set_name_正常",
			params:     map[string]interface{}{"action": "set_name", "group_wxid": "group_1", "name": "新群名"},
			wantMethod: "SetGroupName",
			wantText:   "群名已更新",
		},
		{
			name:       "qrcode_正常",
			params:     map[string]interface{}{"action": "qrcode", "group_wxid": "group_1"},
			wantMethod: "GetGroupQRCode",
			wantText:   "qr.example.com",
		},
		{
			name:       "quit_正常",
			params:     map[string]interface{}{"action": "quit", "group_wxid": "group_1"},
			wantMethod: "QuitGroup",
			wantText:   "已退出群聊",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockWechatOpsProvider{loggedIn: true}
			host := newWechatTestHost(t, provider)

			content, isErr := execTool(t, host, "wechat_groups", tt.params)
			if isErr != tt.wantErr {
				t.Errorf("错误状态 = %v, 期望 %v, 内容: %s", isErr, tt.wantErr, content)
			}
			if tt.wantMethod != "" && !provider.called(tt.wantMethod) {
				t.Errorf("期望调用 %s 但未被调用", tt.wantMethod)
			}
			if tt.wantText != "" && !strings.Contains(content, tt.wantText) {
				t.Errorf("返回内容应包含 %q, 实际: %s", tt.wantText, content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWechatStatus 状态查询工具测试
// ---------------------------------------------------------------------------

func TestWechatStatus_LoginStatus(t *testing.T) {
	// 表驱动测试：验证登录状态查询
	tests := []struct {
		name     string
		loggedIn bool
		wantText string
	}{
		{
			name:     "已登录状态",
			loggedIn: true,
			wantText: "true",
		},
		{
			name:     "未登录状态",
			loggedIn: false,
			wantText: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockWechatOpsProvider{loggedIn: tt.loggedIn}
			host := newWechatTestHost(t, provider)

			content, isErr := execTool(t, host, "wechat_status", map[string]string{
				"action": "login_status",
			})
			if isErr {
				t.Fatalf("wechat_status 不应返回错误: %s", content)
			}
			// 验证返回的 JSON 包含 logged_in 字段
			if !strings.Contains(content, "logged_in") {
				t.Errorf("返回内容应包含 logged_in 字段, 实际: %s", content)
			}
			if !strings.Contains(content, tt.wantText) {
				t.Errorf("返回内容应包含 %q, 实际: %s", tt.wantText, content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestWechatTool_NotLoggedIn 未登录状态测试
// ---------------------------------------------------------------------------

func TestWechatTool_NotLoggedIn(t *testing.T) {
	// 表驱动测试：所有需要登录的工具在未登录时应返回错误
	tests := []struct {
		name     string
		toolName string
		params   map[string]string
	}{
		{
			name:     "联系人工具未登录",
			toolName: "wechat_contacts",
			params:   map[string]string{"action": "list"},
		},
		{
			name:     "群管理工具未登录",
			toolName: "wechat_groups",
			params:   map[string]string{"action": "list"},
		},
		{
			name:     "个人资料工具未登录",
			toolName: "wechat_profile",
			params:   map[string]string{"action": "get"},
		},
		{
			name:     "朋友圈工具未登录",
			toolName: "wechat_moments",
			params:   map[string]string{"action": "timeline"},
		},
		{
			name:     "富媒体消息工具未登录",
			toolName: "wechat_send_rich_message",
			params:   map[string]string{"action": "send_image", "to_wxid": "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置未登录状态
			provider := &mockWechatOpsProvider{loggedIn: false}
			host := newWechatTestHost(t, provider)

			content, isErr := execTool(t, host, tt.toolName, tt.params)
			if !isErr {
				t.Errorf("未登录时 %s 应返回错误", tt.toolName)
			}
			if !strings.Contains(content, "微信未登录") {
				t.Errorf("错误消息应包含 '微信未登录', 实际: %s", content)
			}
		})
	}
}
