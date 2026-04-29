package memory

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// mockMemoryStore 测试用的 MemoryStore 模拟实现
type mockMemoryStore struct {
	searchResult *SearchResult
	searchErr    error
	savedRecords []*MemoryRecord
	saveErr      error
}

func (m *mockMemoryStore) Save(ctx context.Context, record *MemoryRecord) (int64, error) {
	if m.saveErr != nil {
		return 0, m.saveErr
	}
	m.savedRecords = append(m.savedRecords, record)
	return int64(len(m.savedRecords)), nil
}

func (m *mockMemoryStore) Get(_ context.Context, _ int64) (*MemoryRecord, error) {
	return nil, nil
}

func (m *mockMemoryStore) Update(_ context.Context, _ *MemoryRecord) error {
	return nil
}

func (m *mockMemoryStore) Delete(_ context.Context, _ int64) error {
	return nil
}

func (m *mockMemoryStore) Search(_ context.Context, _ SearchOptions) (*SearchResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResult, nil
}

func (m *mockMemoryStore) List(_ context.Context, _ SearchOptions) (*SearchResult, error) {
	return m.searchResult, nil
}

func (m *mockMemoryStore) Stats(_ context.Context) (*MemoryStats, error) {
	return &MemoryStats{}, nil
}

func (m *mockMemoryStore) SetEmbedding(_ EmbeddingProvider, _ VectorStore) {}

func (m *mockMemoryStore) Close() error {
	return nil
}

func TestNewInjector(t *testing.T) {
	logger := zap.NewNop()
	store := &mockMemoryStore{}

	tests := []struct {
		name          string
		maxTokens     int
		topK          int
		wantMaxTokens int
		wantTopK      int
	}{
		{
			name:          "使用自定义参数",
			maxTokens:     1000,
			topK:          5,
			wantMaxTokens: 1000,
			wantTopK:      5,
		},
		{
			name:          "零值使用默认值",
			maxTokens:     0,
			topK:          0,
			wantMaxTokens: 2000,
			wantTopK:      10,
		},
		{
			name:          "负值使用默认值",
			maxTokens:     -1,
			topK:          -1,
			wantMaxTokens: 2000,
			wantTopK:      10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inj := NewInjector(store, tt.maxTokens, tt.topK, logger)
			if inj.maxTokens != tt.wantMaxTokens {
				t.Errorf("maxTokens = %d, want %d", inj.maxTokens, tt.wantMaxTokens)
			}
			if inj.topK != tt.wantTopK {
				t.Errorf("topK = %d, want %d", inj.topK, tt.wantTopK)
			}
		})
	}
}

func TestInjector_InjectContext(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name         string
		userMessage  string
		sessionID    string
		searchResult *SearchResult
		searchErr    error
		maxTokens    int
		topK         int
		wantEmpty    bool
		wantContains []string
		wantErr      bool
	}{
		{
			name:        "空消息返回空字符串",
			userMessage: "",
			sessionID:   "s1",
			wantEmpty:   true,
		},
		{
			name:         "无搜索结果返回空字符串",
			userMessage:  "测试查询",
			sessionID:    "s1",
			searchResult: &SearchResult{Memories: nil, Total: 0},
			maxTokens:    2000,
			topK:         10,
			wantEmpty:    true,
		},
		{
			name:        "搜索结果为 nil 返回空字符串",
			userMessage: "测试查询",
			sessionID:   "s1",
			maxTokens:   2000,
			topK:        10,
			wantEmpty:   true,
		},
		{
			name:        "搜索出错返回错误",
			userMessage: "测试查询",
			sessionID:   "s1",
			searchErr:   context.DeadlineExceeded,
			maxTokens:   2000,
			topK:        10,
			wantErr:     true,
		},
		{
			name:        "正常注入记忆",
			userMessage: "Go 语言开发",
			sessionID:   "s1",
			searchResult: &SearchResult{
				Memories: []MemoryRecord{
					{Type: MemoryTypeUser, Content: "用户偏好 Go 语言"},
					{Type: MemoryTypeProject, Content: "项目采用 Plan-and-Execute 架构"},
				},
				Total: 2,
			},
			maxTokens: 2000,
			topK:      10,
			wantContains: []string{
				"## 相关记忆",
				"[user] 用户偏好 Go 语言",
				"[project] 项目采用 Plan-and-Execute 架构",
			},
		},
		{
			name:        "token 上限截断",
			userMessage: "查询",
			sessionID:   "s1",
			searchResult: &SearchResult{
				Memories: []MemoryRecord{
					{Type: MemoryTypeUser, Content: "短记忆"},
					{Type: MemoryTypeProject, Content: strings.Repeat("很长的记忆内容", 500)},
				},
				Total: 2,
			},
			maxTokens:    50,
			topK:         10,
			wantContains: []string{"[user] 短记忆"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockMemoryStore{
				searchResult: tt.searchResult,
				searchErr:    tt.searchErr,
			}

			maxTokens := tt.maxTokens
			if maxTokens == 0 {
				maxTokens = 2000
			}
			topK := tt.topK
			if topK == 0 {
				topK = 10
			}

			inj := NewInjector(store, maxTokens, topK, logger)
			result, err := inj.InjectContext(context.Background(), tt.userMessage, tt.sessionID, "")

			if tt.wantErr {
				if err == nil {
					t.Error("期望返回错误，但实际无错误")
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望的错误: %v", err)
			}

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("期望空字符串，得到: %q", result)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("结果中未包含 %q\n实际结果: %s", want, result)
				}
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "空字符串", text: "", want: 0},
		{name: "短字符串", text: "hi", want: 1},
		{name: "正常字符串", text: "这是一个测试文本", want: len("这是一个测试文本") / 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got != tt.want {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}
