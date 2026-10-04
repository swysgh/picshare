package gallery

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
	".bmp":  true,
	".tif":  true,
	".tiff": true,
}

var coverNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png", "cover.webp",
	"folder.jpg", "folder.jpeg", "folder.png", "folder.webp",
}

// OrderFileName is the per-directory file that records manual child ordering.
// Each line is a child directory name in the desired display order.
const OrderFileName = ".order"

// HiddenFileName is the per-directory file that lists photos hidden from the
// public album view. Each line is a photo file name. Hidden photos are still
// visible in the admin UI and may still be used as the album cover.
const HiddenFileName = ".hidden"

type Photo struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	ThumbURL  string `json:"thumb_url"`
	Size      int64  `json:"size"`
	Modified  int64  `json:"modified"`
	IsCover   bool   `json:"is_cover"`
	Hidden    bool   `json:"hidden,omitempty"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type Album struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Slug        string   `json:"slug"`
	Description string   `json:"description"`
	CoverURL    string   `json:"cover_url"`
	CoverThumb  string   `json:"cover_thumb"`
	PhotoCount  int      `json:"photo_count"`
	Photos      []Photo  `json:"photos,omitempty"`
	Children    []*Album `json:"children,omitempty"`
}

func IsImage(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

func StripPrefix(name string) string {
	return strings.TrimLeft(name, "0")
}

func naturalLess(a, b string) int {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ad, aOk := readDigit(a, &ai)
		bd, bOk := readDigit(b, &bi)
		if aOk && bOk {
			if ad != bd {
				return cmp.Compare(ad, bd)
			}
			if ai == len(a) && bi == len(b) {
				return 0
			}
			if ai == len(a) {
				return -1
			}
			if bi == len(b) {
				return 1
			}
			continue
		}
		if aOk != bOk {
			if aOk {
				return -1
			}
			return 1
		}
		if a[ai] != b[bi] {
			return cmp.Compare(a[ai], b[bi])
		}
		ai++
		bi++
	}
	return cmp.Compare(len(a), len(b))
}

func readDigit(s string, i *int) (int64, bool) {
	if *i >= len(s) || s[*i] < '0' || s[*i] > '9' {
		return 0, false
	}
	var n int64
	for *i < len(s) && s[*i] >= '0' && s[*i] <= '9' {
		n = n*10 + int64(s[*i]-'0')
		*i++
	}
	return n, true
}

// Scan returns the top-level albums. Each album may contain nested Children,
// so subdirectories are browsable as multi-level folder categories.
func Scan(root, webPrefix string) ([]*Album, error) {
	return scanDir(root, "", webPrefix)
}

func scanDir(root, rel, webPrefix string) ([]*Album, error) {
	dir := root
	if rel != "" {
		dir = filepath.Join(root, filepath.FromSlash(rel))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var albums []*Album
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childRel := e.Name()
		if rel != "" {
			childRel = rel + "/" + e.Name()
		}
		a, err := scanNode(root, childRel, webPrefix, false)
		if err != nil {
			continue
		}
		albums = append(albums, a)
	}
	sortAlbumsByOrder(albums, readOrder(dir))
	return albums, nil
}

// readOrder reads the .order file in dir, returning the listed names in order.
// Missing file or read error yields nil.
func readOrder(dir string) []string {
	return readNameList(filepath.Join(dir, OrderFileName))
}

// WriteOrder persists the manual child order for dir. Pass nil or an empty
// slice to remove the order file.
func WriteOrder(dir string, names []string) error {
	return writeNameList(filepath.Join(dir, OrderFileName), names)
}

// ReadHidden returns the set of hidden photo names in dir.
func ReadHidden(dir string) map[string]bool {
	names := readNameList(filepath.Join(dir, HiddenFileName))
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// WriteHidden persists the hidden photo list for dir. Pass nil or an empty
// slice to remove the file.
func WriteHidden(dir string, names []string) error {
	return writeNameList(filepath.Join(dir, HiddenFileName), names)
}

// readNameList reads a newline-separated name list file, returning the
// trimmed, non-empty, non-comment lines. Missing files yield nil.
func readNameList(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// writeNameList writes a newline-separated name list. An empty slice removes
// the file instead of leaving an empty one behind.
func writeNameList(path string, names []string) error {
	if len(names) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	body := strings.Join(names, "\n") + "\n"
	return os.WriteFile(path, []byte(body), 0644)
}

// sortAlbumsByOrder sorts albums so that names appearing in order come first
// in that sequence; the remainder keep natural name order.
func sortAlbumsByOrder(albums []*Album, order []string) {
	rank := make(map[string]int, len(order))
	for i, n := range order {
		if _, dup := rank[n]; !dup {
			rank[n] = i
		}
	}
	slices.SortStableFunc(albums, func(a, b *Album) int {
		ra, oka := rank[a.Name]
		rb, okb := rank[b.Name]
		if oka && okb {
			return cmp.Compare(ra, rb)
		}
		if oka {
			return -1
		}
		if okb {
			return 1
		}
		return naturalLess(a.Name, b.Name)
	})
}

// scanNode builds a single album node. When withPhotos is true the Photos
// list is populated (album detail); otherwise only PhotoCount is filled so
// the list view stays cheap. PhotoCount excludes hidden photos.
func scanNode(root, rel, webPrefix string, withPhotos bool) (*Album, error) {
	dir := filepath.Join(root, filepath.FromSlash(rel))
	cover, err := findCover(dir)
	if err != nil {
		return nil, err
	}
	photoNames, _ := listImages(dir)
	hidden := ReadHidden(dir)
	visibleCount := 0
	for _, n := range photoNames {
		if !hidden[n] {
			visibleCount++
		}
	}
	desc, _ := os.ReadFile(filepath.Join(dir, "description.txt"))
	children, _ := scanDir(root, rel, webPrefix)
	name := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		name = rel[i+1:]
	}
	a := &Album{
		Name:        name,
		Path:        rel,
		Slug:        urlSlug(rel),
		Description: strings.TrimSpace(string(desc)),
		PhotoCount:  visibleCount,
		Photos:      []Photo{},
		Children:    children,
	}
	if withPhotos {
		a.Photos = buildPhotos(root, rel, webPrefix, dir, cover, false)
	}
	if cover != "" {
		a.CoverURL = joinURL(webPrefix, rel, cover)
		a.CoverThumb = "/thumb?p=" + urlPathEscape(joinURL(webPrefix, rel, cover)) + "&w=480"
	}
	return a, nil
}

// ScanAlbum returns a single album (with photos and nested children) located
// at the slash-separated relative path rel from root. Hidden photos are
// excluded from the Photos list.
func ScanAlbum(root, rel, webPrefix string) (*Album, error) {
	return scanAlbum(root, rel, webPrefix, false)
}

// ScanAlbumAdmin is ScanAlbum but includes hidden photos in the Photos list,
// marking each with Photo.Hidden so the admin UI can render the state.
func ScanAlbumAdmin(root, rel, webPrefix string) (*Album, error) {
	return scanAlbum(root, rel, webPrefix, true)
}

func scanAlbum(root, rel, webPrefix string, includeHidden bool) (*Album, error) {
	clean, err := CleanRelPath(rel)
	if err != nil {
		return nil, fmt.Errorf("album not found")
	}
	dir := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("album not found")
	}
	a, err := scanNode(root, clean, webPrefix, false)
	if err != nil {
		return nil, err
	}
	cover, _ := findCover(dir)
	a.Photos = buildPhotos(root, clean, webPrefix, dir, cover, includeHidden)
	return a, nil
}

// CleanRelPath validates a slash-separated relative path and returns its
// cleaned form, rejecting traversal and empty segments.
func CleanRelPath(rel string) (string, error) {
	if rel == "" || strings.Contains(rel, "\x00") {
		return "", fmt.Errorf("bad path")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("bad path")
		}
	}
	return path.Clean(rel), nil
}

func buildPhotos(root, rel, webPrefix, dir, cover string, includeHidden bool) []Photo {
	names, _ := listImages(dir)
	hidden := ReadHidden(dir)
	photos := make([]Photo, 0, len(names))
	for _, n := range names {
		isHidden := hidden[n]
		if isHidden && !includeHidden {
			continue
		}
		finfo, _ := os.Stat(filepath.Join(dir, n))
		var size int64
		var mtime int64
		if finfo != nil {
			size = finfo.Size()
			mtime = finfo.ModTime().Unix()
		}
		w, h := readImageSize(filepath.Join(dir, n))
		photos = append(photos, Photo{
			Name:     n,
			URL:      joinURL(webPrefix, rel, n),
			ThumbURL: "/thumb?p=" + urlPathEscape(joinURL(webPrefix, rel, n)) + "&w=480",
			Size:     size,
			Modified: mtime,
			IsCover:  n == cover,
			Hidden:   isHidden,
			Width:    w,
			Height:   h,
		})
	}
	// Newest upload first; fall back to natural name order on ties.
	slices.SortStableFunc(photos, func(a, b Photo) int {
		if a.Modified != b.Modified {
			return cmp.Compare(b.Modified, a.Modified)
		}
		return naturalLess(a.Name, b.Name)
	})
	return photos
}

func findCover(dir string) (string, error) {
	for _, n := range coverNames {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return n, nil
		}
	}
	names, err := listImages(dir)
	if err != nil {
		return "", err
	}
	if len(names) > 0 {
		return names[0], nil
	}
	return "", nil
}

func listImages(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if n == "description.txt" {
			continue
		}
		if !IsImage(n) {
			continue
		}
		names = append(names, n)
	}
	slices.SortFunc(names, naturalLess)
	return names, nil
}

func joinURL(parts ...string) string {
	out := ""
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		if out == "" {
			out = p
		} else {
			out += "/" + p
		}
	}
	return "/" + out
}

func urlSlug(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "/", "_"), " ", "_")
}

func readImageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func ModTime(path string) (time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}
