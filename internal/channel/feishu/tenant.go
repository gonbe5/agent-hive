package feishu

import (
	"errors"
	"net/http"
)

const DefaultTenantKey = "default"

var ErrTenantResolverUnavailable = errors.New("feishu tenant resolver unavailable")

// TenantResolver provides tenant resolution hooks for future multi-tenant routing.
// Phase 0 keeps a single fixed tenant, but all call sites must already route through
// this abstraction instead of hard-coding app_id or ad-hoc defaults.
type TenantResolver interface {
	FromEvent() string
	FromHTTP(r *http.Request) (string, error)
}

type SingleTenantResolver struct {
	TenantKey string
}

func NewSingleTenantResolver(tenantKey string) *SingleTenantResolver {
	if tenantKey == "" {
		tenantKey = DefaultTenantKey
	}
	return &SingleTenantResolver{TenantKey: tenantKey}
}

func (r *SingleTenantResolver) FromEvent() string {
	if r == nil || r.TenantKey == "" {
		return DefaultTenantKey
	}
	return r.TenantKey
}

func (r *SingleTenantResolver) FromHTTP(_ *http.Request) (string, error) {
	if r == nil {
		return "", ErrTenantResolverUnavailable
	}
	return r.FromEvent(), nil
}
