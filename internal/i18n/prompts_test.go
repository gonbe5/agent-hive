package i18n

import (
	"strings"
	"testing"
)

func TestPromptManager_GetPrompt(t *testing.T) {
	tests := []struct {
		name     string
		language string
		key      PromptKey
		wantLang string
	}{
		{
			name:     "中文代码审查提示词",
			language: "zh-CN",
			key:      PromptCodeReview,
			wantLang: "zh-CN",
		},
		{
			name:     "英文研究提示词",
			language: "en-US",
			key:      PromptResearch,
			wantLang: "en-US",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewPromptManager(tt.language)
			prompt := pm.GetPrompt(tt.key)

			if prompt == "" {
				t.Errorf("GetPrompt() returned empty string for key %s", tt.key)
				return
			}

			// 验证语言正确性
			if tt.wantLang == "zh-CN" && !containsChinese(prompt) {
				t.Errorf("Expected Chinese prompt but got: %s", prompt[:100])
			}
			if tt.wantLang == "en-US" && containsChinese(prompt) {
				t.Errorf("Expected English prompt but got: %s", prompt[:100])
			}
		})
	}
}

func TestPromptManager_SetLanguage(t *testing.T) {
	pm := NewPromptManager("en-US")

	// 测试初始语言
	prompt1 := pm.GetPrompt(PromptCodeReview)
	if containsChinese(prompt1) {
		t.Errorf("Expected English prompt initially")
	}

	// 切换到中文
	pm.SetLanguage("zh-CN")
	prompt2 := pm.GetPrompt(PromptCodeReview)
	if !containsChinese(prompt2) {
		t.Errorf("Expected Chinese prompt after SetLanguage")
	}

	// 切换回英文
	pm.SetLanguage("en-US")
	prompt3 := pm.GetPrompt(PromptCodeReview)
	if containsChinese(prompt3) {
		t.Errorf("Expected English prompt after switching back")
	}
}

func TestPromptManager_AllPromptsAvailable(t *testing.T) {
	pm := NewPromptManager("zh-CN")

	allKeys := []PromptKey{
		// 系统提示词
		PromptCodeReview,
		PromptResearch,
	}

	for _, key := range allKeys {
		t.Run(string(key), func(t *testing.T) {
			// 测试中文
			pm.SetLanguage("zh-CN")
			promptCN := pm.GetPrompt(key)
			if promptCN == "" {
				t.Errorf("Missing Chinese prompt for key %s", key)
			}

			// 测试英文
			pm.SetLanguage("en-US")
			promptEN := pm.GetPrompt(key)
			if promptEN == "" {
				t.Errorf("Missing English prompt for key %s", key)
			}

			// 确保两个版本不同
			if promptCN == promptEN {
				t.Errorf("Chinese and English prompts are identical for key %s", key)
			}
		})
	}
}

func TestPromptManager_GetLanguage(t *testing.T) {
	pm := NewPromptManager("zh-CN")

	if got := pm.GetLanguage(); got != "zh-CN" {
		t.Errorf("GetLanguage() = %v, want %v", got, "zh-CN")
	}

	pm.SetLanguage("en-US")
	if got := pm.GetLanguage(); got != "en-US" {
		t.Errorf("GetLanguage() = %v, want %v", got, "en-US")
	}
}

func TestPromptManager_FallbackToEnglish(t *testing.T) {
	pm := NewPromptManager("fr-FR") // 不存在的语言

	// 应该降级到英文
	prompt := pm.GetPrompt(PromptCodeReview)
	if prompt == "" {
		t.Error("Expected fallback to English, got empty string")
	}
	if containsChinese(prompt) {
		t.Error("Expected fallback to English, got Chinese")
	}
}

func TestPromptManager_ThreadSafety(t *testing.T) {
	pm := NewPromptManager("zh-CN")

	// 并发读写测试
	done := make(chan bool)

	// 启动多个 goroutine 读取
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = pm.GetPrompt(PromptCodeReview)
			}
			done <- true
		}()
	}

	// 启动多个 goroutine 写入
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					pm.SetLanguage("zh-CN")
				} else {
					pm.SetLanguage("en-US")
				}
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 20; i++ {
		<-done
	}
}

// containsChinese 检查字符串是否包含中文字符
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= '\u4e00' && r <= '\u9fa5' {
			return true
		}
	}
	return false
}

func BenchmarkPromptManager_GetPrompt(b *testing.B) {
	pm := NewPromptManager("zh-CN")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pm.GetPrompt(PromptCodeReview)
	}
}

func BenchmarkPromptManager_SetLanguage(b *testing.B) {
	pm := NewPromptManager("zh-CN")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			pm.SetLanguage("zh-CN")
		} else {
			pm.SetLanguage("en-US")
		}
	}
}

func TestPromptContent_CodeReview(t *testing.T) {
	pm := NewPromptManager("zh-CN")

	// 测试代码审查提示词包含关键元素
	prompt := pm.GetPrompt(PromptCodeReview)

	// 中文版本应该包含这些关键词
	keywords := []string{"审查", "代码"}
	for _, keyword := range keywords {
		if !strings.Contains(prompt, keyword) {
			t.Errorf("Chinese PromptCodeReview missing keyword: %s", keyword)
		}
	}

	// 切换到英文
	pm.SetLanguage("en-US")
	promptEN := pm.GetPrompt(PromptCodeReview)

	// 英文版本应该包含这些关键词
	keywordsEN := []string{"code", "review", "issues"}
	for _, keyword := range keywordsEN {
		if !strings.Contains(strings.ToLower(promptEN), keyword) {
			t.Errorf("English PromptCodeReview missing keyword: %s", keyword)
		}
	}
}
