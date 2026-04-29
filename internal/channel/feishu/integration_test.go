package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"go.uber.org/zap"
)

// TestFeishuIntegrationWebhookFlow 测试飞书 Webhook 完整流程
func TestFeishuIntegrationWebhookFlow(t *testing.T) {
	logger := zap.NewNop()

	var receivedContent string
	var wg sync.WaitGroup
	wg.Add(1)
	processor := &mockProcessor{
		processFunc: func(ctx context.Context, sessionID, content string) (master.TaskResponse, error) {
			defer wg.Done()
			receivedContent = content
			return master.TaskResponse{
				Content: "收到：" + content,
			}, nil
		},
	}

	router := channel.NewRouter(processor, logger)

	// 创建飞书 Plugin
	cfg := config.FeishuConfig{
		AppID:             "cli_test123",
		AppSecret:         "test_secret",
		VerificationToken: "test_token",
	}

	plugin := New(cfg, router, logger)
	router.RegisterPlugin(plugin)

	// 绑定会话
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_test_chat",
		SessionID: "test_session",
	})

	// 模拟 Webhook 消息请求（使用正确的 open_id 字段）
	webhookBody := map[string]any{
		"header": map[string]any{
			"event_type": "im.message.receive_v1",
			"token":      "test_token",
		},
		"event": map[string]any{
			"message": map[string]any{
				"message_id":   "om_test_msg_123",
				"chat_id":      "oc_test_chat",
				"chat_type":    "p2p", // P0-#4 后 webhook/longconn 共用入口；缺省按群聊+@过滤
				"message_type": "text",
				"content":      `{"text":"测试消息"}`,
			},
			"sender": map[string]any{
				"sender_id": map[string]any{
					"open_id": "ou_test_user",
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(webhookBody)
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler := router.WebhookHandler(channel.PlatformFeishu)
	handler(rec, req)

	// 验证响应
	if rec.Code != http.StatusOK {
		t.Errorf("预期状态码 200，实际: %d", rec.Code)
	}

	// 等待异步消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("等待消息处理超时")
	}

	// 验证消息被正确处理
	if receivedContent != "测试消息" {
		t.Errorf("预期接收消息 '测试消息'，实际: %s", receivedContent)
	}
}

// TestFeishuIntegrationURLVerification 测试 URL 验证事件
func TestFeishuIntegrationURLVerification(t *testing.T) {
	logger := zap.NewNop()

	cfg := config.FeishuConfig{
		AppID:             "cli_test123",
		AppSecret:         "test_secret",
		VerificationToken: "test_token",
	}

	plugin := New(cfg, nil, logger)

	// 模拟 URL 验证请求
	verifyBody := map[string]any{
		"challenge": "test_challenge_string",
		"token":     "test_token",
		"type":      "url_verification",
	}

	bodyBytes, _ := json.Marshal(verifyBody)
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler := plugin.WebhookHandler()
	handler(rec, req)

	// 验证响应
	if rec.Code != http.StatusOK {
		t.Errorf("预期状态码 200，实际: %d", rec.Code)
	}

	// 验证返回 challenge
	var response map[string]any
	json.NewDecoder(rec.Body).Decode(&response)

	if response["challenge"] != "test_challenge_string" {
		t.Errorf("预期返回 challenge，实际: %v", response)
	}
}

// TestFeishuIntegrationSendMessage 测试通过 SDK 发送消息
func TestFeishuIntegrationSendMessage(t *testing.T) {
	logger := zap.NewNop()

	// 模拟飞书 API 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 令牌请求
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "test_token",
				"expire":              7200,
			})
			return
		}

		// 发送消息请求
		if strings.Contains(r.URL.Path, "messages") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"message_id": "om_test_msg",
				},
			})
			return
		}
	}))
	defer server.Close()

	// 创建客户端，使用 mock 服务器 URL
	client := NewClient("test_app_id", "test_secret", logger,
		lark.WithOpenBaseUrl(server.URL))

	// 发送消息
	err := client.SendTextMessage(context.Background(), "oc_test_chat", "Hello Feishu")
	if err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}
}

func TestFeishuIntegrationUploadImageAndFile(t *testing.T) {
	logger := zap.NewNop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "test_token",
				"expire":              7200,
			})
			return
		}

		if strings.Contains(r.URL.Path, "/images") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"image_key": "img_v3_uploaded",
				},
			})
			return
		}

		if strings.Contains(r.URL.Path, "/files") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"file_key": "file_v3_uploaded",
				},
			})
			return
		}
	}))
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))

	imageKey, err := client.UploadImage(context.Background(), []byte("fake-image-bytes"))
	if err != nil {
		t.Fatalf("UploadImage failed: %v", err)
	}
	if imageKey != "img_v3_uploaded" {
		t.Fatalf("UploadImage imageKey = %q, want img_v3_uploaded", imageKey)
	}

	fileKey, err := client.UploadFile(context.Background(), []byte("fake-file-bytes"), "report.pdf")
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if fileKey != "file_v3_uploaded" {
		t.Fatalf("UploadFile fileKey = %q, want file_v3_uploaded", fileKey)
	}
}

func TestFeishuIntegrationSendImageAndFile(t *testing.T) {
	logger := zap.NewNop()

	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "test_token",
				"expire":              7200,
			})
			return
		}

		if strings.Contains(r.URL.Path, "/messages") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			bodies = append(bodies, body)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{"message_id": "om_sent"},
			})
			return
		}
	}))
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	adapter := NewToolAdapter(client)

	if err := adapter.SendImage(context.Background(), "oc_test_chat", "img_v3_uploaded"); err != nil {
		t.Fatalf("SendImage failed: %v", err)
	}
	if err := adapter.SendFile(context.Background(), "oc_test_chat", "file_v3_uploaded"); err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("message create calls = %d, want 2", len(bodies))
	}
	if got := bodies[0]["msg_type"]; got != "image" {
		t.Fatalf("first msg_type = %v, want image", got)
	}
	if got := bodies[1]["msg_type"]; got != "file" {
		t.Fatalf("second msg_type = %v, want file", got)
	}
}

func TestFeishuIntegrationWikiGetNodeAndListNodes(t *testing.T) {
	logger := zap.NewNop()

	var requestedGetNode bool
	var requestedListNodes bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "test_token",
				"expire":              7200,
			})
			return
		}

		if r.URL.Path == "/open-apis/wiki/v2/spaces/get_node" {
			if got := r.URL.Query().Get("token"); got != "node_root" {
				t.Fatalf("get_node token = %q, want node_root", got)
			}
			if got := r.URL.Query().Get("obj_type"); got != "wiki" {
				t.Fatalf("get_node obj_type = %q, want wiki", got)
			}
			requestedGetNode = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"node": map[string]any{
						"space_id":   "space_alpha",
						"node_token": "node_root",
						"title":      "产品文档",
						"obj_type":   "docx",
						"obj_token":  "doc_123",
						"has_child":  true,
					},
				},
			})
			return
		}

		if r.URL.Path == "/open-apis/wiki/v2/spaces/space_alpha/nodes" {
			if got := r.URL.Query().Get("page_size"); got != "5" {
				t.Fatalf("list_nodes page_size = %q, want 5", got)
			}
			if got := r.URL.Query().Get("parent_node_token"); got != "node_root" {
				t.Fatalf("list_nodes parent_node_token = %q, want node_root", got)
			}
			requestedListNodes = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"items": []map[string]any{
						{
							"space_id":   "space_alpha",
							"node_token": "node_a",
							"title":      "目录A",
							"obj_type":   "docx",
							"obj_token":  "doc_a",
							"has_child":  true,
						},
						{
							"space_id":   "space_alpha",
							"node_token": "node_b",
							"title":      "目录B",
							"obj_type":   "sheet",
							"obj_token":  "sheet_b",
							"has_child":  false,
						},
					},
					"has_more": false,
				},
			})
			return
		}

		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	adapter := NewToolAdapter(client)

	nodeRaw, err := adapter.GetWikiNode(context.Background(), "space_alpha", "node_root")
	if err != nil {
		t.Fatalf("GetWikiNode failed: %v", err)
	}
	var nodeResult struct {
		Node struct {
			SpaceID   string `json:"space_id"`
			NodeToken string `json:"node_token"`
			Title     string `json:"title"`
			ObjType   string `json:"obj_type"`
		} `json:"node"`
	}
	if err := json.Unmarshal(nodeRaw, &nodeResult); err != nil {
		t.Fatalf("unmarshal get node result failed: %v", err)
	}
	if nodeResult.Node.SpaceID != "space_alpha" || nodeResult.Node.NodeToken != "node_root" || nodeResult.Node.Title != "产品文档" {
		t.Fatalf("unexpected node result: %+v", nodeResult.Node)
	}

	nodesRaw, err := adapter.ListWikiNodes(context.Background(), "space_alpha", "node_root", 5)
	if err != nil {
		t.Fatalf("ListWikiNodes failed: %v", err)
	}
	var nodesResult struct {
		Items []struct {
			NodeToken string `json:"node_token"`
			Title     string `json:"title"`
			ObjType   string `json:"obj_type"`
		} `json:"items"`
	}
	if err := json.Unmarshal(nodesRaw, &nodesResult); err != nil {
		t.Fatalf("unmarshal list nodes result failed: %v", err)
	}
	if len(nodesResult.Items) != 2 {
		t.Fatalf("ListWikiNodes item count = %d, want 2", len(nodesResult.Items))
	}
	if nodesResult.Items[0].NodeToken != "node_a" || nodesResult.Items[1].ObjType != "sheet" {
		t.Fatalf("unexpected list result: %+v", nodesResult.Items)
	}
	if !requestedGetNode || !requestedListNodes {
		t.Fatalf("wiki endpoints not fully requested: get=%v list=%v", requestedGetNode, requestedListNodes)
	}
}

func TestFeishuIntegrationReadSheetRangeResolvesDefaultSheetID(t *testing.T) {
	logger := zap.NewNop()

	var requestedSheetsQuery bool
	var requestedValues bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "test_token",
				"expire":              7200,
			})
			return
		}

		if r.URL.Path == "/open-apis/sheets/v3/spreadsheets/sht_123/sheets/query" {
			requestedSheetsQuery = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"sheets": []map[string]any{
						{
							"sheet_id": "gid_abc",
							"title":    "需求池",
							"index":    0,
						},
					},
				},
			})
			return
		}

		if r.URL.Path == "/open-apis/sheets/v2/spreadsheets/sht_123/values/gid_abc!A1:Z1000" {
			requestedValues = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"valueRange": map[string]any{
						"values": [][]string{{"标题", "状态"}, {"速度慢", "待处理"}},
					},
				},
			})
			return
		}

		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	adapter := NewToolAdapter(client)

	raw, err := adapter.ReadSheetRange(context.Background(), "sht_123", "A1:Z1000")
	if err != nil {
		t.Fatalf("ReadSheetRange failed: %v", err)
	}
	if !strings.Contains(string(raw), "速度慢") {
		t.Fatalf("unexpected sheet data: %s", raw)
	}
	if !requestedSheetsQuery || !requestedValues {
		t.Fatalf("sheet endpoints not fully requested: query=%v values=%v", requestedSheetsQuery, requestedValues)
	}
}

// mockProcessor 模拟消息处理器
type mockProcessor struct {
	processFunc func(ctx context.Context, sessionID, content string) (master.TaskResponse, error)
}

func (m *mockProcessor) ProcessMessage(ctx context.Context, sessionID, content string) (master.TaskResponse, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, sessionID, content)
	}
	return master.TaskResponse{Content: "ok"}, nil
}
