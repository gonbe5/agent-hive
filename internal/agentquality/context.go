package agentquality

import "context"

type contextKey struct{}

type ContextValue struct {
	CaseID        string
	PromptVersion string
	FailureType   FailureType
	FinalStatus   FinalStatus
}

func WithContextValue(ctx context.Context, v ContextValue) context.Context {
	return context.WithValue(ctx, contextKey{}, v)
}

func FromContext(ctx context.Context) ContextValue {
	v, _ := ctx.Value(contextKey{}).(ContextValue)
	return v
}
