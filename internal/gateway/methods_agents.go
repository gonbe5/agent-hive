package gateway

import (
	"context"
	"encoding/json"
)

func registerAgentMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "agents.list",
		Description: "列出所有 Agent",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			agents := deps.Master.ListAgents()
			return json.Marshal(agents)
		},
	})
}
