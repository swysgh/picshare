package gallery

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

func TestScanNested(t *testing.T) {
	dir := t.TempDir()
	mk := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, p), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mk("01.主力产品")
	write("01.主力产品/cover.jpg")
	write("01.主力产品/01.jpg")
	mk("01.主力产品/001.手机")
	write("01.主力产品/001.手机/cover.jpg")
	write("01.主力产品/001.手机/01.jpg")
	mk("01.主力产品/002.平板")
	write("01.主力产品/002.平板/01.jpg")
	mk("02.配件")
	write("02.配件/cover.jpg")

	albums, err := Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 {
		t.Fatalf("top level want 2, got %d", len(albums))
	}
	if albums[0].Name != "01.主力产品" {
		t.Errorf("top[0] name = %q", albums[0].Name)
	}
	if albums[0].Path != "01.主力产品" {
		t.Errorf("top[0] path = %q", albums[0].Path)
	}
	if albums[0].PhotoCount != 2 {
		t.Errorf("top[0] photo_count = %d (own photos only)", albums[0].PhotoCount)
	}
	children := albums[0].Children
	if len(children) != 2 {
		t.Fatalf("children want 2, got %d", len(children))
	}
	if children[0].Name != "001.手机" {
		t.Errorf("child[0] name = %q", children[0].Name)
	}
	if children[0].Path != "01.主力产品/001.手机" {
		t.Errorf("child[0] path = %q", children[0].Path)
	}
	if children[0].CoverURL != "/photos/01.主力产品/001.手机/cover.jpg" {
		t.Errorf("child[0] cover = %q", children[0].CoverURL)
	}
	if len(children[0].Children) != 0 {
		t.Errorf("child[0] children want 0, got %d", len(children[0].Children))
	}
}

func TestScanAlbumNested(t *testing.T) {
	dir := t.TempDir()
	album := filepath.Join(dir, "01.主力产品", "001.手机")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	makeTestJPEG(t, filepath.Join(album, "01.jpg"), 100, 100)
	makeTestJPEG(t, filepath.Join(album, "cover.jpg"), 200, 100)
	if err := os.MkdirAll(filepath.Join(dir, "01.主力产品", "002.平板"), 0755); err != nil {
		t.Fatal(err)
	}

	full, err := ScanAlbum(dir, "01.主力产品/001.手机", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if full.Name != "001.手机" {
		t.Errorf("name = %q", full.Name)
	}
	if full.Path != "01.主力产品/001.手机" {
		t.Errorf("path = %q", full.Path)
	}
	if len(full.Photos) != 2 {
		t.Fatalf("want 2 photos, got %d", len(full.Photos))
	}
	if full.Photos[0].URL != "/photos/01.主力产品/001.手机/01.jpg" {
		t.Errorf("photo url = %q", full.Photos[0].URL)
	}
	if len(full.Children) != 0 {
		t.Errorf("want 0 children, got %d", len(full.Children))
	}

	parent, err := ScanAlbum(dir, "01.主力产品", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Photos) != 0 {
		t.Errorf("parent photos = %d, want 0", len(parent.Photos))
	}
	if len(parent.Children) != 2 {
		t.Fatalf("parent children = %d, want 2", len(parent.Children))
	}
	if parent.Children[0].Name != "001.手机" {
		t.Errorf("parent child[0] = %q", parent.Children[0].Name)
	}
}

func TestCleanRelPath(t *testing.T) {
	good := []string{"a", "a/b", "a/b/c", "01.主力/手机"}
	for _, p := range good {
		if _, err := CleanRelPath(p); err != nil {
			t.Errorf("CleanRelPath(%q) unexpected error: %v", p, err)
		}
	}
	bad := []string{"", ".", "..", "a/../b", "../a", "a/..", "/a", "a//b", "a/\x00b"}
	for _, p := range bad {
		if _, err := CleanRelPath(p); err == nil {
			t.Errorf("CleanRelPath(%q) should fail", p)
		}
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

func TestScanWithManualOrder(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"alpha", "beta", "gamma", "delta"} {
		if err := os.MkdirAll(filepath.Join(dir, n), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// No .order yet: natural name sort.
	albums, err := Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{albums[0].Name, albums[1].Name, albums[2].Name, albums[3].Name}
	want := []string{"alpha", "beta", "delta", "gamma"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default sort = %v, want %v", got, want)
		}
	}

	// Write a manual order.
	if err := WriteOrder(dir, []string{"gamma", "alpha", "delta"}); err != nil {
		t.Fatal(err)
	}
	albums, err = Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	got = []string{albums[0].Name, albums[1].Name, albums[2].Name, albums[3].Name}
	// Listed names first in order; unlisted (beta) after, natural sorted.
	want = []string{"gamma", "alpha", "delta", "beta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("manual sort = %v, want %v", got, want)
		}
	}

	// .order referencing a missing dir is ignored.
	if err := WriteOrder(dir, []string{"ghost", "beta"}); err != nil {
		t.Fatal(err)
	}
	albums, err = Scan(dir, "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if albums[0].Name != "beta" {
		t.Errorf("missing-order entry should be skipped; first = %q", albums[0].Name)
	}

	// WriteOrder with empty list removes the file.
	if err := WriteOrder(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, OrderFileName)); !os.IsNotExist(err) {
		t.Errorf("expected .order removed, stat err = %v", err)
	}
}

func TestPhotosSortedByMtime(t *testing.T) {
	dir := t.TempDir()
	album := filepath.Join(dir, "a")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, mtime time.Time) {
		t.Helper()
		p := filepath.Join(album, name)
		makeTestJPEG(t, p, 10, 10)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mk("old.jpg", base)
	mk("new.jpg", base.Add(2*time.Hour))
	mk("mid.jpg", base.Add(1*time.Hour))

	full, err := ScanAlbum(dir, "a", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Photos) != 3 {
		t.Fatalf("want 3 photos, got %d", len(full.Photos))
	}
	// Expect newest first.
	if full.Photos[0].Name != "new.jpg" || full.Photos[1].Name != "mid.jpg" || full.Photos[2].Name != "old.jpg" {
		t.Errorf("photo mtime sort wrong: %v, %v, %v",
			full.Photos[0].Name, full.Photos[1].Name, full.Photos[2].Name)
	}
}

func TestHiddenPhotos(t *testing.T) {
	dir := t.TempDir()
	album := filepath.Join(dir, "a")
	if err := os.MkdirAll(album, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"01.jpg", "02.jpg", "03.jpg"} {
		makeTestJPEG(t, filepath.Join(album, n), 10, 10)
	}
	// Hide 02.jpg.
	if err := WriteHidden(album, []string{"02.jpg"}); err != nil {
		t.Fatal(err)
	}

	// Public scan: hidden excluded.
	pub, err := ScanAlbum(dir, "a", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.Photos) != 2 {
		t.Fatalf("public want 2 photos, got %d", len(pub.Photos))
	}
	for _, p := range pub.Photos {
		if p.Name == "02.jpg" {
			t.Errorf("public should not see hidden photo")
		}
		if p.Hidden {
			t.Errorf("public photo %s unexpectedly marked hidden", p.Name)
		}
	}
	// PhotoCount should exclude hidden too.
	if pub.PhotoCount != 2 {
		t.Errorf("public PhotoCount want 2, got %d", pub.PhotoCount)
	}

	// Admin scan: hidden included with flag.
	adm, err := ScanAlbumAdmin(dir, "a", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(adm.Photos) != 3 {
		t.Fatalf("admin want 3 photos, got %d", len(adm.Photos))
	}
	var sawHidden bool
	for _, p := range adm.Photos {
		if p.Name == "02.jpg" {
			sawHidden = true
			if !p.Hidden {
				t.Errorf("admin should see 02.jpg marked hidden")
			}
		}
	}
	if !sawHidden {
		t.Errorf("admin should see hidden photo 02.jpg")
	}

	// Hidden photo can be the cover: simulate by writing cover.jpg as hidden.
	makeTestJPEG(t, filepath.Join(album, "cover.jpg"), 10, 10)
	if err := WriteHidden(album, []string{"02.jpg", "cover.jpg"}); err != nil {
		t.Fatal(err)
	}
	pub2, err := ScanAlbum(dir, "a", "/photos")
	if err != nil {
		t.Fatal(err)
	}
	if pub2.CoverURL == "" {
		t.Errorf("cover should still resolve even when hidden")
	}
	// cover.jpg should not appear in the public photos list.
	for _, p := range pub2.Photos {
		if p.Name == "cover.jpg" {
			t.Errorf("hidden cover.jpg should not appear in public photos")
		}
	}

	// Clearing hidden removes the file.
	if err := WriteHidden(album, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(album, HiddenFileName)); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", HiddenFileName, err)
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
