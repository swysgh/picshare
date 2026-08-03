package gallery

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"01.jpg", "02.jpg", -1},
		{"02.jpg", "01.jpg", 1},
		{"10.jpg", "2.jpg", 1},
		{"01.jpg", "cover.jpg", -1},
		{"a.jpg", "b.jpg", -1},
		{"主力", "02", 1},
		{"02", "主力", -1},
		{"1", "2", -1},
		{"2", "2", 0},
		{"1", "1a", -1},
		{"1a", "1", 1},
		{"10", "2", 1},
	}
	for _, tc := range tests {
		got := naturalLess(tc.a, tc.b)
		if (got < 0) != (tc.want < 0) || (got > 0) != (tc.want > 0) || (got == 0) != (tc.want == 0) {
			if got != tc.want {
				t.Errorf("naturalLess(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		}
	}
}

func TestNaturalLessSortNoPanic(t *testing.T) {
	inputs := [][]string{
		{"2", "3", "1"},
		{"新品", "02", "主力"},
		{"1.主力", "2.主力", "02.主力"},
		{"a", "1", "02", "ab"},
		{"10", "9", "1", "100"},
	}
	for _, names := range inputs {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("sort panicked: %v", r)
				}
			}()
			sorted := make([]string, len(names))
			copy(sorted, names)
			slices.SortFunc(sorted, naturalLess)
			t.Logf("input=%v sorted=%v", names, sorted)
		})
	}
}

func TestScanAndCover(t *testing.T) {
	dir := t.TempDir()
	album := filepath.Join(dir, "01.主力产品")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"02.jpg", "01.jpg", "03.jpg", "cover.jpg", "description.txt"} {
		if err := os.WriteFile(filepath.Join(album, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(album, "description.txt"), []byte("产品描述"), 0644); err != nil {
		t.Fatal(err)
	}
	albums, err := Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("want 1 album, got %d", len(albums))
	}
	if albums[0].Name != "01.主力产品" {
		t.Errorf("name = %q", albums[0].Name)
	}
	if albums[0].CoverURL != "/photos/01.主力产品/cover.jpg" {
		t.Errorf("cover = %q", albums[0].CoverURL)
	}
	if albums[0].Description != "产品描述" {
		t.Errorf("desc = %q", albums[0].Description)
	}
	full, err := ScanAlbum(dir, "01.主力产品", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Photos) != 4 {
		t.Fatalf("want 4 photos, got %d", len(full.Photos))
	}
	if full.Photos[0].Name != "01.jpg" {
		t.Errorf("first photo = %q", full.Photos[0].Name)
	}
	coverCount := 0
	for _, p := range full.Photos {
		if p.IsCover {
			coverCount++
		}
	}
	if coverCount != 1 {
		t.Errorf("cover count = %d", coverCount)
	}
}

func TestScanEmptyDir(t *testing.T) {
	dir := t.TempDir()
	albums, err := Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 0 {
		t.Errorf("want 0, got %d", len(albums))
	}
}

func TestScanEmptyAlbum(t *testing.T) {
	dir := t.TempDir()
	album := filepath.Join(dir, "新相册")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	albums, err := Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 {
		t.Fatalf("want 1 album (empty), got %d", len(albums))
	}
	if albums[0].PhotoCount != 0 {
		t.Errorf("photo_count = %d", albums[0].PhotoCount)
	}
	if albums[0].CoverURL != "" {
		t.Errorf("cover_url should be empty, got %q", albums[0].CoverURL)
	}
}

func TestIsImage(t *testing.T) {
	cases := map[string]bool{
		"a.jpg": true, "a.JPG": true, "a.png": true, "a.webp": true,
		"a.gif": true, "a.bmp": true, "a.tiff": true,
		"a.txt": false, "a.mp4": false, "description.txt": false,
	}
	for in, want := range cases {
		if IsImage(in) != want {
			t.Errorf("IsImage(%q) = %v, want %v", in, IsImage(in), want)
		}
	}
}

func TestScanAlbumSizes(t *testing.T) {
	dir := t.TempDir()
	album := filepath.Join(dir, "size_test")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestJPEG(t, filepath.Join(album, "horiz.jpg"), 800, 600)
	makeTestJPEG(t, filepath.Join(album, "vert.jpg"), 600, 800)
	makeTestJPEG(t, filepath.Join(album, "square.jpg"), 1024, 1024)

	full, err := ScanAlbum(dir, "size_test", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Photos) != 3 {
		t.Fatalf("want 3, got %d", len(full.Photos))
	}
	for _, p := range full.Photos {
		if p.Width == 0 || p.Height == 0 {
			t.Errorf("%s has zero dimensions: %dx%d", p.Name, p.Width, p.Height)
		}
	}
	want := map[string][2]int{
		"horiz.jpg":  {800, 600},
		"vert.jpg":   {600, 800},
		"square.jpg": {1024, 1024},
	}
	for _, p := range full.Photos {
		if w, ok := want[p.Name]; ok {
			if p.Width != w[0] || p.Height != w[1] {
				t.Errorf("%s: got %dx%d, want %dx%d", p.Name, p.Width, p.Height, w[0], w[1])
			}
		}
	}
}

var defaultColor = color.RGBA{255, 100, 50, 255}

func makeTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, defaultColor)
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
