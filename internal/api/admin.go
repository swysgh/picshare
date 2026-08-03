package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/swys/picshare/internal/gallery"
	"github.com/swys/picshare/internal/thumb"
)

type AdminHandlers struct {
	PhotosDir    string
	ThumbsDir    string
	WebPrefix    string
	ThumbHandler *thumb.Handler
}

func (h *AdminHandlers) ListManage(w http.ResponseWriter, r *http.Request) {
	albums, err := gallery.Scan(h.PhotosDir, h.WebPrefix)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if albums == nil {
		albums = []gallery.Album{}
	}
	writeJSON(w, map[string]any{
		"albums": albums,
		"path":   ".",
	})
}

func (h *AdminHandlers) ListAlbumManage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validAlbumName(name) {
		http.Error(w, "bad album name", 400)
		return
	}
	full, err := gallery.ScanAlbum(h.PhotosDir, name, h.WebPrefix)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, map[string]any{
		"album":  full,
		"path":   name,
	})
}

func (h *AdminHandlers) Mkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	name := strings.TrimSpace(req.Album)
	if !validAlbumName(name) {
		http.Error(w, "bad album name", 400)
		return
	}
	target := filepath.Join(h.PhotosDir, name)
	if _, err := os.Stat(target); err == nil {
		http.Error(w, "album already exists", 409)
		return
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok", "album": name})
}

func (h *AdminHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Album string   `json:"album"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if !validAlbumName(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	deleted := 0
	for _, p := range req.Paths {
		if err := h.deleteSafe(req.Album, p); err != nil {
			log.Printf("delete %s/%s: %v", req.Album, p, err)
			continue
		}
		deleted++
		h.invalidateThumb(req.Album, p)
	}
	writeJSON(w, map[string]any{"status": "ok", "deleted": deleted})
}

func (h *AdminHandlers) deleteSafe(album, name string) error {
	album = strings.TrimSpace(album)
	name = filepath.Base(name)
	if album == "" || name == "" || name == "." || name == ".." {
		return errors.New("bad name")
	}
	if !gallery.IsImage(name) && name != "description.txt" {
		return errors.New("not an image or description.txt")
	}
	target := filepath.Join(h.PhotosDir, album, name)
	absAlbum, _ := filepath.Abs(filepath.Join(h.PhotosDir, album))
	abs, _ := filepath.Abs(target)
	if !strings.HasPrefix(abs, absAlbum) {
		return errors.New("path traversal")
	}
	return os.Remove(abs)
}

func (h *AdminHandlers) Rename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
		From  string `json:"from"`
		To    string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if !validAlbumName(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	from := filepath.Base(req.From)
	to := filepath.Base(req.To)
	if from == "" || to == "" || !gallery.IsImage(from) || !gallery.IsImage(to) {
		http.Error(w, "bad filename", 400)
		return
	}
	albumDir := filepath.Join(h.PhotosDir, req.Album)
	src := filepath.Join(albumDir, from)
	dst := filepath.Join(albumDir, to)
	if _, err := os.Stat(dst); err == nil {
		http.Error(w, "destination exists", 409)
		return
	}
	if err := os.Rename(src, dst); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.invalidateThumb(req.Album, from)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *AdminHandlers) SetCover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
		Photo string `json:"photo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if !validAlbumName(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	photo := filepath.Base(req.Photo)
	if !gallery.IsImage(photo) {
		http.Error(w, "bad photo", 400)
		return
	}
	albumDir := filepath.Join(h.PhotosDir, req.Album)
	src := filepath.Join(albumDir, photo)
	ext := filepath.Ext(photo)
	cover := filepath.Join(albumDir, "cover"+ext)
	if _, err := os.Stat(src); err != nil {
		http.Error(w, "source not found", 404)
		return
	}
	for _, old := range []string{"cover.jpg", "cover.jpeg", "cover.png", "cover.webp"} {
		_ = os.Remove(filepath.Join(albumDir, old))
	}
	if err := copyFile(src, cover); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok", "cover": "cover" + ext})
}

func (h *AdminHandlers) invalidateThumb(album, name string) {
	rel := path.Join(album, name)
	if h.ThumbsDir == "" {
		return
	}
	for _, size := range []int{480, 1600} {
		_ = os.Remove(filepath.Join(h.ThumbsDir, rel+fmt.Sprintf(".%d.webp", size)))
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
