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
		albums = []*gallery.Album{}
	}
	writeJSON(w, map[string]any{
		"albums": albums,
		"path":   ".",
	})
}

func (h *AdminHandlers) ListAlbumManage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	if !validAlbumPath(name) {
		http.Error(w, "bad album path", 400)
		return
	}
	full, err := gallery.ScanAlbum(h.PhotosDir, name, h.WebPrefix)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, map[string]any{
		"album": full,
		"path":  name,
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
	name = strings.Trim(name, "/")
	if !validAlbumPath(name) {
		http.Error(w, "bad album name", 400)
		return
	}
	target := filepath.Join(h.PhotosDir, filepath.FromSlash(name))
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
	album := strings.TrimSpace(req.Album)
	if album != "" && !validAlbumPath(album) {
		http.Error(w, "bad album", 400)
		return
	}
	deleted := 0
	for _, p := range req.Paths {
		if err := h.deleteSafe(album, p); err != nil {
			log.Printf("delete %s/%s: %v", album, p, err)
			continue
		}
		deleted++
		h.invalidateThumb(album, p)
	}
	writeJSON(w, map[string]any{"status": "ok", "deleted": deleted})
}

// deleteSafe removes an item inside album (a slash-separated path, "" = root).
// name may be "." to delete the album directory itself, a file name, or a
// nested subdirectory name.
func (h *AdminHandlers) deleteSafe(album, name string) error {
	album = strings.TrimSpace(album)
	if name == "." {
		if album == "" {
			return errors.New("cannot delete root")
		}
		if !validAlbumPath(album) {
			return errors.New("bad album")
		}
		target := filepath.Join(h.PhotosDir, filepath.FromSlash(album))
		if !withinRoot(h.PhotosDir, target) {
			return errors.New("path traversal")
		}
		return os.RemoveAll(target)
	}
	name = filepath.Base(name)
	if !validFileName(name) {
		return errors.New("bad name")
	}
	if album != "" && !validAlbumPath(album) {
		return errors.New("bad album")
	}
	target := filepath.Join(h.PhotosDir, filepath.FromSlash(album), name)
	if !withinRoot(h.PhotosDir, target) {
		return errors.New("path traversal")
	}
	fi, err := os.Stat(target)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return os.RemoveAll(target)
	}
	if !gallery.IsImage(name) && name != "description.txt" {
		return errors.New("not an image or description.txt")
	}
	return os.Remove(target)
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
	if req.Album != "" && !validAlbumPath(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	from := filepath.Base(req.From)
	to := filepath.Base(req.To)
	if !validFileName(from) || !validFileName(to) {
		http.Error(w, "bad filename", 400)
		return
	}
	base := h.PhotosDir
	if req.Album != "" {
		base = filepath.Join(h.PhotosDir, filepath.FromSlash(req.Album))
	}
	src := filepath.Join(base, from)
	dst := filepath.Join(base, to)
	if !withinRoot(h.PhotosDir, src) || !withinRoot(h.PhotosDir, dst) {
		http.Error(w, "path traversal", 400)
		return
	}
	fi, err := os.Stat(src)
	if err != nil {
		http.Error(w, "source not found", 404)
		return
	}
	if !fi.IsDir() && !gallery.IsImage(from) && from != "description.txt" {
		http.Error(w, "not an image or description.txt", 400)
		return
	}
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
	if !validAlbumPath(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	photo := filepath.Base(req.Photo)
	if !gallery.IsImage(photo) {
		http.Error(w, "bad photo", 400)
		return
	}
	albumDir := filepath.Join(h.PhotosDir, filepath.FromSlash(req.Album))
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
	if h.ThumbsDir == "" {
		return
	}
	rel := path.Join(album, name)
	_ = os.RemoveAll(filepath.Join(h.ThumbsDir, filepath.FromSlash(rel)))
	for _, size := range []int{480, 1600} {
		_ = os.Remove(filepath.Join(h.ThumbsDir, rel+fmt.Sprintf(".%d.jpg", size)))
	}
}

func withinRoot(root, target string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func validFileName(s string) bool {
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\x00") {
		return false
	}
	return true
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
