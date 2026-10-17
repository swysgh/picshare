package api

import (
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTestJPEGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{200, 80, 40, 255})
		}
	}
	var buf strings.Builder
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}

// makeTestGIF writes a small animated-format-agnostic GIF (single frame).
func makeTestGIF(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{
		color.RGBA{0, 0, 0, 255}, color.RGBA{255, 100, 50, 255},
	})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetColorIndex(x, y, 1)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gif.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

func newSetCoverRequest(t *testing.T, album, photo string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	body := `{"album":` + quoteJSON(album) + `,"photo":` + quoteJSON(photo) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setcover", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), req
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// TestSetCoverSameFileNoop: setting the cover to the current cover file must
// be a 200 no-op and must NOT truncate the file (the old copyFile would have
// copied the file onto itself, zeroing it out).
func TestSetCoverSameFileNoop(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	jpegBytes := makeTestJPEGBytes(t, 40, 30)
	if err := os.WriteFile(filepath.Join(albumDir, "cover.jpg"), jpegBytes, 0644); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos, ThumbsDir: filepath.Join(dir, "thumbs")}
	w, req := newSetCoverRequest(t, "a", "cover.jpg")
	h.SetCover(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(albumDir, "cover.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("cover.jpg was truncated by self-copy")
	}
	if string(got) != string(jpegBytes) {
		t.Fatalf("cover.jpg changed: %d bytes vs original %d", len(got), len(jpegBytes))
	}
}

// TestSetCoverGifReencodedAsJPG: a GIF source must produce a cover.jpg (JPEG
// bytes), never cover.gif.
func TestSetCoverGifReencodedAsJPG(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestGIF(t, filepath.Join(albumDir, "01.gif"), 40, 30)

	h := &AdminHandlers{PhotosDir: photos, ThumbsDir: filepath.Join(dir, "thumbs")}
	w, req := newSetCoverRequest(t, "a", "01.gif")
	h.SetCover(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(albumDir, "cover.gif")); err == nil {
		t.Error("cover.gif must not be created")
	}
	f, err := os.Open(filepath.Join(albumDir, "cover.jpg"))
	if err != nil {
		t.Fatalf("cover.jpg not created: %v", err)
	}
	defer f.Close()
	if _, _, err := image.DecodeConfig(f); err != nil {
		t.Errorf("cover.jpg is not decodable: %v", err)
	}
}

// TestSetCoverReplacesOtherCovers: setting a new cover removes every other
// legacy cover name (both cover.* and folder.*).
func TestSetCoverReplacesOtherCovers(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Seed with a folder.png cover (higher-priority names absent).
	if err := os.WriteFile(filepath.Join(albumDir, "folder.png"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}
	// Note: folder.png content is actually JPEG bytes; only the name matters
	// for this test (removal check).
	if err := os.WriteFile(filepath.Join(albumDir, "01.jpg"), makeTestJPEGBytes(t, 40, 30), 0644); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos, ThumbsDir: filepath.Join(dir, "thumbs")}
	w, req := newSetCoverRequest(t, "a", "01.jpg")
	h.SetCover(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(albumDir, "folder.png")); err == nil {
		t.Error("folder.png should have been removed")
	}
	if _, err := os.Stat(filepath.Join(albumDir, "folder.jpg")); err == nil {
		t.Error("folder.jpg should have been removed")
	}
}
