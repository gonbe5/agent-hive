package webui

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// Handler 返回服务前端静态资源的 HTTP handler
// 对于未匹配到文件的请求，fallback 到 index.html（SPA 路由）
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatal("webui: 无法读取嵌入的 dist 目录: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API 请求不处理
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		urlPath := r.URL.Path
		if urlPath == "/" {
			urlPath = "/index.html"
		}

		// 检查文件是否存在
		cleanPath := strings.TrimPrefix(urlPath, "/")
		if _, err := fs.Stat(sub, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// 静态资源（有扩展名）找不到时返回 404，不能 fallback 到 index.html
		// 否则浏览器加载 .js/.css 时收到 text/html 会触发 MIME 严格检查错误
		if path.Ext(urlPath) != "" {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: 无扩展名的路径（前端路由）返回 index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
