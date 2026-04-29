package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		message string
	}{
		{
			name:    "general internal error",
			code:    CodeInternal,
			message: "internal failure",
		},
		{
			name:    "agent not found error",
			code:    CodeAgentNotFound,
			message: "agent does not exist",
		},
		{
			name:    "zero code",
			code:    0,
			message: "zero",
		},
		{
			name:    "empty message",
			code:    CodeTimeout,
			message: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, tt.message)
			if err.Code != tt.code {
				t.Errorf("Code = %d, want %d", err.Code, tt.code)
			}
			if err.Message != tt.message {
				t.Errorf("Message = %q, want %q", err.Message, tt.message)
			}
			if err.Cause != nil {
				t.Errorf("Cause = %v, want nil", err.Cause)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "without cause",
			err:  &Error{Code: CodeInternal, Message: "something broke"},
			want: "[1001] something broke",
		},
		{
			name: "with cause",
			err:  &Error{Code: CodeMCPConnFailed, Message: "connection lost", Cause: errors.New("dial timeout")},
			want: "[5000] connection lost: dial timeout",
		},
		{
			name: "with nested Error cause",
			err: &Error{
				Code:    CodePlanExecFailed,
				Message: "step failed",
				Cause:   &Error{Code: CodeAgentTimeout, Message: "agent timed out"},
			},
			want: "[4002] step failed: [2002] agent timed out",
		},
		{
			name: "empty message without cause",
			err:  &Error{Code: CodeUnknown, Message: ""},
			want: "[1000] ",
		},
		{
			name: "empty message with cause",
			err:  &Error{Code: CodeUnknown, Message: "", Cause: errors.New("oops")},
			want: "[1000] : oops",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		message string
		cause   error
	}{
		{
			name:    "wrap stdlib error",
			code:    CodeMCPToolExecFailed,
			message: "tool exec failed",
			cause:   errors.New("permission denied"),
		},
		{
			name:    "wrap another Error",
			code:    CodePlanExecFailed,
			message: "plan execution failed",
			cause:   New(CodeAgentPanic, "agent panicked"),
		},
		{
			name:    "wrap nil cause",
			code:    CodeInternal,
			message: "nil cause",
			cause:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Wrap(tt.code, tt.message, tt.cause)
			if err.Code != tt.code {
				t.Errorf("Code = %d, want %d", err.Code, tt.code)
			}
			if err.Message != tt.message {
				t.Errorf("Message = %q, want %q", err.Message, tt.message)
			}
			if err.Cause != tt.cause {
				t.Errorf("Cause = %v, want %v", err.Cause, tt.cause)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want error
	}{
		{
			name: "returns cause when present",
			err:  &Error{Code: CodeInternal, Message: "wrapped", Cause: errors.New("root")},
			want: errors.New("root"),
		},
		{
			name: "returns nil when no cause",
			err:  &Error{Code: CodeInternal, Message: "no cause"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Unwrap()
			if tt.want == nil {
				if got != nil {
					t.Errorf("Unwrap() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("Unwrap() = nil, want %v", tt.want)
				return
			}
			if got.Error() != tt.want.Error() {
				t.Errorf("Unwrap().Error() = %q, want %q", got.Error(), tt.want.Error())
			}
		})
	}
}

func TestUnwrap_WorksWithErrorsIs(t *testing.T) {
	root := errors.New("root cause")
	wrapped := Wrap(CodeInternal, "layer one", root)

	if !errors.Is(wrapped, root) {
		t.Error("errors.Is should find root cause through Unwrap chain")
	}
}

func TestUnwrap_WorksWithErrorsAs(t *testing.T) {
	inner := New(CodeAgentTimeout, "agent timed out")
	outer := fmt.Errorf("higher level: %w", inner)

	var target *Error
	if !errors.As(outer, &target) {
		t.Fatal("errors.As should find *Error through fmt.Errorf wrapping")
	}
	if target.Code != CodeAgentTimeout {
		t.Errorf("Code = %d, want %d", target.Code, CodeAgentTimeout)
	}
}

func TestIsCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{
			name: "direct match",
			err:  New(CodeInternal, "internal"),
			code: CodeInternal,
			want: true,
		},
		{
			name: "direct mismatch",
			err:  New(CodeInternal, "internal"),
			code: CodeTimeout,
			want: false,
		},
		{
			name: "wrapped chain match via fmt.Errorf",
			err:  fmt.Errorf("outer: %w", New(CodeAgentNotFound, "not found")),
			code: CodeAgentNotFound,
			want: true,
		},
		{
			name: "wrapped chain mismatch via fmt.Errorf",
			err:  fmt.Errorf("outer: %w", New(CodeAgentNotFound, "not found")),
			code: CodeSkillNotFound,
			want: false,
		},
		{
			name: "nested Error wrapping via Wrap",
			err:  Wrap(CodePlanExecFailed, "plan failed", New(CodeAgentPanic, "panic")),
			code: CodePlanExecFailed,
			want: true,
		},
		{
			name: "non-Error type returns false",
			err:  errors.New("plain error"),
			code: CodeInternal,
			want: false,
		},
		{
			name: "nil error returns false",
			err:  nil,
			code: CodeInternal,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCode(tt.err, tt.code)
			if got != tt.want {
				t.Errorf("IsCode(%v, %d) = %v, want %v", tt.err, tt.code, got, tt.want)
			}
		})
	}
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "extracts code from Error",
			err:  New(CodeAgentUnavailable, "unavailable"),
			want: CodeAgentUnavailable,
		},
		{
			name: "extracts code from wrapped Error",
			err:  fmt.Errorf("context: %w", New(CodeSkillLoadFailed, "load failed")),
			want: CodeSkillLoadFailed,
		},
		{
			name: "returns CodeUnknown for plain error",
			err:  errors.New("plain"),
			want: CodeUnknown,
		},
		{
			name: "returns CodeUnknown for nil error",
			err:  nil,
			want: CodeUnknown,
		},
		{
			name: "extracts code from Wrap-created error",
			err:  Wrap(CodeMCPToolNotFound, "not found", errors.New("cause")),
			want: CodeMCPToolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCode(tt.err)
			if got != tt.want {
				t.Errorf("GetCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestError_ImplementsErrorInterface(t *testing.T) {
	var _ error = (*Error)(nil)
	var _ error = New(CodeInternal, "test")
}

func TestNewRetryable(t *testing.T) {
	err := NewRetryable(CodeLLMError, "rate limited")
	if !err.Retryable {
		t.Error("expected Retryable=true")
	}
	if err.Code != CodeLLMError {
		t.Errorf("Code = %d, want %d", err.Code, CodeLLMError)
	}
}

func TestWrapRetryable(t *testing.T) {
	cause := errors.New("upstream")
	err := WrapRetryable(CodeLLMError, "llm failed", cause)
	if !err.Retryable {
		t.Error("expected Retryable=true")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"retryable Error", NewRetryable(CodeLLMError, "retry me"), true},
		{"non-retryable Error", New(CodeInvalidInput, "bad input"), false},
		{"plain error", errors.New("plain"), false},
		{"nil", nil, false},
		{"wrapped retryable via fmt.Errorf", fmt.Errorf("wrap: %w", NewRetryable(CodeLLMError, "x")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestGetSeverity(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Severity
	}{
		{"SeverityLow", &Error{Code: CodeInternal, Message: "x", Severity: SeverityLow}, SeverityLow},
		{"SeverityFatal", &Error{Code: CodeInternal, Message: "x", Severity: SeverityFatal}, SeverityFatal},
		{"plain error defaults to SeverityHigh", errors.New("plain"), SeverityHigh},
		{"nil defaults to SeverityHigh", nil, SeverityHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSeverity(tt.err)
			if got != tt.want {
				t.Errorf("GetSeverity(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
