package gateway

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/store"
)

// resourceSaveRequest 外部资源保存请求
type resourceSaveRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Environment string `json:"environment"`
	Description string `json:"description"`
	Connection  string `json:"connection"`
	Endpoint    string `json:"endpoint"`
	Credentials string `json:"credentials"`
	ReadOnly    bool   `json:"read_only"`
	Enabled     bool   `json:"enabled"`
}

// registerResourceMethods 注册外部资源管理相关 RPC 方法
func registerResourceMethods(gw *Gateway, deps Deps) {
	// resources.list — 列出所有外部资源
	gw.Register(MethodDef{
		Name:        "resources.list",
		Description: "列出所有外部资源配置",
		AuthScope:   "",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			if deps.Store == nil {
				return nil, errs.New(errs.CodeInternal, "存储后端未初始化")
			}

			records, err := deps.Store.ListExternalResources(context.Background())
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "查询外部资源列表失败", err)
			}

			return json.Marshal(map[string]any{
				"resources": records,
			})
		},
	})

	// resources.get — 获取单个外部资源
	gw.Register(MethodDef{
		Name:        "resources.get",
		Description: "根据名称获取单个外部资源配置",
		AuthScope:   "",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			if deps.Store == nil {
				return nil, errs.New(errs.CodeInternal, "存储后端未初始化")
			}

			var p struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "解析请求参数失败", err)
			}
			if p.Name == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "缺少 name 参数")
			}

			rec, err := deps.Store.GetExternalResource(context.Background(), p.Name)
			if err != nil {
				return nil, errs.Wrap(errs.CodeNotFound, "外部资源未找到: "+p.Name, err)
			}

			return json.Marshal(rec)
		},
	})

	// resources.save — 创建或更新外部资源（UPSERT）
	gw.Register(MethodDef{
		Name:        "resources.save",
		Description: "创建或更新外部资源配置（UPSERT）",
		AuthScope:   "admin",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			if deps.Store == nil {
				return nil, errs.New(errs.CodeInternal, "存储后端未初始化")
			}

			var req resourceSaveRequest
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "解析资源保存请求失败", err)
			}
			if req.Name == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "缺少 name 字段")
			}

			rec := &store.ExternalResourceRecord{
				Name:        req.Name,
				Type:        req.Type,
				Environment: req.Environment,
				Description: req.Description,
				Connection:  req.Connection,
				Endpoint:    req.Endpoint,
				Credentials: req.Credentials,
				ReadOnly:    req.ReadOnly,
				Enabled:     req.Enabled,
			}

			if err := deps.Store.SaveExternalResource(context.Background(), rec); err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "保存外部资源失败", err)
			}

			return json.Marshal(map[string]string{
				"status": "ok",
				"name":   req.Name,
			})
		},
	})

	// resources.delete — 删除外部资源
	gw.Register(MethodDef{
		Name:        "resources.delete",
		Description: "根据名称删除外部资源配置",
		AuthScope:   "admin",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			if deps.Store == nil {
				return nil, errs.New(errs.CodeInternal, "存储后端未初始化")
			}

			var p struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "解析请求参数失败", err)
			}
			if p.Name == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "缺少 name 参数")
			}

			if err := deps.Store.DeleteExternalResource(context.Background(), p.Name); err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "删除外部资源失败: "+p.Name, err)
			}

			return json.Marshal(map[string]string{
				"status": "ok",
				"name":   p.Name,
			})
		},
	})
}
