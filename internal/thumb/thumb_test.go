package thumb

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
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
