package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uploadRequest(t *testing.T, album string, files map[string][]byte, extraFields ...string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/upload?album="+album, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return httptest.NewRecorder(), req
}

// TestUploadMissingAlbum404: uploading into a nonexistent album must 404
// and must NOT create the directory.
func TestUploadMissingAlbum404(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	if err := os.MkdirAll(photos, 0755); err != nil {
		t.Fatal(err)
	}
	h := &AdminHandlers{PhotosDir: photos}
	w, req := uploadRequest(t, "nope", map[string][]byte{"a.jpg": makeTestJPEGBytes(t, 10, 10)})
	h.Upload(w, req)
	if w.Code != 404 {
		t.Fatalf("want 404, got %d (body %s)", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(photos, "nope")); err == nil {
		t.Error("album dir must not be auto-created")
	}
}

// TestUploadAtomicAndDetail: files land via .part + rename, the response
// carries per-file ok/failed detail, and non-images are reported skipped.
func TestUploadAtomicAndDetail(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos", "a")
	if err := os.MkdirAll(photos, 0755); err != nil {
		t.Fatal(err)
	}
	h := &AdminHandlers{PhotosDir: filepath.Join(dir, "photos")}

	w, req := uploadRequest(t, "a", map[string][]byte{
		"ok.jpg":  makeTestJPEGBytes(t, 12, 10),
		"bad.txt": []byte("not an image"),
	})
	h.Upload(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Status   string `json:"status"`
		Uploaded int    `json:"uploaded"`
		Results  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Uploaded != 1 {
		t.Errorf("uploaded = %d, want 1", resp.Uploaded)
	}
	byName := map[string]string{}
	for _, r := range resp.Results {
		byName[r.Name] = r.Status
	}
	if byName["ok.jpg"] != "ok" {
		t.Errorf("ok.jpg status = %q", byName["ok.jpg"])
	}
	if byName["bad.txt"] != "skipped" {
		t.Errorf("bad.txt status = %q", byName["bad.txt"])
	}
	// No .part leftovers.
	entries, _ := os.ReadDir(photos)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(photos, "ok.jpg")); err != nil {
		t.Errorf("ok.jpg missing: %v", err)
	}
}

// TestUploadSizeLimit: with UploadLimitMB set, an oversized body is
// rejected and no partial file is left behind.
func TestUploadSizeLimit(t *testing.T) {
	dir := t.TempDir()
	photosRoot := filepath.Join(dir, "photos")
	photos := filepath.Join(photosRoot, "a")
	if err := os.MkdirAll(photos, 0755); err != nil {
		t.Fatal(err)
	}
	h := &AdminHandlers{PhotosDir: photosRoot, UploadLimitMB: 1}

	// ~2MB of "image" payload in a .jpg-named part.
	big := make([]byte, 2<<20)
	w, req := uploadRequest(t, "a", map[string][]byte{"big.jpg": big})
	h.Upload(w, req)
	if w.Code != 400 {
		t.Fatalf("want 400 for oversized upload, got %d (body %s)", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(photos, "big.jpg")); err == nil {
		t.Error("oversized file must not land in the album")
	}
	if _, err := os.Stat(filepath.Join(photos, "big.jpg.part")); err == nil {
		t.Error(".part temp file left behind")
	}
	// No other partial leftovers.
	entries, _ := os.ReadDir(photos)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

// TestWriteFileAtomicallyCleansUpOnFailure: a reader that errors mid-copy
// must leave no .part file and no dst.
func TestWriteFileAtomicallyCleansUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "x.jpg")
	_, err := writeFileAtomically(dst, errReader{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Error("dst must not exist after failure")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("temp files left behind: %d", len(entries))
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

var _ = http.MethodPost
