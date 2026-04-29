package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/imctx"
)

func TestContextResolver_ResolveParentAndWiki(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	resolver := NewContextResolver(client, logger).WithTimeout(2 * time.Second)

	msg := &channel.InboundMessage{
		Platform:   channel.PlatformFeishu,
		TenantKey:  "tenant-a",
		MessageID:  "om_current",
		ChatID:     "oc_chat_1",
		SenderID:   "ou_sender",
		ParentID:   "om_parent_user",
		References: []imctx.DocRef{{Type: imctx.RefWiki, Token: "wiki_tok_123", Source: "url"}},
		Mentions:   []imctx.Mention{{Name: "助手", IsBot: true}},
		Timestamp:  time.Unix(1710000000, 0),
	}

	got, err := resolver.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil IM context")
	}
	if got.ParentMessageID != "om_parent_user" {
		t.Fatalf("ParentMessageID = %q, want om_parent_user", got.ParentMessageID)
	}
	if !strings.Contains(got.ParentContent, "父消息正文") {
		t.Fatalf("ParentContent = %q, want contains 父消息正文", got.ParentContent)
	}
	if len(got.References) != 2 {
		t.Fatalf("References len = %d, want 2; refs=%+v", len(got.References), got.References)
	}

	var foundWikiResolved bool
	var foundParentDoc bool
	for _, ref := range got.References {
		if ref.Token == "docx_from_wiki_123" && ref.Type == imctx.RefDocx {
			foundWikiResolved = true
		}
		if ref.Token == "docxparent456" && ref.Type == imctx.RefDocx && ref.Source == "parent" {
			foundParentDoc = true
		}
	}
	if !foundWikiResolved {
		t.Fatalf("wiki ref was not converted: %+v", got.References)
	}
	if !foundParentDoc {
		t.Fatalf("parent doc ref was not merged: %+v", got.References)
	}
	if !strings.Contains(got.SystemPromptPrefix, "<parent_message><![CDATA[父消息正文") {
		t.Fatalf("SystemPromptPrefix missing parent block: %s", got.SystemPromptPrefix)
	}
	if !strings.Contains(got.SystemPromptPrefix, `token="docx_from_wiki_123"`) {
		t.Fatalf("SystemPromptPrefix missing resolved wiki token: %s", got.SystemPromptPrefix)
	}
}

func TestContextResolver_DropsBotParentReflection(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	resolver := NewContextResolver(client, logger).WithTimeout(2 * time.Second)

	msg := &channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		MessageID: "om_current",
		ChatID:    "oc_chat_1",
		SenderID:  "ou_sender",
		ParentID:  "om_parent_bot",
		Timestamp: time.Unix(1710000000, 0),
	}

	got, err := resolver.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil IM context")
	}
	if got.ParentMessageID != "" || got.ParentContent != "" {
		t.Fatalf("bot reflection parent should be dropped, got %+v", got)
	}
}

func TestContextResolver_FallsBackToRootMessageWhenParentHasNoRefs(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	resolver := NewContextResolver(client, logger).WithTimeout(2 * time.Second)

	msg := &channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		MessageID: "om_current_root_fallback",
		ChatID:    "oc_chat_1",
		SenderID:  "ou_sender",
		ParentID:  "om_parent_stub",
		RootID:    "om_root_doc",
		Timestamp: time.Unix(1710000000, 0),
	}

	got, err := resolver.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil IM context")
	}

	var foundRootDoc bool
	for _, ref := range got.References {
		if ref.Token == "docxroot789" && ref.Type == imctx.RefDocx && ref.Source == "root" {
			foundRootDoc = true
			break
		}
	}
	if !foundRootDoc {
		t.Fatalf("root fallback doc ref was not merged: %+v", got.References)
	}
	if got.ParentMessageID != "om_parent_stub" {
		t.Fatalf("ParentMessageID = %q, want om_parent_stub", got.ParentMessageID)
	}
	if got.ParentContent != "引用了一条消息" {
		t.Fatalf("ParentContent = %q, want 引用了一条消息", got.ParentContent)
	}
}

func TestContextResolver_FillsSenderNameAndMentionNames(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	resolver := NewContextResolver(client, logger).WithTimeout(2 * time.Second)

	msg := &channel.InboundMessage{
		Platform:   channel.PlatformFeishu,
		TenantKey:  "tenant-a",
		MessageID:  "om_identity",
		ChatID:     "oc_chat_1",
		SenderID:   "ou_sender",
		SenderName: "ou_sender",
		Mentions: []imctx.Mention{
			{Name: "old-name", OpenID: "ou_mention_1"},
		},
		Timestamp: time.Unix(1710000000, 0),
	}

	got, err := resolver.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil IM context")
	}
	if msg.SenderName != "张三" {
		t.Fatalf("SenderName = %q, want 张三", msg.SenderName)
	}
	if len(got.Mentions) != 1 {
		t.Fatalf("Mentions len = %d, want 1", len(got.Mentions))
	}
	if got.Mentions[0].Name != "李四" {
		t.Fatalf("Mention name = %q, want 李四", got.Mentions[0].Name)
	}
}

func TestContextResolver_PrefersEnglishNamesWhenConfigured(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	resolver := NewContextResolver(client, logger).WithTimeout(2 * time.Second).WithNameLocale("en-US")

	msg := &channel.InboundMessage{
		Platform:   channel.PlatformFeishu,
		MessageID:  "om_identity_en",
		ChatID:     "oc_chat_1",
		SenderID:   "ou_sender",
		SenderName: "ou_sender",
		Mentions: []imctx.Mention{
			{Name: "old-name", OpenID: "ou_mention_1"},
		},
		Timestamp: time.Unix(1710000000, 0),
	}

	got, err := resolver.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil IM context")
	}
	if msg.SenderName != "Zhang San" {
		t.Fatalf("SenderName = %q, want Zhang San", msg.SenderName)
	}
	if got.Mentions[0].Name != "Li Si" {
		t.Fatalf("Mention name = %q, want Li Si", got.Mentions[0].Name)
	}
}

func TestContextResolver_DefaultsLocaleByRegionWhenNameLocaleUnset(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))
	resolver := NewContextResolver(client, logger).
		WithTimeout(2 * time.Second).
		WithRegion("intl")

	msg := &channel.InboundMessage{
		Platform:   channel.PlatformFeishu,
		MessageID:  "om_identity_region",
		ChatID:     "oc_chat_1",
		SenderID:   "ou_sender",
		SenderName: "ou_sender",
		Mentions: []imctx.Mention{
			{Name: "old-name", OpenID: "ou_mention_1"},
		},
		Timestamp: time.Unix(1710000000, 0),
	}

	got, err := resolver.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil IM context")
	}
	if msg.SenderName != "Zhang San" {
		t.Fatalf("SenderName = %q, want Zhang San", msg.SenderName)
	}
	if got.Mentions[0].Name != "Li Si" {
		t.Fatalf("Mention name = %q, want Li Si", got.Mentions[0].Name)
	}
}

func TestDownloadMessageResource_DownloadsFile(t *testing.T) {
	logger := zap.NewNop()
	server := newMockFeishuResolverServer(t)
	defer server.Close()

	client := NewClient("test_app_id", "test_secret", logger, lark.WithOpenBaseUrl(server.URL))

	got, err := DownloadMessageResource(context.Background(), client, DownloadRequest{
		MessageID: "om_current",
		FileKey:   "file_key_1",
		Type:      ResourceTypeFile,
	})
	if err != nil {
		t.Fatalf("DownloadMessageResource failed: %v", err)
	}
	if string(got.Data) != "%PDF-1.7 mock pdf bytes" {
		t.Fatalf("downloaded data mismatch: %q", string(got.Data))
	}
	if got.FileName != "report.pdf" {
		t.Fatalf("FileName = %q, want report.pdf", got.FileName)
	}
}

func newMockFeishuResolverServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "test_token",
				"expire":              7200,
			})
			return

		case r.URL.Path == "/open-apis/bot/v3/info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"bot": map[string]any{
					"open_id": "ou_bot_1",
				},
			})
			return

		case r.URL.Path == "/open-apis/contact/v3/users/ou_sender":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"user": map[string]any{
						"user_id": "user_sender",
						"open_id": "ou_sender",
						"name":    "张三",
						"en_name": "Zhang San",
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/contact/v3/users/ou_mention_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"user": map[string]any{
						"user_id": "user_mention_1",
						"open_id": "ou_mention_1",
						"name":    "李四",
						"en_name": "Li Si",
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/im/v1/messages/om_parent_user":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"items": []map[string]any{
						{
							"message_id": "om_parent_user",
							"msg_type":   "text",
							"sender": map[string]any{
								"id":          "ou_parent_user",
								"id_type":     "open_id",
								"sender_type": "user",
							},
							"body": map[string]any{
								"content": `{"text":"父消息正文 https://abc.feishu.cn/docx/docxparent456"}`,
							},
						},
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/im/v1/messages/om_parent_bot":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"items": []map[string]any{
						{
							"message_id": "om_parent_bot",
							"msg_type":   "text",
							"sender": map[string]any{
								"id":          "ou_bot_1",
								"id_type":     "open_id",
								"sender_type": "app",
							},
							"body": map[string]any{
								"content": `{"text":"这是 bot 自己发的父消息"}`,
							},
						},
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/im/v1/messages/om_parent_stub":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"items": []map[string]any{
						{
							"message_id": "om_parent_stub",
							"msg_type":   "text",
							"sender": map[string]any{
								"id":          "ou_parent_user",
								"id_type":     "open_id",
								"sender_type": "user",
							},
							"body": map[string]any{
								"content": `{"text":"引用了一条消息"}`,
							},
						},
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/im/v1/messages/om_root_doc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"items": []map[string]any{
						{
							"message_id": "om_root_doc",
							"msg_type":   "text",
							"sender": map[string]any{
								"id":          "ou_parent_user",
								"id_type":     "open_id",
								"sender_type": "user",
							},
							"body": map[string]any{
								"content": `{"text":"原始文档 https://abc.feishu.cn/docx/docxroot789"}`,
							},
						},
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/wiki/v2/spaces/get_node" && r.URL.Query().Get("token") == "wiki_tok_123":
			// 飞书 SDK Wiki.Space.GetNode(token=...,obj_type=wiki) 实际打的 endpoint。
			// 老 mock 用 /spaces/by_token/{token} 是错的 — 飞书没这条 API。
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"node": map[string]any{
						"obj_token": "docx_from_wiki_123",
						"obj_type":  "docx",
					},
				},
			})
			return

		case r.URL.Path == "/open-apis/im/v1/messages/om_current/resources/file_key_1":
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
			_, _ = w.Write([]byte("%PDF-1.7 mock pdf bytes"))
			return
		}

		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
	}))
}
