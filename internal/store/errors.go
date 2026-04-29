package store

import "errors"

var (
	ErrNotFound    = errors.New("session not found")
	ErrCorrupted   = errors.New("session data corrupted")
	ErrPermission  = errors.New("permission denied")
	ErrPartialSave = errors.New("partial save failure")

	// ErrSpecChangeConflict：spec-driven Phase 2 SpecChangeStore 的 CAS 冲突信号。
	// 用 sentinel 而不是 struct（skill_store.go:235 那种）——caller 是 HTTP handler /
	// 中间件，errors.Is 判等比 type assertion 更轻。映射 HTTP 409 Conflict。
	ErrSpecChangeConflict = errors.New("spec change revision conflict")
)
