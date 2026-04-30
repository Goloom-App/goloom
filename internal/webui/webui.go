package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/**
var embeddedDist embed.FS

func Handler() http.Handler {
	dist, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		requestPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if requestPath == "." || requestPath == "" {
			requestPath = "index.html"
		}

		if stat, err := fs.Stat(dist, requestPath); err == nil && !stat.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "frontend bundle is not available", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
