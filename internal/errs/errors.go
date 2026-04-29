package errs

import (
	"errors"
	"fmt"
)

// Severity 表示错误的严重程度
type Severity int

const (
	SeverityLow    Severity = iota // 可忽略，不影响主流程
	SeverityMedium                 // 需要关注，但可降级处理
	SeverityHigh                   // 严重错误，需要立即处理
	SeverityFatal                  // 致命错误，系统无法继续
)

// Error 是一个带有错误代码和可选包装原因的结构化错误
type Error struct {
	Code      int
	Message   string
	Cause     error
	Retryable bool     // 是否可重试
	Severity  Severity // 错误严重程度
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// New 使用给定的错误代码和消息创建新的 Error
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap 创建一个包装现有错误的新 Error
func Wrap(code int, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// IsCode 检查错误是否具有给定的错误代码
func IsCode(err error, code int) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

// GetCode 从错误中提取错误代码，如果不是 *Error 则返回 CodeUnknown
func GetCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeUnknown
}

// NewRetryable 创建一个可重试的 Error
func NewRetryable(code int, message string) *Error {
	return &Error{Code: code, Message: message, Retryable: true}
}

// WrapRetryable 创建一个可重试的包装 Error
func WrapRetryable(code int, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause, Retryable: true}
}

// IsRetryable 检查错误是否标记为可重试
func IsRetryable(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Retryable
	}
	return false
}

// GetSeverity 从错误中提取严重程度，非 *Error 返回 SeverityHigh
func GetSeverity(err error) Severity {
	var e *Error
	if errors.As(err, &e) {
		return e.Severity
	}
	return SeverityHigh
}
