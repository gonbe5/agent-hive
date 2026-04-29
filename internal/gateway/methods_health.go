package gateway

import (
	"context"
	"encoding/json"
	"runtime"
	"time"
)

func registerHealthMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "health.check",
		Description: "健康检查",
		AuthScope:   "",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(map[string]string{"status": "ok"})
		},
	})

	gw.Register(MethodDef{
		Name:        "health.status",
		Description: "系统状态",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			return json.Marshal(map[string]any{
				"status":       "ok",
				"time":         time.Now().Format(time.RFC3339),
				"goroutines":   runtime.NumGoroutine(),
				"alloc_mb":     memStats.Alloc / 1024 / 1024,
				"active_model": deps.Master.ActiveModel(),
			})
		},
	})
}
