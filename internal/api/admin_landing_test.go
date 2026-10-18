package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swys/picshare/internal/gallery"
)

// TestSetLandingBgStoresChoice: the handler persists the album-relative photo
// path to photos/.landing_bg and leaves config files alone.
func TestSetLandingBgStoresChoice(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a", "sub")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01.jpg"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos}
	w := postJSON(t, h.SetLandingBg, `{"album":"a/sub","photo":"01.jpg"}`)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	if got := gallery.ReadLandingBg(photos); got != "a/sub/01.jpg" {
		t.Fatalf(".landing_bg = %q, want a/sub/01.jpg", got)
	}
	// The stored path must resolve back to a served thumbnail.
	if _, ok := gallery.LandingBgThumbURL(photos, gallery.ReadLandingBg(photos), gallery.Options{WebPrefix: "/photos"}); !ok {
		t.Fatal("stored background does not resolve to a thumb URL")
	}
}

// TestSetLandingBgRejectsBadInput: traversal, non-images, missing photos and
// an empty album are rejected without touching .landing_bg.
func TestSetLandingBgRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos}
	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty album", `{"album":"","photo":"01.jpg"}`, 400},
		{"traversal album", `{"album":"../outside","photo":"01.jpg"}`, 400},
		{"non-image", `{"album":"a","photo":"notes.txt"}`, 400},
		{"missing photo", `{"album":"a","photo":"ghost.jpg"}`, 404},
	}
	for _, c := range cases {
		w := postJSON(t, h.SetLandingBg, c.body)
		if w.Code != c.want {
			t.Errorf("%s: want %d, got %d (body %s)", c.name, c.want, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(photos, gallery.LandingBgFileName)); err == nil {
		t.Fatal(".landing_bg must not be written for rejected input")
	}
}

// TestSetLandingBgRequiresImageExtension: a dot-file or a name without an
// image extension never becomes the stored background.
func TestSetLandingBgRequiresImageExtension(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, ".landing_bg"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	h := &AdminHandlers{PhotosDir: photos}
	w := postJSON(t, h.SetLandingBg, `{"album":"a","photo":".landing_bg"}`)
	if w.Code != 400 {
		t.Fatalf("want 400, got %d (body %s)", w.Code, w.Body.String())
	}
	if body := strings.TrimSpace(w.Body.String()); body == "" {
		t.Fatal("expected an error body")
	}
}
