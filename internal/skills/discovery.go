package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// SkillIndexEntry 描述远程 skill 仓库中的单个 skill（旧字段保持兼容，新字段均可选）
type SkillIndexEntry struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Files                []string `json:"files"`
	Version              string   `json:"version,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
	ProvidesRequirements []string `json:"provides_requirements,omitempty"`
	Checksum             string   `json:"checksum,omitempty"`
	ScopeHint            string   `json:"scope_hint,omitempty"` // "public" | "personal"
}

// SkillIndex 是远程 skill 仓库的索引文件结构
type SkillIndex struct {
	Skills []SkillIndexEntry `json:"skills"`
}

// ResolvedSkill 是 Discovery 解析结果，供 skill_install / spec_resolver 使用
type ResolvedSkill struct {
	Entry  SkillIndexEntry
	Source string // marketplace URL
}

// cachedIndex 带 TTL 的 index.json 缓存
type cachedIndex struct {
	Index     SkillIndex
	FetchedAt time.Time
}

// Discovery 负责从远程 URL 拉取 skill 到本地缓存
type Discovery struct {
	cacheDir        string
	marketplaceURLs []string
	cacheTTL        time.Duration
	httpClient      *http.Client
	logger          *zap.Logger

	mu         sync.Mutex
	indexCache map[string]cachedIndex // url -> cached index
}

// NewDiscovery 创建新的远程 skill 发现器（兼容老签名，零 marketplace URL）
func NewDiscovery(cacheDir string, logger *zap.Logger) *Discovery {
	return NewDiscoveryWithMarketplaces(cacheDir, nil, logger)
}

// NewDiscoveryWithMarketplaces 新建带 marketplace 列表的 Discovery
func NewDiscoveryWithMarketplaces(cacheDir string, marketplaceURLs []string, logger *zap.Logger) *Discovery {
	return &Discovery{
		cacheDir:        cacheDir,
		marketplaceURLs: append([]string(nil), marketplaceURLs...),
		cacheTTL:        5 * time.Minute,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:     logger,
		indexCache: make(map[string]cachedIndex),
	}
}

// SetMarketplaceURLs 热更新 marketplace 列表（bootstrap / 运维热重载用）
func (d *Discovery) SetMarketplaceURLs(urls []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.marketplaceURLs = append([]string(nil), urls...)
}

// MarketplaceURLs 返回当前配置的 marketplace URL 列表拷贝
func (d *Discovery) MarketplaceURLs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.marketplaceURLs...)
}

// CacheDir 返回缓存目录路径
func (d *Discovery) CacheDir() string {
	return d.cacheDir
}

// Pull 从远程 URL 拉取 skill 索引并下载所有 skill 文件到本地缓存。
// 返回包含 SKILL.md 的目录列表
func (d *Discovery) Pull(ctx context.Context, url string) ([]string, error) {
	// 获取 index.json
	indexURL := url + "/index.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("创建请求失败: %s", indexURL), err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("获取索引失败: %s", indexURL), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errs.New(errs.CodeSkillLoadFailed, fmt.Sprintf("获取索引返回状态码 %d: %s", resp.StatusCode, indexURL))
	}

	var index SkillIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("解析索引 JSON 失败: %s", indexURL), err)
	}

	var dirs []string
	for _, entry := range index.Skills {
		skillDir := filepath.Join(d.cacheDir, entry.Name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			d.logger.Warn("创建 skill 缓存目录失败，跳过",
				zap.String("skill", entry.Name), zap.Error(err))
			continue
		}

		allOK := true
		for _, file := range entry.Files {
			localPath := filepath.Join(skillDir, file)
			// 缓存命中：文件已存在则跳过下载
			if _, err := os.Stat(localPath); err == nil {
				d.logger.Debug("缓存命中，跳过下载",
					zap.String("skill", entry.Name), zap.String("file", file))
				continue
			}

			fileURL := url + "/" + entry.Name + "/" + file
			if err := d.downloadFile(ctx, fileURL, localPath); err != nil {
				d.logger.Warn("下载 skill 文件失败，跳过该 skill",
					zap.String("skill", entry.Name),
					zap.String("file", file),
					zap.String("url", fileURL),
					zap.Error(err))
				allOK = false
				break
			}
		}

		if !allOK {
			continue
		}

		// 检查目录中是否包含 SKILL.md
		skillMD := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillMD); err == nil {
			dirs = append(dirs, skillDir)
		}
	}

	return dirs, nil
}

// downloadFile 从 URL 下载单个文件到本地路径
func (d *Discovery) downloadFile(ctx context.Context, url, localPath string) error {
	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return errs.Wrap(errs.CodeSkillLoadFailed, "创建目录失败", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("创建请求失败: %s", url), err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("下载失败: %s", url), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errs.New(errs.CodeSkillLoadFailed, fmt.Sprintf("下载返回状态码 %d: %s", resp.StatusCode, url))
	}

	f, err := os.Create(localPath)
	if err != nil {
		return errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("创建文件失败: %s", localPath), err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("写入文件失败: %s", localPath), err)
	}

	return nil
}

// fetchIndex 获取指定 marketplace URL 的 index.json，带 TTL 缓存。
// refresh=true 强制越过缓存。
func (d *Discovery) fetchIndex(ctx context.Context, url string, refresh bool) (SkillIndex, error) {
	if !refresh {
		d.mu.Lock()
		if entry, ok := d.indexCache[url]; ok && time.Since(entry.FetchedAt) < d.cacheTTL {
			d.mu.Unlock()
			return entry.Index, nil
		}
		d.mu.Unlock()
	}

	indexURL := strings.TrimRight(url, "/") + "/index.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return SkillIndex{}, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("创建请求失败: %s", indexURL), err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return SkillIndex{}, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("获取索引失败: %s", indexURL), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SkillIndex{}, errs.New(errs.CodeSkillLoadFailed, fmt.Sprintf("获取索引返回状态码 %d: %s", resp.StatusCode, indexURL))
	}
	var index SkillIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return SkillIndex{}, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("解析索引 JSON 失败: %s", indexURL), err)
	}

	d.mu.Lock()
	d.indexCache[url] = cachedIndex{Index: index, FetchedAt: time.Now()}
	d.mu.Unlock()
	return index, nil
}

// ResolveByName 遍历所有 marketplace URL，按顺序查找指定 name 的 skill。
// 同名在多 marketplace → CodeSkillAmbiguous + 候选列表。
// refresh=true 强制刷新 index 缓存。
func (d *Discovery) ResolveByName(ctx context.Context, name string, refresh bool) (*ResolvedSkill, error) {
	if name == "" {
		return nil, errs.New(errs.CodeSkillInvalidName, "skill name 不可为空")
	}
	urls := d.MarketplaceURLs()
	if len(urls) == 0 {
		return nil, errs.New(errs.CodeSkillNotFound, "无可用 marketplace")
	}

	var matches []ResolvedSkill
	var lastErr error
	for _, url := range urls {
		idx, err := d.fetchIndex(ctx, url, refresh)
		if err != nil {
			lastErr = err
			d.logger.Warn("fetchIndex 失败，跳过此 marketplace",
				zap.String("url", url), zap.Error(err))
			continue
		}
		for _, entry := range idx.Skills {
			if entry.Name == name {
				matches = append(matches, ResolvedSkill{Entry: entry, Source: url})
			}
		}
	}
	if len(matches) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errs.New(errs.CodeSkillNotFound, fmt.Sprintf("marketplace 未找到 skill %q", name))
	}
	if len(matches) > 1 {
		sources := make([]string, 0, len(matches))
		for _, m := range matches {
			sources = append(sources, m.Source)
		}
		return nil, errs.New(errs.CodeSkillAmbiguous,
			fmt.Sprintf("skill %q 在多个 marketplace 命中: %s — 请显式指定 source 参数", name, strings.Join(sources, ", ")))
	}
	return &matches[0], nil
}

// ResolveByRequirements 仅查远程 marketplace（方法分工硬约束：本地查询走 Registry.FindBySpecRequirements）。
// 按 ProvidesRequirements 覆盖度降序返回。
func (d *Discovery) ResolveByRequirements(ctx context.Context, reqs []string) ([]*ResolvedSkill, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	urls := d.MarketplaceURLs()
	if len(urls) == 0 {
		return nil, nil
	}
	need := make(map[string]struct{}, len(reqs))
	for _, r := range reqs {
		need[r] = struct{}{}
	}

	type scored struct {
		r     ResolvedSkill
		score int
	}
	var all []scored
	for _, url := range urls {
		idx, err := d.fetchIndex(ctx, url, false)
		if err != nil {
			d.logger.Warn("fetchIndex 失败，跳过此 marketplace",
				zap.String("url", url), zap.Error(err))
			continue
		}
		for _, entry := range idx.Skills {
			hit := 0
			for _, p := range entry.ProvidesRequirements {
				if _, ok := need[p]; ok {
					hit++
				}
			}
			if hit > 0 {
				all = append(all, scored{r: ResolvedSkill{Entry: entry, Source: url}, score: hit})
			}
		}
	}
	// 覆盖度降序（稳定：保持 marketplace + entry 顺序）
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j-1].score < all[j].score; j-- {
			all[j-1], all[j] = all[j], all[j-1]
		}
	}
	out := make([]*ResolvedSkill, 0, len(all))
	for i := range all {
		rs := all[i].r
		out = append(out, &rs)
	}
	return out, nil
}

// PullOne 按 name 从指定 source (marketplace URL) 拉单包，原子写盘（.tmp → rename）。
// 返回最终落地的 skill 目录路径（包含 SKILL.md）。
func (d *Discovery) PullOne(ctx context.Context, source, name string) (string, error) {
	if source == "" || name == "" {
		return "", errs.New(errs.CodeSkillInvalidName, "source 和 name 均不可为空")
	}
	idx, err := d.fetchIndex(ctx, source, false)
	if err != nil {
		return "", err
	}
	var target *SkillIndexEntry
	for i := range idx.Skills {
		if idx.Skills[i].Name == name {
			target = &idx.Skills[i]
			break
		}
	}
	if target == nil {
		return "", errs.New(errs.CodeSkillNotFound,
			fmt.Sprintf("marketplace %s 未找到 skill %q", source, name))
	}

	finalDir := filepath.Join(d.cacheDir, name)
	tmpDir := finalDir + ".tmp"
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", errs.Wrap(errs.CodeSkillLoadFailed, "创建临时目录失败", err)
	}

	for _, file := range target.Files {
		localPath := filepath.Join(tmpDir, file)
		fileURL := strings.TrimRight(source, "/") + "/" + name + "/" + file
		if err := d.downloadFile(ctx, fileURL, localPath); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", err
		}
	}

	skillMD := filepath.Join(tmpDir, "SKILL.md")
	if _, err := os.Stat(skillMD); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", errs.New(errs.CodeSkillLoadFailed,
			fmt.Sprintf("skill %q 下载完成但缺少 SKILL.md", name))
	}

	// 原子替换：先删旧目录，再 rename tmp → final
	_ = os.RemoveAll(finalDir)
	if err := os.Rename(tmpDir, finalDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", errs.Wrap(errs.CodeSkillLoadFailed, "原子写盘 rename 失败", err)
	}
	return finalDir, nil
}
