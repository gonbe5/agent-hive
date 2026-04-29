package explore_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/subagent/explore"
)

// TestExploreTypesMarshaling 测试数据类型的序列化和反序列化
func TestExploreTypesMarshaling(t *testing.T) {
	output := explore.ExploreOutput{
		Summary: "测试项目探索",
		Structure: explore.ProjectStructure{
			RootPath: "/test",
			Directories: map[string]string{
				"/cmd/":      "命令行入口",
				"/internal/": "内部包",
			},
			FileTypes: map[string]int{
				".go": 100,
				".md": 5,
			},
		},
		KeyFiles: []explore.KeyFile{
			{
				Path:       "/go.mod",
				Purpose:    "Go 模块定义",
				Importance: "high",
			},
			{
				Path:       "/README.md",
				Purpose:    "项目说明",
				Importance: "medium",
			},
		},
		Patterns: []explore.CodePattern{
			{
				Pattern:     "依赖注入",
				Description: "通过构造函数注入依赖",
				Examples:    []string{"NewServer(db *DB)", "NewClient(cfg Config)"},
			},
		},
		Insights: []string{
			"项目结构清晰",
			"遵循 Go 最佳实践",
			"包含完善的测试",
		},
	}

	// 序列化
	data, err := json.Marshal(output)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var decoded explore.ExploreOutput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// 验证字段
	assert.Equal(t, output.Summary, decoded.Summary)
	assert.Equal(t, output.Structure.RootPath, decoded.Structure.RootPath)
	assert.Equal(t, len(output.Structure.Directories), len(decoded.Structure.Directories))
	assert.Equal(t, len(output.Structure.FileTypes), len(decoded.Structure.FileTypes))
	assert.Equal(t, len(output.KeyFiles), len(decoded.KeyFiles))
	assert.Equal(t, len(output.Patterns), len(decoded.Patterns))
	assert.Equal(t, len(output.Insights), len(decoded.Insights))

	// 验证具体内容
	assert.Equal(t, output.KeyFiles[0].Path, decoded.KeyFiles[0].Path)
	assert.Equal(t, output.Patterns[0].Pattern, decoded.Patterns[0].Pattern)
}

// TestExploreRequestMarshaling 测试请求类型的序列化
func TestExploreRequestMarshaling(t *testing.T) {
	tests := []struct {
		name string
		req  explore.ExploreRequest
	}{
		{
			name: "完整请求",
			req: explore.ExploreRequest{
				Target: "/path/to/project",
				Focus:  "architecture",
				Depth:  "deep",
			},
		},
		{
			name: "最小请求",
			req: explore.ExploreRequest{
				Target: "/project",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			require.NoError(t, err)

			var decoded explore.ExploreRequest
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.req.Target, decoded.Target)
			assert.Equal(t, tt.req.Focus, decoded.Focus)
			assert.Equal(t, tt.req.Depth, decoded.Depth)
		})
	}
}

// TestExploreOutputValidation 测试输出结构的有效性
func TestExploreOutputValidation(t *testing.T) {
	tests := []struct {
		name    string
		output  explore.ExploreOutput
		wantErr bool
	}{
		{
			name: "有效输出",
			output: explore.ExploreOutput{
				Summary: "项目探索完成",
				Structure: explore.ProjectStructure{
					RootPath:    "/project",
					Directories: make(map[string]string),
					FileTypes:   make(map[string]int),
				},
				KeyFiles: []explore.KeyFile{},
				Patterns: []explore.CodePattern{},
				Insights: []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 验证可以正常序列化
			data, err := json.Marshal(tt.output)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, data)

				// 验证必要字段存在
				var decoded map[string]interface{}
				err = json.Unmarshal(data, &decoded)
				require.NoError(t, err)

				assert.Contains(t, decoded, "summary")
				assert.Contains(t, decoded, "structure")
				assert.Contains(t, decoded, "key_files")
				assert.Contains(t, decoded, "patterns")
				assert.Contains(t, decoded, "insights")
			}
		})
	}
}
