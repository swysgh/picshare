package thumb

import (
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeTestImage(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 128, 255})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
}

func TestThumbGenerate(t *testing.T) {
	dir := t.TempDir()
	photosDir := filepath.Join(dir, "photos")
	thumbsDir := filepath.Join(dir, "thumbs")
	album := filepath.Join(photosDir, "test")
	src := filepath.Join(album, "01.jpg")
	makeTestImage(t, src, 1000, 800)

	h := &Handler{
		PhotosDir:    photosDir,
		ThumbsDir:    thumbsDir,
		WebPrefix:    "/photos",
		GridSize:     480,
		LightboxSize: 1600,
		EnableExif:   true,
	}
	cached := h.cachePath(src, 480)
	if err := h.generate(src, cached, 480); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(cached)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() == 0 {
		t.Errorf("thumb file empty")
	}
}

func TestResolvePathTraversal(t *testing.T) {
	dir := t.TempDir()
	photosDir := filepath.Join(dir, "photos")
	os.MkdirAll(photosDir, 0755)
	h := &Handler{
		PhotosDir: photosDir,
		WebPrefix: "/photos",
	}
	if _, err := h.resolveSource("/photos/../../etc/passwd"); err == nil {
		t.Errorf("expected path traversal to be rejected")
	}
}

// TestThumbServeFilenameWithPercent: a real file named with '%' (and '+')
// must resolve through /thumb?p=… — the query value is already decoded by
// r.URL.Query().Get, so a second unescape used to break these names.
func TestThumbServeFilenameWithPercent(t *testing.T) {
	dir := t.TempDir()
	photosDir := filepath.Join(dir, "photos")
	thumbsDir := filepath.Join(dir, "thumbs")
	album := filepath.Join(photosDir, "a")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	// File names whose raw forms contain percent-encoding metacharacters.
	src := filepath.Join(album, "50%off+new.jpg")
	makeTestImage(t, src, 100, 80)

	h := &Handler{
		PhotosDir:    photosDir,
		ThumbsDir:    thumbsDir,
		WebPrefix:    "/photos",
		GridSize:     480,
		LightboxSize: 1600,
		EnableExif:   false,
	}

	// The scanner escapes the URL for the query component (QueryEscape,
	// which also escapes '+'); the handler must decode it exactly once
	// (via net/url's query parsing).
	escaped := url.QueryEscape("/photos/a/50%off+new.jpg")
	req := httptest.NewRequest(http.MethodGet, "/thumb?p="+escaped+"&w=480", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 for %%-filename, got %d", w.Code)
	}
	// The cache file must exist next to the source name, not an escaped ghost.
	cached := h.cachePath(src, 480)
	if _, err := os.Stat(cached); err != nil {
		t.Errorf("cached thumb missing: %v", err)
	}
}

// TestThumbServeRejectsDoubleEscape: a query value that still contains a
// raw '%' after net/url decoding (e.g. a forged "%2e%2e" that decodes to
// ".." is handled by traversal checks; here we assert a literal bogus
// percent sequence fails cleanly rather than 500ing).
func TestThumbServeRejectsDoubleEscape(t *testing.T) {
	dir := t.TempDir()
	photosDir := filepath.Join(dir, "photos")
	os.MkdirAll(photosDir, 0755)
	h := &Handler{
		PhotosDir: photosDir,
		WebPrefix: "/photos",
		GridSize:  480,
	}
	req := httptest.NewRequest(http.MethodGet, "/thumb?p="+url.QueryEscape("/photos/bad%zz.jpg")+"&w=480", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("want 400 for undecodable name, got %d", w.Code)
	}
}

// TestGenerateRejectsPixelBomb: sources whose declared dimensions exceed
// maxPixels must be rejected before the full decode allocates memory.
func TestGenerateRejectsPixelBomb(t *testing.T) {
	dir := t.TempDir()
	photosDir := filepath.Join(dir, "photos")
	thumbsDir := filepath.Join(dir, "thumbs")
	album := filepath.Join(photosDir, "bomb")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	// Craft a JPEG header declaring 9000x9000 (81M pixels) without
	// encoding the pixel data: truncate right after the headers of a
	// small image whose SOF we rewrite. Simpler: write a real tiny JPEG
	// but override checkPixels via a fake? No — build a real header.
	src := filepath.Join(album, "bomb.jpg")
	makeTestImage(t, src, 10, 10)

	h := &Handler{PhotosDir: photosDir, ThumbsDir: thumbsDir, WebPrefix: "/photos"}

	// Directly test the pixel check with a hand-built DecodeConfig result:
	// patch the source with a giant SOF instead. Use a real oversized PNG
	// header (cheap to construct, no pixel data needed since DecodeConfig
	// only reads the header).
	bigPath := filepath.Join(album, "big.png")
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	// PNG: 8-byte signature + IHDR declaring 8000x6000 (48M pixels).
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], 8000)
	binary.BigEndian.PutUint32(ihdr[4:8], 6000)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type RGB
	body := append([]byte{}, pngSignature...)
	body = append(body, pngChunk("IHDR", ihdr)...)
	body = append(body, pngChunk("IEND", nil)...)
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cached := h.cachePath(bigPath, 480)
	err = h.generate(bigPath, cached, 480)
	if err == nil {
		t.Fatal("expected pixel-bomb rejection, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(cached); statErr == nil {
		t.Error("cached thumb must not be created for a rejected source")
	}
}

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func pngChunk(typ string, data []byte) []byte {
	buf := make([]byte, 8+len(data)+4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(data)))
	copy(buf[4:4+len(typ)], typ)
	copy(buf[8:], data)
	binary.BigEndian.PutUint32(buf[8+len(data):], pngCRC(typ, data))
	return buf
}

func pngCRC(typ string, data []byte) uint32 {
	h := crc32.NewIEEE()
	h.Write([]byte(typ))
	h.Write(data)
	return h.Sum32()
}

func TestCacheFresh(t *testing.T) {
	dir := t.TempDir()
	photosDir := filepath.Join(dir, "photos")
	thumbsDir := filepath.Join(dir, "thumbs")
	src := filepath.Join(photosDir, "cover.jpg")
	makeTestImage(t, src, 100, 100)

	h := &Handler{
		PhotosDir: photosDir,
		ThumbsDir: thumbsDir,
		WebPrefix: "/photos",
	}
	cached := h.cachePath(src, 480)

	// No cache yet: not fresh.
	if h.cacheFresh(cached, src) {
		t.Errorf("expected no cache to be stale")
	}

	// Generate a cache, ensure it is fresh (cached >= src mtime).
	if err := h.generate(src, cached, 480); err != nil {
		t.Fatal(err)
	}
	// Force cache mtime to be after source.
	now := time.Now()
	if err := os.Chtimes(cached, now, now); err != nil {
		t.Fatal(err)
	}
	if !h.cacheFresh(cached, src) {
		t.Errorf("expected fresh cache after generate")
	}

	// Simulate SetCover replacing the source with a newer file.
	future := now.Add(2 * time.Second)
	if err := os.Chtimes(src, future, future); err != nil {
		t.Fatal(err)
	}
	if h.cacheFresh(cached, src) {
		t.Errorf("expected stale cache when source is newer than cache")
	}
}
