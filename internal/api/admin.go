package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/disintegration/imaging"
	"github.com/swys/picshare/internal/config"
	"github.com/swys/picshare/internal/gallery"
)

type AdminHandlers struct {
	PhotosDir    string
	ThumbsDir    string
	WebPrefix    string
	GridSize     int
	LightboxSize int
	// UploadLimit reads the live max_upload_mb on every upload, so a value
	// saved in the settings page applies without a restart. A nil func means
	// "no limit" (older wiring / tests).
	UploadLimit func() int
	// Settings is the hot-reloadable config behind GET/POST
	// /api/admin/settings; nil disables those two endpoints.
	Settings SettingsStore
	// mu serializes read-modify-write cycles on the per-album metadata
	// files (.hidden/.order) across admin mutations.
	mu sync.Mutex
}

// SettingsStore is the live configuration the settings page reads and writes.
// main wires it to liveCfg, which persists the change and only then swaps the
// in-memory config.
type SettingsStore interface {
	Settings() config.Settings
	ApplySettings(config.Settings) error
}

// maxSettingsTextLen mirrors config.MaxTextLen so the API can reject an
// oversized value before it reaches the config layer.
const maxSettingsTextLen = config.MaxTextLen

// GetSettings returns the admin-editable settings. The payload deliberately
// has no admin_pass / admin_user / listen / directory fields (see
// config.Settings).
func (h *AdminHandlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		settingsError(w, http.StatusServiceUnavailable, "设置接口未启用")
		return
	}
	writeJSON(w, h.Settings.Settings())
}

// UpdateSettings validates and applies a settings form post. The whole
// allowlisted field set must be present: a partial body would silently reset
// the missing fields to their zero value (e.g. max_upload_mb=0 = unlimited).
func (h *AdminHandlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		settingsError(w, http.StatusServiceUnavailable, "设置接口未启用")
		return
	}
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&raw); err != nil {
		settingsError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}
	if msg := checkSettingsKeys(raw); msg != "" {
		settingsError(w, http.StatusBadRequest, msg)
		return
	}
	var s config.Settings
	body, err := json.Marshal(raw)
	if err != nil {
		settingsError(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return
	}
	if err := json.Unmarshal(body, &s); err != nil {
		settingsError(w, http.StatusBadRequest, "字段类型不正确（数字字段请传数字）")
		return
	}
	s.SiteTitle = strings.TrimSpace(s.SiteTitle)
	s.LandingRemember = strings.TrimSpace(strings.ToLower(s.LandingRemember))
	if n := utf8.RuneCountInString(s.SiteTitle); n > maxSettingsTextLen {
		settingsError(w, http.StatusBadRequest, fmt.Sprintf("站点标题最长 %d 个字符（当前 %d）", maxSettingsTextLen, n))
		return
	}
	if msg := checkLandingI18n(raw["landing_i18n"]); msg != "" {
		settingsError(w, http.StatusBadRequest, msg)
		return
	}
	s.LandingI18n = config.LandingI18n{
		Title:    trimTexts(s.LandingI18n.Title),
		Subtitle: trimTexts(s.LandingI18n.Subtitle),
		CTA:      trimTexts(s.LandingI18n.CTA),
	}
	if msg := checkTexts("大标题", s.LandingI18n.Title); msg != "" {
		settingsError(w, http.StatusBadRequest, msg)
		return
	}
	if msg := checkTexts("副标题", s.LandingI18n.Subtitle); msg != "" {
		settingsError(w, http.StatusBadRequest, msg)
		return
	}
	if msg := checkTexts("进入提示文字", s.LandingI18n.CTA); msg != "" {
		settingsError(w, http.StatusBadRequest, msg)
		return
	}
	if s.LandingBackdropOpacity < 0 || s.LandingBackdropOpacity > 100 {
		settingsError(w, http.StatusBadRequest, fmt.Sprintf("遮罩透明度必须在 0-100 之间（当前 %d）", s.LandingBackdropOpacity))
		return
	}
	if _, err := config.ParseRemember(s.LandingRemember); err != nil {
		settingsError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.MaxUploadMB < 0 {
		settingsError(w, http.StatusBadRequest, fmt.Sprintf("单文件上传上限不能为负数（当前 %d；0 = 不限制）", s.MaxUploadMB))
		return
	}
	if err := h.Settings.ApplySettings(s); err != nil {
		log.Printf("update settings: %v", err)
		settingsError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Echo the saved values: the response is what the form re-renders from,
	// and it proves the write round-tripped through disk.
	writeJSON(w, h.Settings.Settings())
}

// settingsError writes a JSON error body so the admin UI can show the server's
// Chinese reason verbatim; http.Error's text/plain body would be swallowed by
// the client's JSON error parser and degrade to "HTTP 400".
func settingsError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"error": msg})
}

// checkSettingsKeys enforces the allowlist in both directions: no unknown
// key may be sent (it could smuggle a field the UI must never touch), and no
// allowlisted key may be omitted (see UpdateSettings).
func checkSettingsKeys(raw map[string]json.RawMessage) string {
	for k := range raw {
		if !config.IsSettingsKey(k) {
			return "不支持的字段: " + k
		}
	}
	for _, k := range config.SettingsKeys {
		if _, ok := raw[k]; !ok {
			return "缺少字段: " + k
		}
	}
	return ""
}

// checkLandingI18n rejects unknown group/language keys inside landing_i18n.
// The struct decode would silently drop them, which would look like a
// successful save that changed nothing.
func checkLandingI18n(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var groups map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return "landing_i18n 必须是 {\"title\":{\"zh\":…}} 形式的对象"
	}
	for name, langs := range groups {
		switch name {
		case "title", "subtitle", "cta":
		default:
			return "landing_i18n 不支持的字段: " + name
		}
		for lang := range langs {
			switch lang {
			case "zh", "ko", "en":
			default:
				return "landing_i18n." + name + " 不支持的语言: " + lang
			}
		}
	}
	return ""
}

func trimTexts(t config.LandingTexts) config.LandingTexts {
	trim := strings.TrimSpace
	return config.LandingTexts{Zh: trim(t.Zh), Ko: trim(t.Ko), En: trim(t.En)}
}

func checkTexts(label string, t config.LandingTexts) string {
	for _, lang := range config.Langs {
		if n := utf8.RuneCountInString(t.For(lang)); n > maxSettingsTextLen {
			return fmt.Sprintf("%s（%s）最长 %d 个字符（当前 %d）", label, lang, maxSettingsTextLen, n)
		}
	}
	return ""
}

func (h *AdminHandlers) ListManage(w http.ResponseWriter, r *http.Request) {
	albums, err := gallery.ScanOpts(h.PhotosDir, h.scanOpts())
	if err != nil {
		log.Printf("admin list: %v", err)
		http.Error(w, "internal error", 500)
		return
	}
	if albums == nil {
		albums = []*gallery.Album{}
	}
	writeJSON(w, map[string]any{
		"albums": albums,
		"path":   ".",
	})
}

func (h *AdminHandlers) ListAlbumManage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	if !validAlbumPath(name) {
		http.Error(w, "bad album path", 400)
		return
	}
	full, err := gallery.ScanAlbumAdminOpts(h.PhotosDir, name, h.scanOpts())
	if err != nil {
		http.Error(w, "album not found", 404)
		return
	}
	writeJSON(w, map[string]any{
		"album": full,
		"path":  name,
	})
}

func (h *AdminHandlers) Mkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	name := strings.TrimSpace(req.Album)
	name = strings.Trim(name, "/")
	if !validAlbumPath(name) {
		http.Error(w, "bad album name", 400)
		return
	}
	target := filepath.Join(h.PhotosDir, filepath.FromSlash(name))
	if _, err := os.Stat(target); err == nil {
		http.Error(w, "album already exists", 409)
		return
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	gallery.InvalidateScan()
	writeJSON(w, map[string]string{"status": "ok", "album": name})
}

func (h *AdminHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Album string   `json:"album"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	album := strings.TrimSpace(req.Album)
	if album != "" && !validAlbumPath(album) {
		http.Error(w, "bad album", 400)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	type delResult struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	var results []delResult
	deleted, failed := 0, 0
	for _, p := range req.Paths {
		if err := h.deleteSafeLocked(album, p); err != nil {
			log.Printf("delete %s/%s: %v", album, p, err)
			results = append(results, delResult{Path: p, Status: "failed", Error: "delete failed"})
			failed++
			continue
		}
		h.removeHiddenEntry(album, p)
		deleted++
		results = append(results, delResult{Path: p, Status: "ok"})
		h.invalidateThumb(album, p)
		gallery.InvalidateScan()
	}
	body := map[string]any{"status": "ok", "deleted": deleted, "results": results}
	if failed > 0 {
		body["status"] = "partial"
		body["failed"] = failed
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(500)
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(body)
		return
	}
	writeJSON(w, body)
}

// removeHiddenEntry drops name from the album's .hidden list (if present),
// so deleting a photo does not leave a dangling hidden entry behind.
func (h *AdminHandlers) removeHiddenEntry(album, name string) {
	if name == "." {
		return
	}
	albumDir := filepath.Join(h.PhotosDir, filepath.FromSlash(album))
	hidden := gallery.ReadHidden(albumDir)
	if hidden == nil {
		return
	}
	base := filepath.Base(name)
	if !hidden[base] {
		return
	}
	delete(hidden, base)
	names := make([]string, 0, len(hidden))
	for n := range hidden {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := gallery.WriteHidden(albumDir, names); err != nil {
		log.Printf("remove hidden entry %s/%s: %v", album, base, err)
	}
}

// deleteSafe removes an item inside album (a slash-separated path, "" = root).
// name may be "." to delete the album directory itself, a file name, or a
// nested subdirectory name. Callers must hold h.mu.
func (h *AdminHandlers) deleteSafeLocked(album, name string) error {
	album = strings.TrimSpace(album)
	if name == "." {
		if album == "" {
			return errors.New("cannot delete root")
		}
		if !validAlbumPath(album) {
			return errors.New("bad album")
		}
		target := filepath.Join(h.PhotosDir, filepath.FromSlash(album))
		if !withinRoot(h.PhotosDir, target) {
			return errors.New("path traversal")
		}
		return os.RemoveAll(target)
	}
	name = filepath.Base(name)
	if !validFileName(name) {
		return errors.New("bad name")
	}
	if album != "" && !validAlbumPath(album) {
		return errors.New("bad album")
	}
	target := filepath.Join(h.PhotosDir, filepath.FromSlash(album), name)
	if !withinRoot(h.PhotosDir, target) {
		return errors.New("path traversal")
	}
	fi, err := os.Stat(target)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return os.RemoveAll(target)
	}
	if !gallery.IsImage(name) && name != "description.txt" {
		return errors.New("not an image or description.txt")
	}
	return os.Remove(target)
}

func (h *AdminHandlers) Rename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
		From  string `json:"from"`
		To    string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.Album != "" && !validAlbumPath(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	from := filepath.Base(req.From)
	to := filepath.Base(req.To)
	if !validFileName(from) || !validFileName(to) {
		http.Error(w, "bad filename", 400)
		return
	}
	// The destination of a photo rename must stay a servable image with a
	// normal name: renaming to "a.txt" (or any dot-file) would produce a
	// file the admin UI can neither delete nor rename back.
	if strings.HasPrefix(to, ".") {
		http.Error(w, "bad target name", 400)
		return
	}
	base := h.PhotosDir
	if req.Album != "" {
		base = filepath.Join(h.PhotosDir, filepath.FromSlash(req.Album))
	}
	src := filepath.Join(base, from)
	dst := filepath.Join(base, to)
	if !withinRoot(h.PhotosDir, src) || !withinRoot(h.PhotosDir, dst) {
		http.Error(w, "path traversal", 400)
		return
	}
	fi, err := os.Stat(src)
	if err != nil {
		http.Error(w, "source not found", 404)
		return
	}
	if !fi.IsDir() {
		if !gallery.IsImage(from) || !gallery.IsImage(to) {
			http.Error(w, "not an image", 400)
			return
		}
		if from == "description.txt" {
			http.Error(w, "not an image", 400)
			return
		}
	} else if strings.HasPrefix(to, ".") {
		http.Error(w, "bad target name", 400)
		return
	}
	if _, err := os.Stat(dst); err == nil {
		http.Error(w, "destination exists", 409)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := os.Rename(src, dst); err != nil {
		log.Printf("rename %s -> %s: %v", src, dst, err)
		http.Error(w, "rename failed", 500)
		return
	}
	h.renameHiddenEntry(req.Album, base, from, to)
	h.invalidateThumb(req.Album, from)
	gallery.InvalidateScan()
	writeJSON(w, map[string]string{"status": "ok"})
}

// renameHiddenEntry rewrites a .hidden entry from the old file name to the
// new one, so a hidden photo stays hidden across a rename. Callers must
// hold h.mu.
func (h *AdminHandlers) renameHiddenEntry(album, albumDir, from, to string) {
	hidden := gallery.ReadHidden(albumDir)
	if hidden == nil || !hidden[from] {
		return
	}
	delete(hidden, from)
	hidden[to] = true
	names := make([]string, 0, len(hidden))
	for n := range hidden {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := gallery.WriteHidden(albumDir, names); err != nil {
		log.Printf("rename hidden entry %s: %s -> %s: %v", album, from, to, err)
	}
}

func (h *AdminHandlers) SetCover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
		Photo string `json:"photo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if !validAlbumPath(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	photo := filepath.Base(req.Photo)
	if !gallery.IsImage(photo) {
		http.Error(w, "bad photo", 400)
		return
	}
	albumDir := filepath.Join(h.PhotosDir, filepath.FromSlash(req.Album))
	src := filepath.Join(albumDir, photo)
	// GIF/BMP/TIFF never become cover.<same ext>: the cover must be a
	// browser-friendly JPEG, so those are re-encoded into cover.jpg.
	ext := strings.ToLower(filepath.Ext(photo))
	if ext == ".gif" || ext == ".bmp" || ext == ".tif" || ext == ".tiff" {
		ext = ".jpg"
	}
	cover := filepath.Join(albumDir, "cover"+ext)
	if _, err := os.Stat(src); err != nil {
		http.Error(w, "source not found", 404)
		return
	}
	// Setting the cover to itself is a no-op; copying a file onto itself
	// would truncate it to zero bytes.
	if filepath.Clean(src) == filepath.Clean(cover) {
		gallery.InvalidateScan()
		writeJSON(w, map[string]string{"status": "ok", "cover": "cover" + ext})
		return
	}
	for _, old := range gallery.CoverNames {
		_ = os.Remove(filepath.Join(albumDir, old))
		h.invalidateThumb(req.Album, old)
	}
	if err := copyFile(src, cover, ext); err != nil {
		log.Printf("setcover %s/%s: %v", req.Album, photo, err)
		http.Error(w, "set cover failed", 500)
		return
	}
	// Ensure subsequent thumbnail requests for the new cover miss the cache
	// even when the copy preserves an mtime older than a previously cached
	// thumbnail. The thumb handler also re-checks source mtime, but doing it
	// here makes the intent explicit and avoids stale hits from proxies.
	h.invalidateThumb(req.Album, "cover"+ext)
	gallery.InvalidateScan()
	writeJSON(w, map[string]string{"status": "ok", "cover": "cover" + ext})
}

// SetLandingBg pins one photo as the landing page background. The choice is
// stored in photos/.landing_bg (not config.json: that file holds the admin
// credentials and an HTTP handler must not rewrite it) and re-read on every
// landing render, so it applies without a restart.
func (h *AdminHandlers) SetLandingBg(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string `json:"album"`
		Photo string `json:"photo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if !validAlbumPath(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	photo := filepath.Base(req.Photo)
	if !gallery.IsImage(photo) {
		http.Error(w, "bad photo", 400)
		return
	}
	rel := req.Album + "/" + photo
	src := filepath.Join(h.PhotosDir, filepath.FromSlash(rel))
	if !withinRoot(h.PhotosDir, src) {
		http.Error(w, "path traversal", 400)
		return
	}
	if fi, err := os.Stat(src); err != nil || fi.IsDir() {
		http.Error(w, "source not found", 404)
		return
	}
	if err := gallery.WriteLandingBg(h.PhotosDir, rel); err != nil {
		log.Printf("setlandingbg %s: %v", rel, err)
		http.Error(w, "save landing background failed", 500)
		return
	}
	gallery.InvalidateScan()
	writeJSON(w, map[string]string{"status": "ok", "photo": rel})
}

// Reorder persists a manual child-directory order for an album ("" = root).
// The names slice must list existing subdirectory names of that album.
func (h *AdminHandlers) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album string   `json:"album"`
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	album := strings.TrimSpace(req.Album)
	if album != "" && !validAlbumPath(album) {
		http.Error(w, "bad album", 400)
		return
	}
	dir := h.PhotosDir
	if album != "" {
		dir = filepath.Join(h.PhotosDir, filepath.FromSlash(album))
	}
	if !withinRoot(h.PhotosDir, dir) {
		http.Error(w, "path traversal", 400)
		return
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		http.Error(w, "album not found", 404)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	clean := make([]string, 0, len(req.Names))
	seen := make(map[string]bool, len(req.Names))
	for _, n := range req.Names {
		n = filepath.Base(strings.TrimSpace(n))
		if !validFileName(n) {
			http.Error(w, "bad name", 400)
			return
		}
		if seen[n] {
			continue
		}
		sub := filepath.Join(dir, n)
		sfi, err := os.Stat(sub)
		if err != nil || !sfi.IsDir() {
			http.Error(w, "unknown child: "+n, 400)
			return
		}
		seen[n] = true
		clean = append(clean, n)
	}
	if err := gallery.WriteOrder(dir, clean); err != nil {
		log.Printf("reorder %s: %v", album, err)
		http.Error(w, "save order failed", 500)
		return
	}
	gallery.InvalidateScan()
	writeJSON(w, map[string]string{"status": "ok"})
}

// SetHidden marks a photo as hidden (or clears the mark). Hidden photos are
// excluded from the public album view but remain visible in the admin UI
// and may still be used as the album cover.
func (h *AdminHandlers) SetHidden(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Album  string `json:"album"`
		Photo  string `json:"photo"`
		Hidden bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if !validAlbumPath(req.Album) {
		http.Error(w, "bad album", 400)
		return
	}
	photo := filepath.Base(req.Photo)
	if !gallery.IsImage(photo) {
		http.Error(w, "bad photo", 400)
		return
	}
	albumDir := filepath.Join(h.PhotosDir, filepath.FromSlash(req.Album))
	src := filepath.Join(albumDir, photo)
	if fi, err := os.Stat(src); err != nil || fi.IsDir() {
		http.Error(w, "source not found", 404)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	hidden := gallery.ReadHidden(albumDir)
	if req.Hidden {
		hidden[photo] = true
	} else {
		delete(hidden, photo)
	}
	names := make([]string, 0, len(hidden))
	for n := range hidden {
		names = append(names, n)
	}
	sort.Strings(names)
	if err := gallery.WriteHidden(albumDir, names); err != nil {
		log.Printf("sethidden %s/%s: %v", req.Album, photo, err)
		http.Error(w, "save hidden state failed", 500)
		return
	}
	gallery.InvalidateScan()
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *AdminHandlers) scanOpts() gallery.Options {
	return gallery.Options{
		WebPrefix:    h.WebPrefix,
		GridSize:     h.GridSize,
		LightboxSize: h.LightboxSize,
	}
}

// thumbSizes returns the sizes /thumb actually serves for this deployment
// (falling back to the built-in defaults), so cache invalidation removes
// the same files the handler generates.
func (h *AdminHandlers) thumbSizes() []int {
	g, l := h.GridSize, h.LightboxSize
	if g <= 0 {
		g = 480
	}
	if l <= 0 {
		l = 1600
	}
	if g == l {
		return []int{g}
	}
	return []int{g, l}
}

func (h *AdminHandlers) invalidateThumb(album, name string) {
	if h.ThumbsDir == "" {
		return
	}
	rel := path.Join(album, name)
	_ = os.RemoveAll(filepath.Join(h.ThumbsDir, filepath.FromSlash(rel)))
	for _, size := range h.thumbSizes() {
		_ = os.Remove(filepath.Join(h.ThumbsDir, rel+fmt.Sprintf(".%d.jpg", size)))
	}
}

func withinRoot(root, target string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func validFileName(s string) bool {
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\x00") {
		return false
	}
	return true
}

// copyFile copies src to dst atomically: it writes a temp file next to dst,
// syncs it, and renames it into place, so a crash or error never leaves a
// truncated half-written dst behind. When ext indicates a format that must
// not be stored as the cover (.gif/.bmp/.tif/.tiff), the image is decoded
// and re-encoded as JPEG instead of a byte copy.
func copyFile(src, dst, ext string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".cover-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := writeCoverFile(tmp, in, ext); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dst)
}

// writeCoverFile writes the cover bytes to w: a plain stream copy for
// JPEG/PNG/WebP, or a JPEG re-encode for formats the cover name must not
// carry (GIF/BMP/TIFF).
func writeCoverFile(w io.Writer, r io.Reader, ext string) error {
	switch ext {
	case ".gif", ".bmp", ".tif", ".tiff":
		img, err := imaging.Decode(r)
		if err != nil {
			return fmt.Errorf("decode %s: %w", ext, err)
		}
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 90})
	default:
		_, err := io.Copy(w, r)
		return err
	}
}
