package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/swys/picshare/internal/gallery"
)

func (h *AdminHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	album := r.URL.Query().Get("album")
	if !validAlbumPath(album) {
		http.Error(w, "bad album", 400)
		return
	}
	target := filepath.Join(h.PhotosDir, filepath.FromSlash(album))
	// Only upload into existing album directories: auto-creating them here
	// silently turns a typo'd album path into a junk directory.
	fi, err := os.Stat(target)
	if err != nil || !fi.IsDir() {
		http.Error(w, "album not found", 404)
		return
	}

	if limit := h.uploadLimitMB(); limit > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, int64(limit)<<20)
	}

	mr, err := r.MultipartReader()
	if err != nil {
		log.Printf("upload multipart: %v", err)
		http.Error(w, "upload error", 400)
		return
	}

	type fileResult struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	var results []fileResult
	uploaded := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Body over the MaxBytesReader cap shows up here as a
			// read error; report it and stop.
			log.Printf("upload next part: %v", err)
			if len(results) == 0 {
				http.Error(w, "upload error", 400)
				return
			}
			break
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}
		name := filepath.Base(part.FileName())
		if !gallery.IsImage(name) {
			part.Close()
			results = append(results, fileResult{Name: name, Status: "skipped", Error: "not an image"})
			continue
		}
		dst := filepath.Join(target, name)
		n, err := writeFileAtomically(dst, part)
		part.Close()
		if err != nil {
			log.Printf("upload %s: %v", dst, err)
			results = append(results, fileResult{Name: name, Status: "failed", Error: uploadErrText(err)})
			continue
		}
		if n == 0 {
			os.Remove(dst)
			results = append(results, fileResult{Name: name, Status: "failed", Error: "empty file"})
			continue
		}
		uploaded++
		results = append(results, fileResult{Name: name, Status: "ok"})
		gallery.InvalidateScan()
	}
	if uploaded == 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(400)
		fmt.Fprintf(w, `{"status":"error","uploaded":0,"results":%s}`, mustMarshalResults(results))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"status":"ok","uploaded":%d,"album":%q,"results":%s}`, uploaded, album, mustMarshalResults(results))
}

// uploadLimitMB reads the live max_upload_mb for this request. A nil reader
// means the deployment did not wire a limit (tests), which is "no limit".
func (h *AdminHandlers) uploadLimitMB() int {
	if h.UploadLimit == nil {
		return 0
	}
	return h.UploadLimit()
}

// uploadErrText maps low-level write errors to stable client-facing text
// without leaking absolute paths.
func uploadErrText(err error) string {
	if err.Error() == "http: request body too large" {
		return "exceeds max_upload_mb"
	}
	return "write failed"
}

func mustMarshalResults(results any) string {
	b, err := json.Marshal(results)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// writeFileAtomically streams r into dst+".part", syncs, closes (checking
// the close error), then renames into place. On any failure the temp file
// is removed, so a half-written file never enters the album.
func writeFileAtomically(dst string, r io.Reader) (int64, error) {
	part := dst + ".part"
	out, err := os.Create(part)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(out, r)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		os.Remove(part)
		return n, copyErr
	}
	if err := os.Rename(part, dst); err != nil {
		os.Remove(part)
		return n, err
	}
	return n, nil
}
