package gmapp

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// uiFS 嵌入 Vue 构建产物；无 npm 构建时为占位 index.html。
//
//go:embed all:ui
var uiFS embed.FS

func spaHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "gm ui not embedded", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// /gm API 由精确路由处理；此处兜底避免被 SPA 吞掉。
		if r.URL.Path == "/gm" || strings.HasPrefix(r.URL.Path, "/gm/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		f, err := sub.Open(p)
		if err == nil {
			_ = f.Close()
			files.ServeHTTP(w, r)
			return
		}
		// FileServer 对 /index.html 会 301 到 /；前端路由直接吐出入口页。
		http.ServeFileFS(w, r, sub, "index.html")
	})
}
