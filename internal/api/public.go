package api

import (
	"encoding/json"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/swys/picshare/internal/gallery"
)

type PublicHandlers struct {
	PhotosDir string
	ThumbsDir string
	WebPrefix string
}

func (h *PublicHandlers) ListAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := gallery.Scan(h.PhotosDir, h.WebPrefix)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if albums == nil {
		albums = []*gallery.Album{}
	}
	writeJSON(w, albums)
}

func (h *PublicHandlers) GetAlbum(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	if !validAlbumPath(name) {
		http.Error(w, "bad album path", 400)
		return
	}
	full, err := gallery.ScanAlbum(h.PhotosDir, name, h.WebPrefix)
	if err != nil {
		http.Error(w, "album not found", 404)
		return
	}
	writeJSON(w, full)
}

func (h *PublicHandlers) ServeImage(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	clean := filepath.Clean("/" + p)
	abs := filepath.Join(h.PhotosDir, clean)
	rel, err := filepath.Rel(h.PhotosDir, abs)
	if err != nil || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, string(filepath.Separator)+"..") {
		http.Error(w, "forbidden", 403)
		return
	}
	if !strings.HasPrefix(abs, h.PhotosDir) {
		http.Error(w, "forbidden", 403)
		return
	}
	if !gallery.IsImage(path.Base(abs)) {
		http.Error(w, "forbidden", 403)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, abs)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func validAlbumPath(p string) bool {
	if p == "" || strings.Contains(p, "\x00") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
