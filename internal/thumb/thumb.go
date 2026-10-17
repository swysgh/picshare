package thumb

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
)

// maxPixels bounds the decoded source dimensions (W*H). A public endpoint
// must never allocate a decoder bomb's worth of pixels, so anything larger
// is rejected before full decode.
const maxPixels = 40_000_000

type Handler struct {
	PhotosDir    string
	ThumbsDir    string
	WebPrefix    string
	GridSize     int
	LightboxSize int
	EnableExif   bool

	// genSem caps concurrent thumbnail generations (buffered channel,
	// capacity 2) so a burst of cache misses cannot decode many huge
	// images at once. Lazily initialized via semInit.
	genSem  chan struct{}
	semInit sync.Once
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	p := r.URL.Query().Get("p")
	if p == "" {
		http.Error(w, "missing p", 400)
		return
	}
	// r.URL.Query().Get already returns the decoded value; unescaping
	// again would corrupt names containing '%' or '+'.
	src, err := h.resolveSource(p)
	if err != nil {
		http.Error(w, "bad path", 400)
		return
	}
	size := h.GridSize
	if s := r.URL.Query().Get("w"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			size = n
		}
	}
	if size != h.GridSize && size != h.LightboxSize {
		size = h.GridSize
	}

	cached := h.cachePath(src, size)
	if h.cacheFresh(cached, src) {
		w.Header().Set("Cache-Control", "public, max-age=2592000")
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, cached)
		return
	}

	if err := h.generate(src, cached, size); err != nil {
		log.Printf("thumb generate failed: %v", err)
		http.Error(w, "decode failed", 400)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, cached)
}

// cacheFresh reports whether the cached thumbnail exists and is at least as
// new as the source file. If the source was modified after the cache was
// generated (e.g. SetCover replaced cover.jpg), the cache is considered stale
// and must be regenerated.
func (h *Handler) cacheFresh(cached, src string) bool {
	ci, err := os.Stat(cached)
	if err != nil || ci.Size() == 0 {
		return false
	}
	si, err := os.Stat(src)
	if err != nil {
		return false
	}
	return !ci.ModTime().Before(si.ModTime())
}

func (h *Handler) resolveSource(rawPath string) (string, error) {
	decoded := strings.TrimPrefix(rawPath, h.WebPrefix)
	decoded = strings.TrimPrefix(decoded, "/")
	clean := filepath.Clean("/" + decoded)
	abs := filepath.Join(h.PhotosDir, clean)
	rel, err := filepath.Rel(h.PhotosDir, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("path traversal")
	}
	if !strings.HasPrefix(abs, h.PhotosDir) {
		return "", errors.New("path traversal")
	}
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func (h *Handler) cachePath(src string, size int) string {
	rel, err := filepath.Rel(h.PhotosDir, src)
	if err != nil {
		rel = src
	}
	return filepath.Join(h.ThumbsDir, rel+fmt.Sprintf(".%d.jpg", size))
}

// generate decodes src (bounded by maxPixels), resizes it, and writes the
// thumbnail atomically: a temp file is synced and renamed into place, so two
// concurrent misses that race on the same cache entry can never leave a
// half-written permanent cache file behind.
func (h *Handler) generate(src, cached string, size int) error {
	if err := h.checkPixels(src); err != nil {
		return err
	}

	h.acquireSem()
	defer h.releaseSem()

	img, err := h.open(src)
	if err != nil {
		return err
	}
	img = imaging.Fit(img, size, size, imaging.Lanczos)
	if err := os.MkdirAll(filepath.Dir(cached), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cached), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: 82}); err != nil {
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
	if err := os.Rename(tmpName, cached); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// checkPixels pre-checks the source dimensions with image.DecodeConfig and
// rejects images whose pixel count exceeds maxPixels, before any full decode
// allocates memory.
func (h *Handler) checkPixels(src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return fmt.Errorf("image too large: %dx%d exceeds %d pixels", cfg.Width, cfg.Height, maxPixels)
	}
	return nil
}

// sem lazily initializes and returns the generation semaphore (capacity 2).
func (h *Handler) sem() chan struct{} {
	h.semInit.Do(func() {
		h.genSem = make(chan struct{}, 2)
	})
	return h.genSem
}

func (h *Handler) acquireSem() { h.sem() <- struct{}{} }

func (h *Handler) releaseSem() { <-h.sem() }

func (h *Handler) open(src string) (image.Image, error) {
	if h.EnableExif {
		return imaging.Open(src, imaging.AutoOrientation(true))
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
