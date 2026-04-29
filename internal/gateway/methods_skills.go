package gateway

import (
	"context"
	"encoding/json"
)

func registerSkillMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "skills.list",
		Description: "列出所有 Skill",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			skills := deps.SkillRegistry.List()
			return json.Marshal(skills)
		},
	})
}
