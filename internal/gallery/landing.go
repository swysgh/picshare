package gallery

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// LandingBgFileName records which photo the admin picked as the landing page
// background. One line: the photo path relative to photos_dir, slash
// separated (e.g. "主力产品/01.jpg"). It lives in the photos tree instead of
// config.json because the config file holds credentials and an HTTP handler
// must never rewrite it.
const LandingBgFileName = ".landing_bg"

// ReadLandingBg returns the stored relative photo path, or "" when nothing
// has been picked (or the file cannot be read).
func ReadLandingBg(photosDir string) string {
	names := readNameList(filepath.Join(photosDir, LandingBgFileName))
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// WriteLandingBg persists photoPath (relative to photosDir) as the landing
// background. The write is atomic (temp file + rename), like .order/.hidden,
// so a crash mid-write cannot leave a truncated file behind.
func WriteLandingBg(photosDir, photoPath string) error {
	clean, err := CleanRelPath(photoPath)
	if err != nil {
		return fmt.Errorf("bad photo path: %w", err)
	}
	if !IsImage(clean) {
		return fmt.Errorf("not an image: %s", clean)
	}
	// The file is a one-line list, so a newline in the name would split the
	// entry in two and silently corrupt the next read.
	if strings.ContainsAny(clean, "\r\n") {
		return fmt.Errorf("bad photo name: %q", clean)
	}
	return writeNameList(filepath.Join(photosDir, LandingBgFileName), []string{clean})
}

// LandingBgThumbURL turns a stored .landing_bg path into the lightbox-sized
// thumbnail URL used as the page background. ok is false when the photo was
// deleted, moved, or is no longer an image, so the caller can fall back to
// the default cover instead of serving a broken background.
func LandingBgThumbURL(photosDir, photoPath string, opts Options) (string, bool) {
	clean, err := CleanRelPath(photoPath)
	if err != nil || !IsImage(clean) {
		return "", false
	}
	fi, err := os.Stat(filepath.Join(photosDir, filepath.FromSlash(clean)))
	if err != nil || fi.IsDir() {
		return "", false
	}
	dir := path.Dir(clean)
	if dir == "." {
		dir = ""
	}
	imgURL := joinURL(opts.WebPrefix, dir, path.Base(clean))
	return "/thumb?p=" + urlPathEscape(imgURL) +
		"&w=" + strconv.Itoa(lightboxSize(opts)) + cacheBust(mtimeNano(fi)), true
}
