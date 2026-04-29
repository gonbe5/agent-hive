package auth

import "context"

// UserIDFrom 从 context 提取用户 ID，未登录时返回空字符串
func UserIDFrom(ctx context.Context) string {
	if u := UserFrom(ctx); u != nil {
		return u.ID
	}
	return ""
}
