package api

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/swys/picshare/internal/gallery"
)

func (h *AdminHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	album := r.URL.Query().Get("album")
	if !validAlbumPath(album) {
		http.Error(w, "bad album", 400)
		return
	}
	target := filepath.Join(h.PhotosDir, filepath.FromSlash(album))
	if err := os.MkdirAll(target, 0755); err != nil {
		http.Error(w, "mkdir: "+err.Error(), 500)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "upload error: "+err.Error(), 400)
		return
	}

	uploaded := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("upload next part: %v", err)
			break
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}
		name := filepath.Base(part.FileName())
		if !gallery.IsImage(name) {
			part.Close()
			continue
		}
		dst := filepath.Join(target, name)
		out, err := os.Create(dst)
		if err != nil {
			log.Printf("create %s: %v", dst, err)
			part.Close()
			continue
		}
		n, err := io.Copy(out, part)
		out.Close()
		part.Close()
		if err != nil {
			log.Printf("write %s: %v", dst, err)
			continue
		}
		if n == 0 {
			os.Remove(dst)
			continue
		}
		uploaded++
	}
	if uploaded == 0 {
		http.Error(w, "no valid images uploaded", 400)
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "uploaded": uploaded, "album": album})
}
