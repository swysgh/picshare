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

type Photo struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	ThumbURL  string `json:"thumb_url"`
	Size      int64  `json:"size"`
	Modified  int64  `json:"modified"`
	IsCover   bool   `json:"is_cover"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type Album struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	CoverURL    string  `json:"cover_url"`
	CoverThumb  string  `json:"cover_thumb"`
	PhotoCount  int     `json:"photo_count"`
	Photos      []Photo `json:"photos"`
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

func Scan(root, webPrefix string) ([]Album, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var albums []Album
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		cover, err := findCover(dir)
		if err != nil {
			continue
		}
		photoNames, _ := listImages(dir)
		desc, _ := os.ReadFile(filepath.Join(dir, "description.txt"))
		slug := urlSlug(e.Name())
		a := Album{
			Name:        e.Name(),
			Slug:        slug,
			Description: strings.TrimSpace(string(desc)),
			PhotoCount:  len(photoNames),
			Photos:      []Photo{},
		}
		if cover != "" {
			a.CoverURL = joinURL(webPrefix, e.Name(), cover)
			a.CoverThumb = "/thumb?p=" + urlPathEscape(joinURL(webPrefix, e.Name(), cover)) + "&w=480"
		}
		albums = append(albums, a)
	}
	slices.SortFunc(albums, func(a, b Album) int {
		return naturalLess(a.Name, b.Name)
	})
	return albums, nil
}

func ScanAlbum(root, album, webPrefix string) (*Album, error) {
	dir := filepath.Join(root, album)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("album not found")
	}
	cover, _ := findCover(dir)
	names, _ := listImages(dir)
	desc, _ := os.ReadFile(filepath.Join(dir, "description.txt"))
	slug := urlSlug(album)
	a := &Album{
		Name:        album,
		Slug:        slug,
		Description: strings.TrimSpace(string(desc)),
		PhotoCount:  len(names),
		Photos:      []Photo{},
	}
	for _, n := range names {
		finfo, _ := os.Stat(filepath.Join(dir, n))
		var size int64
		var mtime int64
		if finfo != nil {
			size = finfo.Size()
			mtime = finfo.ModTime().Unix()
		}
		w, h := readImageSize(filepath.Join(dir, n))
		a.Photos = append(a.Photos, Photo{
			Name:     n,
			URL:      joinURL(webPrefix, album, n),
			ThumbURL: "/thumb?p=" + urlPathEscape(joinURL(webPrefix, album, n)) + "&w=480",
			Size:     size,
			Modified: mtime,
			IsCover:  n == cover,
			Width:    w,
			Height:   h,
		})
	}
	if cover != "" {
		a.CoverURL = joinURL(webPrefix, album, cover)
		a.CoverThumb = "/thumb?p=" + urlPathEscape(joinURL(webPrefix, album, cover)) + "&w=480"
	}
	return a, nil
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
