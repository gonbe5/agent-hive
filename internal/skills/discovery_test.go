package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestDiscovery_Pull(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name         string
		setupServer  func() *httptest.Server
		setupCache   func(cacheDir string)
		wantDirs     int
		wantErr      bool
		checkNoFetch bool // 验证缓存命中时不会重新下载
	}{
		{
			name: "成功拉取所有 skill",
			setupServer: func() *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
					idx := SkillIndex{
						Skills: []SkillIndexEntry{
							{
								Name:        "remote-skill",
								Description: "远程 skill",
								Files:       []string{"SKILL.md"},
							},
						},
					}
					json.NewEncoder(w).Encode(idx)
				})
				mux.HandleFunc("/remote-skill/SKILL.md", func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("---\nname: remote-skill\ndescription: 远程 skill\n---\n\n远程内容。\n"))
				})
				return httptest.NewServer(mux)
			},
			wantDirs: 1,
			wantErr:  false,
		},
		{
			name: "无效 JSON 返回错误",
			setupServer: func() *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("not valid json{{{"))
				})
				return httptest.NewServer(mux)
			},
			wantDirs: 0,
			wantErr:  true,
		},
		{
			name: "部分失败：一个 skill 下载失败，其他成功",
			setupServer: func() *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
					idx := SkillIndex{
						Skills: []SkillIndexEntry{
							{
								Name:        "good-skill",
								Description: "正常 skill",
								Files:       []string{"SKILL.md"},
							},
							{
								Name:        "bad-skill",
								Description: "下载失败的 skill",
								Files:       []string{"SKILL.md"},
							},
						},
					}
					json.NewEncoder(w).Encode(idx)
				})
				mux.HandleFunc("/good-skill/SKILL.md", func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("---\nname: good-skill\ndescription: 正常 skill\n---\n\n内容。\n"))
				})
				mux.HandleFunc("/bad-skill/SKILL.md", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				})
				return httptest.NewServer(mux)
			},
			wantDirs: 1,
			wantErr:  false,
		},
		{
			name: "缓存命中：已存在的文件不重新下载",
			setupServer: func() *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
					idx := SkillIndex{
						Skills: []SkillIndexEntry{
							{
								Name:        "cached-skill",
								Description: "已缓存的 skill",
								Files:       []string{"SKILL.md"},
							},
						},
					}
					json.NewEncoder(w).Encode(idx)
				})
				// 不提供文件下载端点 — 如果尝试下载会 404
				return httptest.NewServer(mux)
			},
			setupCache: func(cacheDir string) {
				skillDir := filepath.Join(cacheDir, "cached-skill")
				os.MkdirAll(skillDir, 0o755)
				os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: cached-skill\ndescription: 已缓存\n---\n\n缓存内容。\n"), 0o644)
			},
			wantDirs:     1,
			wantErr:      false,
			checkNoFetch: true,
		},
		{
			name: "空索引返回空结果",
			setupServer: func() *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
					idx := SkillIndex{Skills: []SkillIndexEntry{}}
					json.NewEncoder(w).Encode(idx)
				})
				return httptest.NewServer(mux)
			},
			wantDirs: 0,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			srv := tt.setupServer()
			defer srv.Close()

			if tt.setupCache != nil {
				tt.setupCache(cacheDir)
			}

			d := NewDiscovery(cacheDir, logger)
			dirs, err := d.Pull(context.Background(), srv.URL)

			if tt.wantErr {
				if err == nil {
					t.Fatal("期望返回错误，但得到 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Pull 返回意外错误: %v", err)
			}
			if len(dirs) != tt.wantDirs {
				t.Errorf("期望 %d 个目录，得到 %d", tt.wantDirs, len(dirs))
			}
		})
	}
}
