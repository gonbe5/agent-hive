package mcphost

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// RegisterBuiltinResources 注册内置系统资源到 MCP Host
func RegisterBuiltinResources(host *Host, logger *zap.Logger) {
	// 1. system://info - 系统信息资源
	host.RegisterResource(
		ResourceDefinition{
			URI:         "system://info",
			Name:        "系统信息",
			Description: "返回 Go 版本、操作系统、架构等系统信息",
			MimeType:    "application/json",
		},
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			info := map[string]string{
				"go_version": runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
				"num_cpu":    fmt.Sprintf("%d", runtime.NumCPU()),
			}
			data, err := json.Marshal(info)
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "序列化系统信息失败", err)
			}
			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	)

	// 2. system://tools - 工具列表资源
	host.RegisterResource(
		ResourceDefinition{
			URI:         "system://tools",
			Name:        "工具列表",
			Description: "返回当前所有注册的工具名称列表（JSON 格式）",
			MimeType:    "application/json",
		},
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			tools := host.ListTools()
			names := make([]string, 0, len(tools))
			for _, t := range tools {
				names = append(names, t.Name)
			}
			data, err := json.Marshal(map[string]any{
				"count": len(names),
				"tools": names,
			})
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "序列化工具列表失败", err)
			}
			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	)

	logger.Info("已注册内置系统资源",
		zap.Strings("资源", []string{"system://info", "system://tools"}),
	)
}
