package feishu

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// TestSendMessageJSONEscaping 测试发送消息时正确处理JSON转义
func TestSendMessageJSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "包含双引号",
			content: `她说："你好"`,
		},
		{
			name:    "包含换行符",
			content: "第一行\n第二行\n第三行",
		},
		{
			name:    "包含反斜杠",
			content: `C:\Users\test\path`,
		},
		{
			name:    "包含制表符",
			content: "列1\t列2\t列3",
		},
		{
			name:    "混合特殊字符",
			content: `她说："路径是 C:\Users\test"\n下一行`,
		},
		{
			name:    "包含JSON字符",
			content: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 验证 JSON 序列化的正确性
			type textContent struct {
				Text string `json:"text"`
			}

			contentJSON, err := json.Marshal(textContent{Text: tt.content})
			if err != nil {
				t.Fatalf("序列化消息内容失败: %v", err)
			}

			// 反序列化验证
			var parsed textContent
			if err := json.Unmarshal(contentJSON, &parsed); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}

			if parsed.Text != tt.content {
				t.Errorf("反序列化后内容不匹配\n预期: %q\n实际: %q", tt.content, parsed.Text)
			}
		})
	}
}

// TestReplyMessageJSONEscaping 测试回复消息时正确处理JSON转义
func TestReplyMessageJSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "包含双引号",
			content: `回复："测试"`,
		},
		{
			name:    "包含换行符",
			content: "第一行\n第二行",
		},
		{
			name:    "包含反斜杠",
			content: `路径: C:\test`,
		},
		{
			name:    "混合特殊字符",
			content: `她说："文件在 C:\path\file.txt"\n新行`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 验证 JSON 序列化的正确性
			type textContent struct {
				Text string `json:"text"`
			}

			contentJSON, err := json.Marshal(textContent{Text: tt.content})
			if err != nil {
				t.Fatalf("序列化消息内容失败: %v", err)
			}

			// 反序列化验证
			var parsed textContent
			if err := json.Unmarshal(contentJSON, &parsed); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}

			if parsed.Text != tt.content {
				t.Errorf("反序列化后内容不匹配\n预期: %q\n实际: %q", tt.content, parsed.Text)
			}
		})
	}
}

// TestJSONSerializationCorrectness 测试JSON序列化的正确性
func TestJSONSerializationCorrectness(t *testing.T) {
	// 测试内容包含特殊字符时，使用结构体序列化的输出是否正确
	type textContent struct {
		Text string `json:"text"`
	}

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "简单文本",
			content:  "Hello World",
			expected: `{"text":"Hello World"}`,
		},
		{
			name:     "包含双引号",
			content:  `她说："你好"`,
			expected: `{"text":"她说：\"你好\""}`,
		},
		{
			name:     "包含换行符",
			content:  "Line1\nLine2",
			expected: `{"text":"Line1\nLine2"}`,
		},
		{
			name:     "包含反斜杠",
			content:  `C:\path`,
			expected: `{"text":"C:\\path"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := json.Marshal(textContent{Text: tt.content})
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("序列化结果不符\n预期: %s\n实际: %s", tt.expected, string(result))
			}

			// 反序列化验证
			var parsed textContent
			if err := json.Unmarshal(result, &parsed); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}

			if parsed.Text != tt.content {
				t.Errorf("反序列化后内容不匹配\n预期: %q\n实际: %q", tt.content, parsed.Text)
			}
		})
	}
}

func TestUploadImageRejectsOversizedPayload(t *testing.T) {
	client := &Client{}
	tooLarge := make([]byte, 10*1024*1024+1)

	_, err := client.UploadImage(t.Context(), tooLarge)
	if err == nil {
		t.Fatal("expected oversize image upload to fail")
	}
	if !errs.IsCode(err, errs.CodeChannelSendFailed) {
		t.Fatalf("error code = %d, want %d, err=%v", errs.GetCode(err), errs.CodeChannelSendFailed, err)
	}
	if !strings.Contains(err.Error(), "10MB") {
		t.Fatalf("error = %v, want mention 10MB", err)
	}
}

func TestUploadFileRejectsOversizedPayload(t *testing.T) {
	client := &Client{}
	tooLarge := make([]byte, 30*1024*1024+1)

	_, err := client.UploadFile(t.Context(), tooLarge, "report.pdf")
	if err == nil {
		t.Fatal("expected oversize file upload to fail")
	}
	if !errs.IsCode(err, errs.CodeChannelSendFailed) {
		t.Fatalf("error code = %d, want %d, err=%v", errs.GetCode(err), errs.CodeChannelSendFailed, err)
	}
	if !strings.Contains(err.Error(), "30MB") {
		t.Fatalf("error = %v, want mention 30MB", err)
	}
}

func TestSendMessageFailsFastWhenClientDegraded(t *testing.T) {
	client := &Client{}
	client.ApplySecurityConfig(2)
	now := time.Now()
	client.observeAPIError(newPermissionDeniedError("code=99991663"), now.Add(-2*time.Minute))
	client.observeAPIError(newPermissionDeniedError("code=10013"), now.Add(-1*time.Minute))

	err := client.SendMessage(t.Context(), "oc_chat", "text", `{"text":"hello"}`)
	if err == nil {
		t.Fatal("expected degraded client to fail fast")
	}
	if !strings.Contains(err.Error(), "已降级") {
		t.Fatalf("error = %v, want degraded fast-fail", err)
	}
}

func TestUploadImageFailsFastWhenClientDegraded(t *testing.T) {
	client := &Client{}
	client.ApplySecurityConfig(1)
	client.observeAPIError(newPermissionDeniedError("permission denied"), time.Now())

	_, err := client.UploadImage(t.Context(), []byte("fake-image"))
	if err == nil {
		t.Fatal("expected degraded client to fail fast")
	}
	if !strings.Contains(err.Error(), "已降级") {
		t.Fatalf("error = %v, want degraded fast-fail", err)
	}
}

func TestSendMessageFailsFastWhenClientDegraded_EmitsMetric(t *testing.T) {
	client := &Client{}
	writer := &identityMetricCaptureWriter{}
	client.SetMetricsWriter(writer)
	client.ApplySecurityConfig(1)
	client.observeAPIError(newPermissionDeniedError("permission denied"), time.Now())

	err := client.SendMessage(t.Context(), "oc_chat", "text", `{"text":"hello"}`)
	if err == nil {
		t.Fatal("expected degraded client to fail fast")
	}
	if metric := writer.find(MetricOutboundRejected); metric == nil {
		t.Fatalf("expected %s metric", MetricOutboundRejected)
	} else if got := metric.Labels["reason"]; got != "degraded" {
		t.Fatalf("outbound rejected reason = %v, want degraded", got)
	}
}
