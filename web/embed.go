// Package web 承载编译期嵌入的前端构建产物。
//
// 生产环境不依赖 Node.js：web/dist 在 go build 时被嵌入二进制。
// 注意：go:embed 路径相对于本文件所在目录，因此本文件必须放在 web/ 下。
package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS 返回 dist 子文件系统。
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return distFS
	}
	return sub
}

// ServeSPA 服务单页应用：命中静态资源直出，其余回退到 index.html。
func ServeSPA(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if p == "" || p == "." {
		p = "index.html"
	}

	data, err := fs.ReadFile(FS(), p)
	if err == nil {
		writeAsset(w, p, data)
		return
	}
	// SPA 回退：所有前端路由都返回 index.html
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		http.Error(w, "前端资源未构建，请先执行前端构建或下载 Release 版本", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

func writeAsset(w http.ResponseWriter, name string, data []byte) {
	ctype := mime.TypeByExtension(filepath.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if strings.HasPrefix(ctype, "text/") || strings.Contains(ctype, "javascript") || strings.Contains(ctype, "json") {
		ctype += "; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(data)
}

// IndexHTML 返回首页 HTML（用于测试与自检）。
func IndexHTML() ([]byte, error) { return fs.ReadFile(FS(), "index.html") }
