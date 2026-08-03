package thumb

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
)

type Handler struct {
	PhotosDir    string
	ThumbsDir    string
	WebPrefix    string
	GridSize     int
	LightboxSize int
	EnableExif   bool
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
	if fi, err := os.Stat(cached); err == nil && fi.Size() > 0 {
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

func (h *Handler) resolveSource(rawPath string) (string, error) {
	decoded, err := url.QueryUnescape(rawPath)
	if err != nil {
		return "", err
	}
	decoded = strings.TrimPrefix(decoded, h.WebPrefix)
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

func (h *Handler) generate(src, cached string, size int) error {
	img, err := h.open(src)
	if err != nil {
		return err
	}
	img = imaging.Fit(img, size, size, imaging.Lanczos)
	if err := os.MkdirAll(filepath.Dir(cached), 0755); err != nil {
		return err
	}
	out, err := os.Create(cached)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, img, &jpeg.Options{Quality: 82})
}

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

func (h *Handler) Regenerate(src string) error {
	for _, size := range []int{h.GridSize, h.LightboxSize} {
		cached := h.cachePath(src, size)
		if err := os.Remove(cached); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := h.generate(src, cached, size); err != nil {
			return err
		}
	}
	return nil
}
