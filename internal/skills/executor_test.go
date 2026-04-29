package skills

import (
	"fmt"
	"testing"
)

type testExecutor struct {
	outputs map[string]string
}

func (e *testExecutor) Execute(command string) (string, string, error) {
	out, ok := e.outputs[command]
	if !ok {
		return "", "", fmt.Errorf("unknown command: %s", command)
	}
	return out, "", nil
}

func TestExecuteDynamicContext(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		outputs  map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "single command",
			content:  "Version: !`echo hello`",
			outputs:  map[string]string{"echo hello": "hello"},
			expected: "Version: hello",
		},
		{
			name:     "multiple commands",
			content:  "A: !`cmd1`, B: !`cmd2`",
			outputs:  map[string]string{"cmd1": "val1", "cmd2": "val2"},
			expected: "A: val1, B: val2",
		},
		{
			name:     "no commands",
			content:  "Plain text without dynamic context.",
			outputs:  map[string]string{},
			expected: "Plain text without dynamic context.",
		},
		{
			name:     "command fails",
			content:  "Result: !`failing-cmd`",
			outputs:  map[string]string{},
			expected: "Result: !`failing-cmd`",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &testExecutor{outputs: tt.outputs}
			result, err := ExecuteDynamicContext(tt.content, exec)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
