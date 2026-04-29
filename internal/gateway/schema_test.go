package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name     string
		params   string
		required []string
		wantErr  bool
	}{
		{"无必需字段", `{}`, nil, false},
		{"字段存在", `{"name":"test"}`, []string{"name"}, false},
		{"缺少字段", `{}`, []string{"name"}, true},
		{"无效JSON", `invalid`, []string{"name"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParams(json.RawMessage(tt.params), tt.required)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
