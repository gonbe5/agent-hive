package i18n

import "sync"

// PromptKey 提示词键名
type PromptKey string

// ProviderKey LLM 提供商标识
type ProviderKey string

const (
	// 系统提示词
	PromptCodeReview   PromptKey = "system.code_review"
	PromptResearch     PromptKey = "system.research"
)

const (
	// ProviderClaude Anthropic Claude 系列模型（匹配 llm.DetectProvider 返回值 "anthropic"）
	ProviderClaude ProviderKey = "anthropic"
	// ProviderGPT OpenAI GPT 系列模型（匹配 llm.DetectProvider 返回值 "openai"）
	ProviderGPT ProviderKey = "openai"
	// ProviderGemini Google Gemini 系列模型（匹配 llm.DetectProvider 返回值 "google"）
	ProviderGemini ProviderKey = "google"
	// ProviderDeepSeek DeepSeek 系列模型
	ProviderDeepSeek ProviderKey = "deepseek"
	// ProviderDefault 默认提供商（通用模板）
	ProviderDefault ProviderKey = "default"
)

// PromptManager 管理提示词的国际化和模型感知选择
type PromptManager struct {
	mu              sync.RWMutex
	language        string
	prompts         map[PromptKey]map[string]string                 // key -> language -> prompt
	providerPrompts map[ProviderKey]map[PromptKey]map[string]string // provider -> key -> language -> prompt
}

// NewPromptManager 创建一个新的 PromptManager
func NewPromptManager(language string) *PromptManager {
	pm := &PromptManager{
		language:        language,
		prompts:         make(map[PromptKey]map[string]string),
		providerPrompts: make(map[ProviderKey]map[PromptKey]map[string]string),
	}
	pm.registerDefaultPrompts()
	pm.registerProviderPrompts()
	return pm
}

// GetPrompt 根据当前语言获取提示词（使用默认模板）
func (pm *PromptManager) GetPrompt(key PromptKey) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.resolvePromptLocked(pm.prompts[key])
}

// GetPromptForProvider 根据当前语言和指定 provider 获取提示词
// 如果该 provider 没有专门模板，则降级到默认模板
func (pm *PromptManager) GetPromptForProvider(key PromptKey, provider ProviderKey) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 优先从 provider 专属提示词中查找
	if providerMap, ok := pm.providerPrompts[provider]; ok {
		if translations, ok := providerMap[key]; ok {
			if result := pm.resolvePromptLocked(translations); result != "" {
				return result
			}
		}
	}

	// 降级到默认提示词
	return pm.resolvePromptLocked(pm.prompts[key])
}

// resolvePromptLocked 根据当前语言从翻译映射中解析提示词（调用方须持有读锁）
func (pm *PromptManager) resolvePromptLocked(translations map[string]string) string {
	if translations == nil {
		return ""
	}

	// 尝试获取当前语言的提示词
	if prompt, ok := translations[pm.language]; ok {
		return prompt
	}

	// 降级到英文
	if prompt, ok := translations["en-US"]; ok {
		return prompt
	}

	// 返回任意可用的提示词
	for _, prompt := range translations {
		return prompt
	}

	return ""
}

// SetLanguage 设置当前语言
func (pm *PromptManager) SetLanguage(lang string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.language = lang
}

// GetLanguage 获取当前语言
func (pm *PromptManager) GetLanguage() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.language
}

// RegisterPrompt 注册一个提示词的多语言版本
func (pm *PromptManager) RegisterPrompt(key PromptKey, translations map[string]string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.prompts[key] = translations
}

// RegisterProviderPrompt 注册某个 provider 的提示词变体
func (pm *PromptManager) RegisterProviderPrompt(provider ProviderKey, key PromptKey, translations map[string]string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.providerPrompts[provider] == nil {
		pm.providerPrompts[provider] = make(map[PromptKey]map[string]string)
	}
	pm.providerPrompts[provider][key] = translations
}

// registerDefaultPrompts 注册所有默认提示词
func (pm *PromptManager) registerDefaultPrompts() {
	for key, translations := range defaultPrompts {
		pm.RegisterPrompt(key, translations)
	}
}

// registerProviderPrompts 注册所有 provider 专属提示词
func (pm *PromptManager) registerProviderPrompts() {
	for provider, prompts := range providerSpecificPrompts {
		for key, translations := range prompts {
			pm.RegisterProviderPrompt(provider, key, translations)
		}
	}
}
