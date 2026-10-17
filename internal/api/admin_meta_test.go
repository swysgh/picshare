package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swys/picshare/internal/gallery"
)

func postJSON(t *testing.T, handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// TestDeleteRemovesHiddenEntry: deleting a hidden photo must drop its
// .hidden entry instead of leaving a dangling name behind.
func TestDeleteRemovesHiddenEntry(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01.jpg"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "02.jpg"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}
	if err := gallery.WriteHidden(albumDir, []string{"01.jpg", "02.jpg"}); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos}
	w := postJSON(t, h.Delete, `{"album":"a","paths":["01.jpg"]}`)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	hidden := gallery.ReadHidden(albumDir)
	if hidden["01.jpg"] {
		t.Error(".hidden still contains deleted 01.jpg")
	}
	if !hidden["02.jpg"] {
		t.Error(".hidden lost 02.jpg, which was not deleted")
	}
}

// TestDeletePartialFailureReturns500: when some paths fail, the response
// must be 500 with per-path results and deleted/failed counts, not a plain
// 200 "ok".
func TestDeletePartialFailureReturns500(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01.jpg"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos}
	// 01.jpg exists; ghost.jpg does not.
	w := postJSON(t, h.Delete, `{"album":"a","paths":["01.jpg","ghost.jpg"]}`)
	if w.Code != 500 {
		t.Fatalf("want 500 on partial failure, got %d (body %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"deleted":1`, `"failed":1`, `"partial"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if _, err := os.Stat(filepath.Join(albumDir, "01.jpg")); err == nil {
		t.Error("01.jpg should have been deleted despite the sibling failure")
	}
}

// TestRenameHiddenEntryFollows: renaming a hidden photo rewrites its
// .hidden entry to the new name.
func TestRenameHiddenEntryFollows(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "old.jpg"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}
	if err := gallery.WriteHidden(albumDir, []string{"old.jpg"}); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos}
	w := postJSON(t, h.Rename, `{"album":"a","from":"old.jpg","to":"new.jpg"}`)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	hidden := gallery.ReadHidden(albumDir)
	if hidden["old.jpg"] {
		t.Error(".hidden still references old name")
	}
	if !hidden["new.jpg"] {
		t.Error(".hidden missing new name")
	}
}

// TestRenameRejectsNonImageTarget: renaming a photo to a non-image name
// (a.txt) or a dot-file must be rejected with 400.
func TestRenameRejectsNonImageTarget(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01.jpg"), makeTestJPEGBytes(t, 10, 10), 0644); err != nil {
		t.Fatal(err)
	}

	h := &AdminHandlers{PhotosDir: photos}
	for _, to := range []string{"a.txt", ".hidden2"} {
		w := postJSON(t, h.Rename, `{"album":"a","from":"01.jpg","to":"`+to+`"}`)
		if w.Code != 400 {
			t.Errorf("rename to %q: want 400, got %d", to, w.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(albumDir, "01.jpg")); err != nil {
		t.Error("source must be untouched after rejected renames")
	}
}

// TestRenameDirectoryStillWorks: folder renames keep working (no image
// extension check on directories).
func TestRenameDirectoryStillWorks(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	if err := os.MkdirAll(filepath.Join(photos, "a", "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	h := &AdminHandlers{PhotosDir: photos}
	w := postJSON(t, h.Rename, `{"album":"a","from":"sub","to":"sub2"}`)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(photos, "a", "sub2")); err != nil {
		t.Errorf("directory not renamed: %v", err)
	}
}

// TestSetHiddenRenameSerialize: a burst of concurrent SetHidden calls must
// not lose updates — the mutex serializes the read-modify-write cycle.
func TestSetHiddenRenameSerialize(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	albumDir := filepath.Join(photos, "a")
	if err := os.MkdirAll(albumDir, 0755); err != nil {
		t.Fatal(err)
	}
	var names []string
	for i := 0; i < 8; i++ {
		name := strings.Repeat("x", i) + string(rune('a'+i)) + ".jpg"
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(albumDir, name), makeTestJPEGBytes(t, 5, 5), 0644); err != nil {
			t.Fatal(err)
		}
	}

	h := &AdminHandlers{PhotosDir: photos}
	done := make(chan struct{})
	for _, name := range names {
		go func(n string) {
			defer func() { done <- struct{}{} }()
			body := `{"album":"a","photo":"` + n + `","hidden":true}`
			w := postJSON(t, h.SetHidden, body)
			if w.Code != 200 {
				t.Errorf("sethidden %s: got %d", n, w.Code)
			}
		}(name)
	}
	for range names {
		<-done
	}
	hidden := gallery.ReadHidden(albumDir)
	if len(hidden) != len(names) {
		t.Fatalf("hidden entries = %d, want %d (lost update under concurrency)", len(hidden), len(names))
	}
	for _, n := range names {
		if !hidden[n] {
			t.Errorf("hidden missing %s", n)
		}
	}
}
