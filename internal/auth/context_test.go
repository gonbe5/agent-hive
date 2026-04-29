package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithUserAndUserFrom(t *testing.T) {
	u := &User{ID: "user-1", Role: "user", Status: "active"}
	ctx := WithUser(context.Background(), u)
	got := UserFrom(ctx)
	assert.Equal(t, u, got)
}

func TestUserFrom_Nil(t *testing.T) {
	got := UserFrom(context.Background())
	assert.Nil(t, got)
}

func TestUserIDFrom(t *testing.T) {
	u := &User{ID: "user-1", Role: "user", Status: "active"}
	ctx := WithUser(context.Background(), u)
	assert.Equal(t, "user-1", UserIDFrom(ctx))
}

func TestUserIDFrom_NoUser(t *testing.T) {
	assert.Equal(t, "", UserIDFrom(context.Background()))
}

func TestWithAuthEnabled(t *testing.T) {
	ctx := WithAuthEnabled(context.Background())
	assert.True(t, IsAuthEnabled(ctx))
}

func TestIsAuthEnabled_Default(t *testing.T) {
	assert.False(t, IsAuthEnabled(context.Background()))
}

func TestAuthEnabledAndUser(t *testing.T) {
	u := &User{ID: "admin-1", Role: "admin", Status: "active"}
	ctx := WithUser(WithAuthEnabled(context.Background()), u)
	assert.True(t, IsAuthEnabled(ctx))
	assert.Equal(t, u, UserFrom(ctx))
	assert.Equal(t, "admin-1", UserIDFrom(ctx))
}
