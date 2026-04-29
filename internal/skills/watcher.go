package skills

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Watcher 轮询 skill 目录，检测 SKILL.md 变更并触发重新注册。
// 使用轮询而非 fsnotify，保持零外部依赖。
type Watcher struct {
	finder   *Finder
	registry *Registry
	interval time.Duration
	logger   *zap.Logger
	modTimes map[string]time.Time // skillPath → SKILL.md 最后修改时间
}

// NewWatcher 创建新的 Watcher。interval 为 0 时默认 5 秒。
func NewWatcher(finder *Finder, registry *Registry, interval time.Duration, logger *zap.Logger) *Watcher {
	if interval == 0 {
		interval = 5 * time.Second
	}
	return &Watcher{
		finder:   finder,
		registry: registry,
		interval: interval,
		logger:   logger,
		modTimes: make(map[string]time.Time),
	}
}

// Start 开始监视，阻塞直到 ctx 取消。应在 goroutine 中调用。
func (w *Watcher) Start(ctx context.Context) {
	// 初始化 modTimes 快照，避免启动时误报变更
	w.snapshot()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// snapshot 记录当前所有已知 skill 目录的 SKILL.md 修改时间
func (w *Watcher) snapshot() {
	skills := w.registry.listSkillPaths()
	for _, path := range skills {
		skillFile := filepath.Join(path, "SKILL.md")
		if info, err := os.Stat(skillFile); err == nil {
			w.modTimes[path] = info.ModTime()
		}
	}
}

// check 扫描所有搜索路径，检测新增、修改、删除的 skill
func (w *Watcher) check() {
	// 1. 发现当前文件系统上的所有 skill
	discovered, err := w.finder.Discover()
	if err != nil {
		w.logger.Warn("watcher: 发现 skill 失败", zap.Error(err))
		return
	}

	seen := make(map[string]bool)

	for _, s := range discovered {
		seen[s.Path] = true
		skillFile := filepath.Join(s.Path, "SKILL.md")
		info, err := os.Stat(skillFile)
		if err != nil {
			continue
		}
		modTime := info.ModTime()
		prev, known := w.modTimes[s.Path]

		if !known {
			// 新增 skill
			if err := w.registry.Register(s); err != nil {
				w.logger.Warn("watcher: 注册新 skill 失败",
					zap.String("name", s.Metadata.Name), zap.Error(err))
			} else {
				w.logger.Info("watcher: 发现新 skill",
					zap.String("name", s.Metadata.Name),
					zap.String("path", s.Path))
			}
			w.modTimes[s.Path] = modTime
		} else if modTime.After(prev) {
			// 已修改 skill — 重新解析并注册
			oldVersion := ""
			if existing, err := w.registry.Get(s.Metadata.Name); err == nil {
				oldVersion = existing.Metadata.Version
			}
			// 重置 sync.Once，强制重新加载内容
			fresh := &Skill{
				Metadata: s.Metadata,
				Path:     s.Path,
			}
			if err := w.registry.Register(fresh); err != nil {
				w.logger.Warn("watcher: 重新注册 skill 失败",
					zap.String("name", s.Metadata.Name), zap.Error(err))
			} else {
				if oldVersion != "" && oldVersion != s.Metadata.Version {
					w.logger.Info("watcher: skill 版本升级",
						zap.String("name", s.Metadata.Name),
						zap.String("old_version", oldVersion),
						zap.String("new_version", s.Metadata.Version))
				} else {
					w.logger.Info("watcher: skill 已更新",
						zap.String("name", s.Metadata.Name))
				}
			}
			w.modTimes[s.Path] = modTime
		}
	}

	// 2. 检测已删除的 skill
	for path, _ := range w.modTimes {
		if !seen[path] {
			skillFile := filepath.Join(path, "SKILL.md")
			if _, err := os.Stat(skillFile); os.IsNotExist(err) {
				// 找到对应的 skill 名称并注销
				name := w.registry.nameByPath(path)
				if name != "" {
					if err := w.registry.Unregister(name); err == nil {
						w.logger.Info("watcher: skill 已删除",
							zap.String("name", name),
							zap.String("path", path))
					}
				}
				delete(w.modTimes, path)
			}
		}
	}
}
