package mcphost

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterResource(t *testing.T) {
	tests := []struct {
		name     string
		defs     []ResourceDefinition
		wantLen  int
	}{
		{
			name:    "注册单个资源",
			defs:    []ResourceDefinition{{URI: "file:///a.txt", Name: "a"}},
			wantLen: 1,
		},
		{
			name: "注册多个资源",
			defs: []ResourceDefinition{
				{URI: "file:///a.txt", Name: "a"},
				{URI: "file:///b.txt", Name: "b"},
			},
			wantLen: 2,
		},
		{
			name: "重复URI覆盖注册",
			defs: []ResourceDefinition{
				{URI: "file:///a.txt", Name: "a-old"},
				{URI: "file:///a.txt", Name: "a-new"},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(testLogger())
			provider := func(_ context.Context, _ string) (*ResourceContent, error) {
				return &ResourceContent{}, nil
			}
			for _, d := range tt.defs {
				h.RegisterResource(d, provider)
			}
			assert.Len(t, h.ListResources(), tt.wantLen)
		})
	}
}

func TestReadResource(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		register    bool
		providerErr error
		wantErr     bool
		errContains string
	}{
		{
			name:     "读取已注册资源",
			uri:      "file:///test.txt",
			register: true,
			wantErr:  false,
		},
		{
			name:        "读取未注册资源",
			uri:         "file:///missing.txt",
			register:    false,
			wantErr:     true,
			errContains: "未找到",
		},
		{
			name:        "资源提供者返回错误",
			uri:         "file:///error.txt",
			register:    true,
			providerErr: errors.New("读取失败"),
			wantErr:     true,
			errContains: "读取资源",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(testLogger())
			if tt.register {
				h.RegisterResource(
					ResourceDefinition{URI: tt.uri, Name: "test"},
					func(_ context.Context, uri string) (*ResourceContent, error) {
						if tt.providerErr != nil {
							return nil, tt.providerErr
						}
						return &ResourceContent{URI: uri, Text: "内容"}, nil
					},
				)
			}

			content, err := h.ReadResource(context.Background(), tt.uri)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, content)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.uri, content.URI)
				assert.Equal(t, "内容", content.Text)
			}
		})
	}
}

func TestListResources(t *testing.T) {
	tests := []struct {
		name    string
		uris    []string
		wantLen int
	}{
		{
			name:    "空列表",
			uris:    nil,
			wantLen: 0,
		},
		{
			name:    "多个资源",
			uris:    []string{"file:///a", "file:///b", "file:///c"},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(testLogger())
			provider := func(_ context.Context, _ string) (*ResourceContent, error) {
				return &ResourceContent{}, nil
			}
			for _, uri := range tt.uris {
				h.RegisterResource(ResourceDefinition{URI: uri, Name: uri}, provider)
			}
			defs := h.ListResources()
			assert.Len(t, defs, tt.wantLen)
		})
	}
}
