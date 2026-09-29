package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// engineFiles serves the browser build of the calculation engine. The deployment image carries it; a local
// `go run` has no such directory and answers 404, and the page then calculates on the server as before.
func (s *Server) engineFiles(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/engine/")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		writeError(w, r, http.StatusNotFound, "Файл движка не найден.")
		return
	}
	path := filepath.Join(s.engineDir(), name)
	f, err := os.Open(path)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "Файл движка не найден.")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, r, http.StatusNotFound, "Файл движка не найден.")
		return
	}
	switch filepath.Ext(name) {
	case ".wasm":
		w.Header().Set("Content-Type", "application/wasm")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	}
	// One build makes one file, and a new build makes a new one, so the browser may keep it.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

func (s *Server) engineDir() string {
	if s.cfg.EngineDir != "" {
		return s.cfg.EngineDir
	}
	return "/engine"
}
