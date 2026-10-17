package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/swys/picshare/internal/gallery"
)

type AdminHandlers struct {
	PhotosDir     string
	ThumbsDir     string
	WebPrefix     string
	UploadLimitMB int
	// mu serializes read-modify-write cycles on the per-album metadata
	// files (.hidden/.order) across admin mutations.
	mu sync.Mutex
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
	full, err := gallery.ScanAlbumAdmin(h.PhotosDir, name, h.WebPrefix)
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
	h.mu.Lock()
	defer h.mu.Unlock()
	type delResult struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	var results []delResult
	deleted, failed := 0, 0
	for _, p := range req.Paths {
		if err := h.deleteSafeLocked(album, p); err != nil {
			log.Printf("delete %s/%s: %v", album, p, err)
			results = append(results, delResult{Path: p, Status: "failed", Error: "delete failed"})
			failed++
			continue
		}
		h.removeHiddenEntry(album, p)
		deleted++
		results = append(results, delResult{Path: p, Status: "ok"})
		h.invalidateThumb(album, p)
	}
	body := map[string]any{"status": "ok", "deleted": deleted, "results": results}
	if failed > 0 {
		body["status"] = "partial"
		body["failed"] = failed
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(body)
		return
	}
	writeJSON(w, body)
}

// removeHiddenEntry drops name from the album's .hidden list (if present),
// so deleting a photo does not leave a dangling hidden entry behind.
func (h *AdminHandlers) removeHiddenEntry(album, name string) {
	if name == "." {
		return
	}
	albumDir := filepath.Join(h.PhotosDir, filepath.FromSlash(album))
	hidden := gallery.ReadHidden(albumDir)
	if hidden == nil {
		return
	}
	base := filepath.Base(name)
	if !hidden[base] {
		return
	}
	delete(hidden, base)
	names := make([]string, 0, len(hidden))
	for n := range hidden {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := gallery.WriteHidden(albumDir, names); err != nil {
		log.Printf("remove hidden entry %s/%s: %v", album, base, err)
	}
}

// deleteSafe removes an item inside album (a slash-separated path, "" = root).
// name may be "." to delete the album directory itself, a file name, or a
// nested subdirectory name. Callers must hold h.mu.
func (h *AdminHandlers) deleteSafeLocked(album, name string) error {
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
	// The destination of a photo rename must stay a servable image with a
	// normal name: renaming to "a.txt" (or any dot-file) would produce a
	// file the admin UI can neither delete nor rename back.
	if strings.HasPrefix(to, ".") {
		http.Error(w, "bad target name", 400)
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
	if !fi.IsDir() {
		if !gallery.IsImage(from) || !gallery.IsImage(to) {
			http.Error(w, "not an image", 400)
			return
		}
		if from == "description.txt" {
			http.Error(w, "not an image", 400)
			return
		}
	} else if strings.HasPrefix(to, ".") {
		http.Error(w, "bad target name", 400)
		return
	}
	if _, err := os.Stat(dst); err == nil {
		http.Error(w, "destination exists", 409)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := os.Rename(src, dst); err != nil {
		log.Printf("rename %s -> %s: %v", src, dst, err)
		http.Error(w, "rename failed", 500)
		return
	}
	h.renameHiddenEntry(req.Album, base, from, to)
	h.invalidateThumb(req.Album, from)
	writeJSON(w, map[string]string{"status": "ok"})
}

// renameHiddenEntry rewrites a .hidden entry from the old file name to the
// new one, so a hidden photo stays hidden across a rename. Callers must
// hold h.mu.
func (h *AdminHandlers) renameHiddenEntry(album, albumDir, from, to string) {
	hidden := gallery.ReadHidden(albumDir)
	if hidden == nil || !hidden[from] {
		return
	}
	delete(hidden, from)
	hidden[to] = true
	names := make([]string, 0, len(hidden))
	for n := range hidden {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := gallery.WriteHidden(albumDir, names); err != nil {
		log.Printf("rename hidden entry %s: %s -> %s: %v", album, from, to, err)
	}
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
	// GIF/BMP/TIFF never become cover.<same ext>: the cover must be a
	// browser-friendly JPEG, so those are re-encoded into cover.jpg.
	ext := strings.ToLower(filepath.Ext(photo))
	if ext == ".gif" || ext == ".bmp" || ext == ".tif" || ext == ".tiff" {
		ext = ".jpg"
	}
	cover := filepath.Join(albumDir, "cover"+ext)
	if _, err := os.Stat(src); err != nil {
		http.Error(w, "source not found", 404)
		return
	}
	// Setting the cover to itself is a no-op; copying a file onto itself
	// would truncate it to zero bytes.
	if filepath.Clean(src) == filepath.Clean(cover) {
		writeJSON(w, map[string]string{"status": "ok", "cover": "cover" + ext})
		return
	}
	for _, old := range gallery.CoverNames {
		_ = os.Remove(filepath.Join(albumDir, old))
		h.invalidateThumb(req.Album, old)
	}
	if err := copyFile(src, cover, ext); err != nil {
		log.Printf("setcover %s/%s: %v", req.Album, photo, err)
		http.Error(w, "set cover failed", 500)
		return
	}
	// Ensure subsequent thumbnail requests for the new cover miss the cache
	// even when the copy preserves an mtime older than a previously cached
	// thumbnail. The thumb handler also re-checks source mtime, but doing it
	// here makes the intent explicit and avoids stale hits from proxies.
	h.invalidateThumb(req.Album, "cover"+ext)
	writeJSON(w, map[string]string{"status": "ok", "cover": "cover" + ext})
}

// Reorder persists a manual child-directory order for an album ("" = root).
// The names slice must list existing subdirectory names of that album.
func (h *AdminHandlers) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string   `json:"album"`
		Names []string `json:"names"`
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
	dir := h.PhotosDir
	if album != "" {
		dir = filepath.Join(h.PhotosDir, filepath.FromSlash(album))
	}
	if !withinRoot(h.PhotosDir, dir) {
		http.Error(w, "path traversal", 400)
		return
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		http.Error(w, "album not found", 404)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	clean := make([]string, 0, len(req.Names))
	seen := make(map[string]bool, len(req.Names))
	for _, n := range req.Names {
		n = filepath.Base(strings.TrimSpace(n))
		if !validFileName(n) {
			http.Error(w, "bad name", 400)
			return
		}
		if seen[n] {
			continue
		}
		sub := filepath.Join(dir, n)
		sfi, err := os.Stat(sub)
		if err != nil || !sfi.IsDir() {
			http.Error(w, "unknown child: "+n, 400)
			return
		}
		seen[n] = true
		clean = append(clean, n)
	}
	if err := gallery.WriteOrder(dir, clean); err != nil {
		log.Printf("reorder %s: %v", album, err)
		http.Error(w, "save order failed", 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// SetHidden marks a photo as hidden (or clears the mark). Hidden photos are
// excluded from the public album view but remain visible in the admin UI
// and may still be used as the album cover.
func (h *AdminHandlers) SetHidden(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album  string `json:"album"`
		Photo  string `json:"photo"`
		Hidden bool   `json:"hidden"`
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
	if fi, err := os.Stat(src); err != nil || fi.IsDir() {
		http.Error(w, "source not found", 404)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	hidden := gallery.ReadHidden(albumDir)
	if req.Hidden {
		hidden[photo] = true
	} else {
		delete(hidden, photo)
	}
	names := make([]string, 0, len(hidden))
	for n := range hidden {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := gallery.WriteHidden(albumDir, names); err != nil {
		log.Printf("sethidden %s/%s: %v", req.Album, photo, err)
		http.Error(w, "save hidden state failed", 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
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

// copyFile copies src to dst atomically: it writes a temp file next to dst,
// syncs it, and renames it into place, so a crash or error never leaves a
// truncated half-written dst behind. When ext indicates a format that must
// not be stored as the cover (.gif/.bmp/.tif/.tiff), the image is decoded
// and re-encoded as JPEG instead of a byte copy.
func copyFile(src, dst, ext string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".cover-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := writeCoverFile(tmp, in, ext); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dst)
}

// writeCoverFile writes the cover bytes to w: a plain stream copy for
// JPEG/PNG/WebP, or a JPEG re-encode for formats the cover name must not
// carry (GIF/BMP/TIFF).
func writeCoverFile(w io.Writer, r io.Reader, ext string) error {
	switch ext {
	case ".gif", ".bmp", ".tif", ".tiff":
		img, err := imaging.Decode(r)
		if err != nil {
			return fmt.Errorf("decode %s: %w", ext, err)
		}
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 90})
	default:
		_, err := io.Copy(w, r)
		return err
	}
}
