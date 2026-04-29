package bootstrap

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// masterHITLAdapter 把 *master.Master 适配为 mcphost.HITLEmitter，打破
// mcphost → master 的反向依赖。master 自己已 import tools → mcphost，tools
// 不能反向 import master，故 HITL 类型定义在 mcphost 中性镜像层，本适配器
// 在 bootstrap 组装时把镜像翻译为 master 的实现。
//
// 无状态 → goroutine-safe；同实例可被任意数量 Host 共享。
type masterHITLAdapter struct {
	m *master.Master
}

// newMasterHITLAdapter 构造适配器。m 必须非 nil——调用侧应只在 Master 已就绪时
// 才 SetHITLEmitter；nil 情况下让 Host.EmitInputRequest 返回
// ErrHITLEmitterNotConfigured，而不是在 dispatch 时 panic。
func newMasterHITLAdapter(m *master.Master) mcphost.HITLEmitter {
	if m == nil {
		return nil
	}
	return &masterHITLAdapter{m: m}
}

// EmitInputRequest 翻译镜像字段 → master.InputRequest，调用 Master.EmitInputRequest，
// 再把响应翻译回镜像。ctx、err 原样返回，不做语义改写。
func (a *masterHITLAdapter) EmitInputRequest(ctx context.Context, req mcphost.HITLInputRequest) (*mcphost.HITLInputResponse, error) {
	mReq := master.InputRequest{
		ID:          req.ID,
		TaskID:      req.TaskID,
		StepID:      req.StepID,
		Type:        master.InputRequestType(req.Type),
		Prompt:      req.Prompt,
		Options:     req.Options,
		Default:     req.Default,
		Timeout:     req.Timeout,
		ToolName:    req.ToolName,
		ChoiceType:  req.ChoiceType,
		Data:        req.Data,
		SessionID:   req.SessionID,
		Fingerprint: req.Fingerprint,
		CreatedAt:   req.CreatedAt,
	}
	resp, err := a.m.EmitInputRequest(ctx, mReq)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &mcphost.HITLInputResponse{
		RequestID: resp.RequestID,
		TaskID:    resp.TaskID,
		Value:     resp.Value,
		Action:    resp.Action,
		Remember:  resp.Remember,
	}, nil
}
