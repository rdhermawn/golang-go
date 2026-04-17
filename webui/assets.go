package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:frontend/dist
var embeddedFrontend embed.FS

func frontendFS() fs.FS {
	sub, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		panic(err)
	}
	return sub
}

func serveFrontendAssets(fileSystem fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if requestPath == "." || requestPath == "/" {
			requestPath = "index.html"
		}

		if requestPath == "" || strings.HasPrefix(requestPath, "api/") {
			http.NotFound(w, r)
			return
		}

		file, err := fileSystem.Open(requestPath)
		if err != nil {
			indexFile, indexErr := fileSystem.Open("index.html")
			if indexErr != nil {
				http.NotFound(w, r)
				return
			}
			defer indexFile.Close()
			serveFile(w, r, "index.html", indexFile)
			return
		}
		defer file.Close()

		serveFile(w, r, requestPath, file)
	})
}

func serveFile(w http.ResponseWriter, r *http.Request, name string, file fs.File) {
	stat, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	readSeeker, ok := file.(io.ReadSeeker)
	if !ok {
		http.NotFound(w, r)
		return
	}

	http.ServeContent(w, r, name, stat.ModTime(), readSeeker)
}
