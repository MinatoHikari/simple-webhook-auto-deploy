package main

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// 前端构建产物嵌入二进制,保持单文件分发。
// 构建方法:cd frontend && npm run build(产物提交进 git,无 Node 环境也能 go build)
//
//go:embed all:frontend/dist
var distFS embed.FS

var distRoot fs.FS

func init() {
	sub, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		panic(err)
	}
	distRoot = sub
}

// serveSPA 处理前端静态资源,未命中的路径回退到 index.html(history 路由)。
func serveSPA(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if rel == "" || rel == "." {
		rel = "index.html"
	}

	f, err := distRoot.Open(rel)
	if err != nil {
		serveIndex(w)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		serveIndex(w)
		return
	}

	ct := mime.TypeByExtension(path.Ext(rel))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if strings.HasPrefix(rel, "assets/") {
		// vite 产物带内容哈希,可以长缓存
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = io.Copy(w, f)
}

func serveIndex(w http.ResponseWriter) {
	f, err := distRoot.Open("index.html")
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, f)
}
