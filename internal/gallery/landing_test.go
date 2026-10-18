package gallery

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestPhoto(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	// Only the extension matters for these helpers (no decoding happens).
	if err := os.WriteFile(path, []byte("not really a jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestLandingBgRoundTrip: the stored value survives a write/read cycle and
// resolves to a lightbox-sized /thumb URL for the same file.
func TestLandingBgRoundTrip(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	writeTestPhoto(t, filepath.Join(photos, "相册 一", "01.jpg"))

	if got := ReadLandingBg(photos); got != "" {
		t.Fatalf("ReadLandingBg on fresh dir = %q, want empty", got)
	}
	if err := WriteLandingBg(photos, "相册 一/01.jpg"); err != nil {
		t.Fatalf("WriteLandingBg: %v", err)
	}
	if got := ReadLandingBg(photos); got != "相册 一/01.jpg" {
		t.Fatalf("ReadLandingBg = %q", got)
	}

	got, ok := LandingBgThumbURL(photos, "相册 一/01.jpg", Options{WebPrefix: "/photos", LightboxSize: 1600})
	if !ok {
		t.Fatal("LandingBgThumbURL reported the existing photo as unusable")
	}
	if !strings.HasPrefix(got, "/thumb?p=") || !strings.Contains(got, "&w=1600") {
		t.Fatalf("unexpected thumb URL %q", got)
	}
	// The query value is escaped; unescaping must give back the served path.
	p := strings.SplitN(strings.TrimPrefix(got, "/thumb?p="), "&", 2)[0]
	unescaped, err := url.QueryUnescape(p)
	if err != nil {
		t.Fatalf("unescape %q: %v", p, err)
	}
	if unescaped != "/photos/相册 一/01.jpg" {
		t.Fatalf("thumb URL points at %q", unescaped)
	}
}

// TestLandingBgReplacesPreviousChoice: a second pick overwrites the first
// instead of appending a second line.
func TestLandingBgReplacesPreviousChoice(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	writeTestPhoto(t, filepath.Join(photos, "a", "01.jpg"))
	writeTestPhoto(t, filepath.Join(photos, "a", "02.png"))

	if err := WriteLandingBg(photos, "a/01.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := WriteLandingBg(photos, "a/02.png"); err != nil {
		t.Fatal(err)
	}
	if got := ReadLandingBg(photos); got != "a/02.png" {
		t.Fatalf("ReadLandingBg = %q, want a/02.png", got)
	}
	body, err := os.ReadFile(filepath.Join(photos, LandingBgFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.TrimSpace(string(body)), "\n") != 0 {
		t.Fatalf("expected a single line, got %q", body)
	}
}

// TestLandingBgMissingPhotoFallsBack: once the chosen photo is gone, the
// resolver reports it unusable so the caller can use the default cover.
func TestLandingBgMissingPhotoFallsBack(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	writeTestPhoto(t, filepath.Join(photos, "a", "01.jpg"))
	if err := WriteLandingBg(photos, "a/01.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(photos, "a", "01.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, ok := LandingBgThumbURL(photos, ReadLandingBg(photos), Options{WebPrefix: "/photos"}); ok {
		t.Fatal("deleted photo must not resolve")
	}
}

// TestLandingBgRejectsBadPaths: traversal, non-image names and dot-files
// never reach the file.
func TestLandingBgRejectsBadPaths(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	if err := os.MkdirAll(photos, 0755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"../escape.jpg", "a/../../escape.jpg", "a/notes.txt", ".hidden", "a/"} {
		if err := WriteLandingBg(photos, p); err == nil {
			t.Errorf("WriteLandingBg(%q) accepted a bad path", p)
		}
	}
	if _, err := os.Stat(filepath.Join(photos, LandingBgFileName)); err == nil {
		t.Fatal(".landing_bg must not be created for rejected input")
	}
}
