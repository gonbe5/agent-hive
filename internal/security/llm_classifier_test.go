package security

import (
	"context"
	"encoding/json"
	"testing"
)

// mockLLMClassifier 是 LLMClassifier 的 mock 版本（绕过真实 LLM 调用）
type mockLLMClassifier struct {
	result ClassifyResult
}

func (m *mockLLMClassifier) Classify(_ context.Context, _ string, _ json.RawMessage) ClassifyResult {
	return m.result
}

func TestClassifyResult_SafeTrue(t *testing.T) {
	c := &mockLLMClassifier{result: ClassifyResult{Safe: true, Reason: "only reads files"}}
	input := json.RawMessage(`{"file_path": "src/main.go"}`)
	result := c.Classify(context.Background(), "read_file", input)
	if !result.Safe {
		t.Errorf("expected Safe=true, got false (reason: %s)", result.Reason)
	}
}

func TestClassifyResult_SafeFalse(t *testing.T) {
	c := &mockLLMClassifier{result: ClassifyResult{Safe: false, Reason: "rm -rf is dangerous"}}
	input := json.RawMessage(`{"command": "rm -rf /tmp/test"}`)
	result := c.Classify(context.Background(), "bash", input)
	if result.Safe {
		t.Errorf("expected Safe=false for dangerous command")
	}
}
