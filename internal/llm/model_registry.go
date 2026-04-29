package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
	"go.uber.org/zap"
)

// ModelRegistry 模型注册表，支持动态加载和远程获取
type ModelRegistry struct {
	mu        sync.RWMutex
	models    map[string]ModelMeta
	logger    *zap.Logger
	remoteURL string // 远程模型 API URL；为空则跳过远程获取
}

// NewModelRegistry 创建模型注册表，内置默认模型
func NewModelRegistry(logger *zap.Logger) *ModelRegistry {
	// 复制默认模型注册表
	defaults := make(map[string]ModelMeta, len(modelRegistry))
	for k, v := range modelRegistry {
		defaults[k] = v
	}

	return &ModelRegistry{
		models: defaults,
		logger: logger,
	}
}

// Get 获取模型元数据
func (r *ModelRegistry) Get(modelID string) *ModelMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.models[modelID]
	if !ok {
		return nil
	}
	return &meta
}

// List 列出所有已注册模型
func (r *ModelRegistry) List() map[string]ModelMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]ModelMeta, len(r.models))
	for k, v := range r.models {
		result[k] = v
	}
	return result
}

// Register 手动注册或覆盖模型
func (r *ModelRegistry) Register(modelID string, meta ModelMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models[modelID] = meta
	if r.logger != nil {
		r.logger.Debug("注册模型元数据",
			zap.String("model_id", modelID),
			zap.String("name", meta.Name),
		)
	}
}

// remoteModelEntry 远程 API 返回的模型条目结构
type remoteModelEntry struct {
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
	Capabilities  struct {
		Vision        bool `json:"vision"`
		Audio         bool `json:"audio"`
		PDF           bool `json:"pdf"`
		ToolUse       bool `json:"tool_use"`
		JSON          bool `json:"json"`
		Streaming     bool `json:"streaming"`
		Reasoning     bool `json:"reasoning"`
		PromptCaching bool `json:"prompt_caching"`
	} `json:"capabilities"`
}

// FetchRemote 从远程 API 获取模型元数据并合并。
// 如果未配置远程 URL，静默跳过（不产生 warn 日志）。
func (r *ModelRegistry) FetchRemote(ctx context.Context) error {
	if r.remoteURL == "" {
		if r.logger != nil {
			r.logger.Debug("未配置远程模型 URL，跳过远程获取")
		}
		return nil
	}

	// 远程获取超时从 5s 提升到 15s，避免慢网络下频繁降级到缓存
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, r.remoteURL, nil)
	if err != nil {
		return r.handleFetchError(fmt.Sprintf("创建请求失败: %v", err))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return r.handleFetchError(fmt.Sprintf("请求远程模型列表失败: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return r.handleFetchError(fmt.Sprintf("远程 API 返回非 200 状态码: %d", resp.StatusCode))
	}

	var entries []remoteModelEntry
	if decErr := json.NewDecoder(resp.Body).Decode(&entries); decErr != nil {
		// 解析失败，降级处理
		if r.logger != nil {
			r.logger.Warn("远程模型列表 JSON 解析失败，保留默认模型",
				zap.Error(decErr),
			)
		}
		return r.handleFetchError(fmt.Sprintf("解析远程模型列表失败: %v", decErr))
	}

	// 合并到注册表
	r.mu.Lock()
	for _, entry := range entries {
		if entry.Name == "" {
			continue
		}
		r.models[entry.Name] = ModelMeta{
			Name:          entry.Name,
			ContextWindow: entry.ContextWindow,
			MaxOutput:     entry.MaxOutput,
			Capabilities: ModelCapabilities{
				Vision:        entry.Capabilities.Vision,
				Audio:         entry.Capabilities.Audio,
				PDF:           entry.Capabilities.PDF,
				ToolUse:       entry.Capabilities.ToolUse,
				JSON:          entry.Capabilities.JSON,
				Streaming:     entry.Capabilities.Streaming,
				Reasoning:     entry.Capabilities.Reasoning,
				PromptCaching: entry.Capabilities.PromptCaching,
			},
		}
	}
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("远程模型元数据已合并",
			zap.Int("remote_count", len(entries)),
		)
	}

	// 保存缓存
	if cacheErr := r.saveCache(); cacheErr != nil && r.logger != nil {
		r.logger.Warn("保存模型缓存失败", zap.Error(cacheErr))
	}

	return nil
}

// handleFetchError 处理远程获取失败：尝试加载本地缓存，返回结构化错误
func (r *ModelRegistry) handleFetchError(msg string) error {
	if r.logger != nil {
		r.logger.Warn("远程模型获取失败，尝试加载本地缓存",
			zap.String("reason", msg),
		)
	}

	if cacheErr := r.loadCache(); cacheErr != nil {
		if r.logger != nil {
			r.logger.Warn("本地缓存加载也失败，使用内置默认模型",
				zap.Error(cacheErr),
			)
		}
		return errs.New(errs.CodeModelFetchFailed, msg)
	}

	// 缓存加载成功，记录日志但不返回错误
	if r.logger != nil {
		r.logger.Info("已从本地缓存加载模型元数据")
	}
	return nil
}

// cacheFilePath 返回缓存文件路径: ~/.claw/cache/models.json
func cacheFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	return filepath.Join(home, ".claw", "cache", "models.json"), nil
}

// cacheMaxAge 缓存最大有效期
const cacheMaxAge = 24 * time.Hour

// loadCache 从本地缓存加载模型元数据
func (r *ModelRegistry) loadCache() error {
	path, err := cacheFilePath()
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("缓存文件不存在: %w", err)
	}

	// 检查缓存是否过期
	if time.Since(info.ModTime()) > cacheMaxAge {
		return fmt.Errorf("缓存已过期（超过 %v）", cacheMaxAge)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取缓存文件失败: %w", err)
	}

	var cached map[string]ModelMeta
	if err := json.Unmarshal(data, &cached); err != nil {
		return fmt.Errorf("解析缓存 JSON 失败: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 合并缓存数据（缓存覆盖同名条目，保留本地独有条目）
	for k, v := range cached {
		r.models[k] = v
	}

	return nil
}

// saveCache 保存模型元数据到本地缓存
func (r *ModelRegistry) saveCache() error {
	path, err := cacheFilePath()
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}

	r.mu.RLock()
	data, err := json.MarshalIndent(r.models, "", "  ")
	r.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("序列化模型数据失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入缓存文件失败: %w", err)
	}

	return nil
}
