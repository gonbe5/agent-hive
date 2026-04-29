package feishu

import "context"

type CommandACL interface {
	CanExecute(ctx context.Context, tenantKey, chatID, openID string, isDirect bool, command string) (bool, error)
}

type GroupAdminChecker interface {
	IsGroupAdmin(ctx context.Context, tenantKey, chatID, openID string) (bool, error)
}

type StaticAllowlistACL struct {
	allowed map[string]map[string]struct{}
}

func NewStaticAllowlistACL(allowed map[string][]string) *StaticAllowlistACL {
	m := make(map[string]map[string]struct{}, len(allowed))
	for tenant, users := range allowed {
		set := make(map[string]struct{}, len(users))
		for _, user := range users {
			set[user] = struct{}{}
		}
		m[tenant] = set
	}
	return &StaticAllowlistACL{allowed: m}
}

func (a *StaticAllowlistACL) CanExecute(_ context.Context, tenantKey, _ string, openID string, isDirect bool, _ string) (bool, error) {
	if tenantKey == "" {
		return false, ErrTenantKeyRequired
	}
	if isDirect {
		return true, nil
	}
	if a == nil {
		return false, nil
	}
	set := a.allowed[tenantKey]
	_, ok := set[openID]
	return ok, nil
}

type GroupAdminACL struct {
	checker     GroupAdminChecker
	superAdmins map[string]struct{}
}

func NewGroupAdminACL(checker GroupAdminChecker, superAdmins []string) *GroupAdminACL {
	set := make(map[string]struct{}, len(superAdmins))
	for _, id := range superAdmins {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	return &GroupAdminACL{checker: checker, superAdmins: set}
}

func (a *GroupAdminACL) CanExecute(ctx context.Context, tenantKey, chatID, openID string, isDirect bool, _ string) (bool, error) {
	if tenantKey == "" {
		return false, ErrTenantKeyRequired
	}
	if isDirect {
		return true, nil
	}
	if a == nil {
		return false, nil
	}
	if _, ok := a.superAdmins[openID]; ok {
		return true, nil
	}
	if a.checker == nil {
		return false, nil
	}
	return a.checker.IsGroupAdmin(ctx, tenantKey, chatID, openID)
}
