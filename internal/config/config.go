package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultMaxUploadMB is the per-request upload body cap (in MB) when
// max_upload_mb is unset. 0 explicitly disables the limit.
const DefaultMaxUploadMB = 512

// MaxTextLen bounds every admin-editable free-text setting. It keeps the
// settings payload (and the JSON embedded into landing.html) small, and
// stops a pasted novel from bloating config.json.
const MaxTextLen = 200

// Langs are the languages the landing copy may be overridden in. It matches
// the keys of the i18n dictionary in web/js/i18n.js.
var Langs = []string{"zh", "ko", "en"}

// LandingTexts is one landing-page string in every supported language. An
// empty value falls back to the legacy single-language config field, then to
// the i18n dictionary in the browser.
type LandingTexts struct {
	Zh string `json:"zh"`
	Ko string `json:"ko"`
	En string `json:"en"`
}

// For returns the copy for lang, or "" when lang has no override.
func (t LandingTexts) For(lang string) string {
	switch lang {
	case "zh":
		return t.Zh
	case "ko":
		return t.Ko
	case "en":
		return t.En
	}
	return ""
}

func (t LandingTexts) trimmed() LandingTexts {
	return LandingTexts{
		Zh: strings.TrimSpace(t.Zh),
		Ko: strings.TrimSpace(t.Ko),
		En: strings.TrimSpace(t.En),
	}
}

// LandingI18n is the per-language override of the landing page copy. The
// legacy landing_title/landing_subtitle/landing_cta_text fields stay in the
// config as the fallback, so an existing config keeps rendering unchanged
// without a migration.
type LandingI18n struct {
	Title    LandingTexts `json:"title"`
	Subtitle LandingTexts `json:"subtitle"`
	CTA      LandingTexts `json:"cta"`
}

func (t LandingI18n) trimmed() LandingI18n {
	return LandingI18n{
		Title:    t.Title.trimmed(),
		Subtitle: t.Subtitle.trimmed(),
		CTA:      t.CTA.trimmed(),
	}
}

// Settings is the admin-editable subset of Config. It must never grow an
// admin_pass / admin_user / listen / directory field: the settings endpoint
// is browser-reachable and its response is rendered into the admin page.
type Settings struct {
	SiteTitle              string      `json:"site_title"`
	LandingEnabled         bool        `json:"landing_enabled"`
	LandingI18n            LandingI18n `json:"landing_i18n"`
	LandingBackdropOpacity int         `json:"landing_backdrop_opacity"`
	LandingRemember        string      `json:"landing_remember"`
	MaxUploadMB            int         `json:"max_upload_mb"`
}

// SettingsKeys is the allowlist of config.json keys the settings page may
// rewrite. Every other key in the file (admin_pass above all) is preserved
// byte for byte by SaveSettings.
var SettingsKeys = []string{
	"site_title",
	"landing_enabled",
	"landing_i18n",
	"landing_backdrop_opacity",
	"landing_remember",
	"max_upload_mb",
}

// IsSettingsKey reports whether key may be written back to config.json.
func IsSettingsKey(key string) bool {
	for _, k := range SettingsKeys {
		if k == key {
			return true
		}
	}
	return false
}

type Config struct {
	Listen           string `json:"listen"`
	PhotosDir        string `json:"photos_dir"`
	ThumbsDir        string `json:"thumbs_dir"`
	AdminUser        string `json:"admin_user"`
	AdminPass        string `json:"admin_pass"`
	SiteTitle        string `json:"site_title"`
	DefaultLang      string `json:"default_lang"`
	GridThumbSize    int    `json:"grid_thumb_size"`
	LightboxSize     int    `json:"lightbox_size"`
	EnableExifRotate bool   `json:"enable_exif_rotation"`
	WebPathPrefix    string `json:"web_path_prefix"`
	MaxUploadMB      int    `json:"max_upload_mb"`

	// Landing page (the root "/" screen). LandingEnabled defaults to true
	// in Default(); a config file that omits it therefore keeps the
	// feature on, and writing "landing_enabled": false is the kill switch
	// that restores the pre-landing behaviour of "/".
	LandingEnabled            bool        `json:"landing_enabled"`
	LandingTitle              string      `json:"landing_title"`
	LandingSubtitle           string      `json:"landing_subtitle"`
	LandingCTAText            string      `json:"landing_cta_text"`
	LandingI18n               LandingI18n `json:"landing_i18n"`
	LandingBackground         string      `json:"landing_background"`
	LandingBackgroundPortrait string      `json:"landing_background_portrait"`
	LandingBackdropOpacity    int         `json:"landing_backdrop_opacity"`
	LandingRemember           string      `json:"landing_remember"`
	LandingVersion            int         `json:"landing_version"`

	// LandingRememberTTL is derived from LandingRemember by validate();
	// it never comes from JSON (time.Duration has no JSON form).
	LandingRememberTTL time.Duration `json:"-"`
}

func Default() *Config {
	return &Config{
		Listen:           "127.0.0.1:8080",
		PhotosDir:        "./photos",
		ThumbsDir:        "./thumbs",
		AdminUser:        "admin",
		AdminPass:        "admin",
		SiteTitle:        "Product Gallery",
		DefaultLang:      "zh",
		GridThumbSize:    480,
		LightboxSize:     1600,
		EnableExifRotate: true,
		WebPathPrefix:    "/photos",
		MaxUploadMB:      DefaultMaxUploadMB,

		LandingEnabled:         true,
		LandingBackdropOpacity: 45,
		LandingRemember:        "30d",
		LandingVersion:         1,
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := validate(cfg); err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(cfg.PhotosDir)
	cfg.PhotosDir = abs
	abs, _ = filepath.Abs(cfg.ThumbsDir)
	cfg.ThumbsDir = abs
	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.AdminUser == "" || cfg.AdminPass == "" {
		return errors.New("admin_user and admin_pass must be set")
	}
	if cfg.Listen == "" {
		return errors.New("listen 不能为空（例如 127.0.0.1:8080）")
	}
	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		return fmt.Errorf("listen %q 不是合法的 host:port: %v", cfg.Listen, err)
	}
	if cfg.GridThumbSize <= 0 {
		cfg.GridThumbSize = 480
	}
	if cfg.LightboxSize <= 0 {
		cfg.LightboxSize = 1600
	}
	switch cfg.DefaultLang {
	case "":
		cfg.DefaultLang = "zh"
	case "zh", "ko", "en":
	default:
		return fmt.Errorf("default_lang 只允许 zh/ko/en（当前 %q）", cfg.DefaultLang)
	}
	if cfg.SiteTitle == "" {
		cfg.SiteTitle = "Product Gallery"
	}
	if n := utf8.RuneCountInString(cfg.SiteTitle); n > MaxTextLen {
		return fmt.Errorf("site_title 最长 %d 个字符（当前 %d）", MaxTextLen, n)
	}
	if err := validateWebPathPrefix(cfg); err != nil {
		return err
	}
	if cfg.MaxUploadMB < 0 {
		return fmt.Errorf("max_upload_mb 不能为负数（当前 %d；0 = 不限制）", cfg.MaxUploadMB)
	}
	if err := validateDirs(cfg); err != nil {
		return err
	}
	if err := validateAdminPass(cfg); err != nil {
		return err
	}
	if err := validateLanding(cfg); err != nil {
		return err
	}
	return nil
}

func validateLanding(cfg *Config) error {
	if cfg.LandingBackdropOpacity < 0 || cfg.LandingBackdropOpacity > 100 {
		return fmt.Errorf("landing_backdrop_opacity 必须在 0-100 之间（当前 %d）", cfg.LandingBackdropOpacity)
	}
	if err := validateLandingTexts(cfg.LandingI18n); err != nil {
		return err
	}
	if cfg.LandingVersion <= 0 {
		// A cookie value of "v0" is valid but meaningless; treat a missing
		// or nonsensical version as the first one.
		cfg.LandingVersion = 1
	}
	ttl, err := ParseRemember(cfg.LandingRemember)
	if err != nil {
		return err
	}
	cfg.LandingRememberTTL = ttl
	return nil
}

// LandingTitleFor resolves the landing page title for lang. The title is
// also the document <title> and the h1, so it never resolves to "": the
// fallback chain ends at site_title.
func (c *Config) LandingTitleFor(lang string) string {
	if v := c.LandingI18n.Title.For(lang); v != "" {
		return v
	}
	if c.LandingTitle != "" {
		return c.LandingTitle
	}
	return c.SiteTitle
}

// LandingSubtitleFor resolves the landing subtitle for lang: the per-language
// override, then the legacy single-language field, then "" (the browser falls
// back to the i18n dictionary).
func (c *Config) LandingSubtitleFor(lang string) string {
	if v := c.LandingI18n.Subtitle.For(lang); v != "" {
		return v
	}
	return c.LandingSubtitle
}

// LandingCTAFor resolves the landing CTA hint for lang, with the same
// fallback chain as LandingSubtitleFor.
func (c *Config) LandingCTAFor(lang string) string {
	if v := c.LandingI18n.CTA.For(lang); v != "" {
		return v
	}
	return c.LandingCTAText
}

// LandingTextsJSON returns the per-language landing copy as compact JSON for
// the data-landing-texts attribute of landing.html. The legacy fields are
// already folded in, so the browser only has to pick a language.
func (c *Config) LandingTextsJSON() string {
	out := LandingI18n{
		Title:    LandingTexts{Zh: c.LandingTitleFor("zh"), Ko: c.LandingTitleFor("ko"), En: c.LandingTitleFor("en")},
		Subtitle: LandingTexts{Zh: c.LandingSubtitleFor("zh"), Ko: c.LandingSubtitleFor("ko"), En: c.LandingSubtitleFor("en")},
		CTA:      LandingTexts{Zh: c.LandingCTAFor("zh"), Ko: c.LandingCTAFor("ko"), En: c.LandingCTAFor("en")},
	}
	b, err := json.Marshal(out)
	if err != nil {
		// Only reachable for unsupported value types; an empty object makes
		// the page fall back to the i18n dictionary.
		return "{}"
	}
	return string(b)
}

// Settings returns the admin-editable view of the configuration.
func (c *Config) Settings() Settings {
	return Settings{
		SiteTitle:              c.SiteTitle,
		LandingEnabled:         c.LandingEnabled,
		LandingI18n:            c.LandingI18n,
		LandingBackdropOpacity: c.LandingBackdropOpacity,
		LandingRemember:        c.LandingRemember,
		MaxUploadMB:            c.MaxUploadMB,
	}
}

// patch renders the settings as the config.json key/value set SaveSettings
// writes. Keys not in SettingsKeys never appear here.
func (s Settings) patch() map[string]any {
	return map[string]any{
		"site_title":               s.SiteTitle,
		"landing_enabled":          s.LandingEnabled,
		"landing_i18n":             s.LandingI18n,
		"landing_backdrop_opacity": s.LandingBackdropOpacity,
		"landing_remember":         s.LandingRemember,
		"max_upload_mb":            s.MaxUploadMB,
	}
}

// WithSettings returns a copy of c with s applied and re-validated. c itself
// is never mutated, so a rejected save cannot corrupt the live config.
func (c *Config) WithSettings(s Settings) (*Config, error) {
	next := *c
	next.SiteTitle = strings.TrimSpace(s.SiteTitle)
	next.LandingEnabled = s.LandingEnabled
	next.LandingI18n = s.LandingI18n.trimmed()
	next.LandingBackdropOpacity = s.LandingBackdropOpacity
	next.LandingRemember = strings.TrimSpace(strings.ToLower(s.LandingRemember))
	next.MaxUploadMB = s.MaxUploadMB
	if err := validate(&next); err != nil {
		return nil, err
	}
	return &next, nil
}

// SaveSettings writes the editable subset of s back into the config file at
// path. Every other key already in the file — admin_pass above all, but also
// listen, directories and keys this build does not know — is preserved byte
// for byte, so the admin UI can never drop or overwrite a credential.
//
// The write is atomic (temp file in the same directory, fsync, rename) and
// the result is 0600: the file holds the admin password.
func SaveSettings(path string, s Settings) error {
	if path == "" {
		return errors.New("没有配置文件路径，无法保存设置（启动时用 --config 指定）")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	for key, value := range s.patch() {
		if !IsSettingsKey(key) {
			return fmt.Errorf("%s 不允许通过设置页修改", key)
		}
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", key, err)
		}
		doc[key] = b
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	out = append(out, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".picshare-config-*.json")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(out); err != nil {
		cleanup()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", err)
	}
	// Keep the owner of the file being replaced: the service often runs as
	// root while the operator edits the config as their own user.
	chownLike(tmpName, path)
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// validateLandingTexts bounds the per-language landing copy. Unknown language
// keys are rejected by the settings API, not here (JSON decoding already
// ignores them when the file is loaded).
func validateLandingTexts(t LandingI18n) error {
	fields := []struct {
		name string
		v    LandingTexts
	}{
		{"title", t.Title},
		{"subtitle", t.Subtitle},
		{"cta", t.CTA},
	}
	for _, f := range fields {
		for _, lang := range Langs {
			if n := utf8.RuneCountInString(f.v.For(lang)); n > MaxTextLen {
				return fmt.Errorf("landing_i18n.%s.%s 最长 %d 个字符（当前 %d）", f.name, lang, MaxTextLen, n)
			}
		}
	}
	return nil
}

// ParseRemember converts landing_remember into the landing cookie's TTL.
// "off" disables the cookie (0). time.ParseDuration does not understand
// "d", so day values are parsed here; everything else falls through to it.
func ParseRemember(s string) (time.Duration, error) {
	v := strings.TrimSpace(strings.ToLower(s))
	switch v {
	case "":
		return 0, errors.New("landing_remember 不能为空（允许 off / 12h / 7d / 30d）")
	case "off":
		return 0, nil
	}
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil || days <= 0 || days > 3650 {
			return 0, fmt.Errorf("landing_remember %q 不是合法的天数（允许 1d-3650d，或 off / 12h）", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("landing_remember %q 非法（允许 off / 12h / 7d / 30d）", s)
	}
	return d, nil
}

// reservedPrefixes are URL paths owned by the app's own mux routes; a
// web_path_prefix equal to any of them would shadow or duplicate those
// routes. Note "/photos" is NOT reserved: it is the default value of
// web_path_prefix and the expected production value behind nginx.
var reservedPrefixes = []string{"/api", "/thumb", "/i", "/admin"}

func validateWebPathPrefix(cfg *Config) error {
	if cfg.WebPathPrefix == "" {
		cfg.WebPathPrefix = "/photos"
	}
	p := cfg.WebPathPrefix
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("web_path_prefix 必须以 / 开头（当前 %q）", p)
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return fmt.Errorf("web_path_prefix 不能以 / 结尾（当前 %q）", p)
	}
	if strings.Contains(p, "{") {
		return fmt.Errorf("web_path_prefix 不能包含 '{'（当前 %q）", p)
	}
	if p == "/" {
		return fmt.Errorf("web_path_prefix 不能是 \"/\"，会与前端路由冲突")
	}
	for _, r := range reservedPrefixes {
		if p == r {
			return fmt.Errorf("web_path_prefix 不能使用保留路径 %s", r)
		}
	}
	return nil
}

func validateDirs(cfg *Config) error {
	absPhotos, err := filepath.Abs(cfg.PhotosDir)
	if err != nil {
		return fmt.Errorf("photos_dir 无法解析: %w", err)
	}
	absThumbs, err := filepath.Abs(cfg.ThumbsDir)
	if err != nil {
		return fmt.Errorf("thumbs_dir 无法解析: %w", err)
	}
	if absThumbs == absPhotos {
		return errors.New("thumbs_dir 不能与 photos_dir 相同")
	}
	if rel, err := filepath.Rel(absPhotos, absThumbs); err == nil &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("thumbs_dir 不能位于 photos_dir 之下，否则缩略图会被当作照片扫描")
	}
	return nil
}

func validateAdminPass(cfg *Config) error {
	lower := strings.ToLower(strings.TrimSpace(cfg.AdminPass))
	switch {
	case cfg.AdminUser == "admin" && lower == "admin":
		return errors.New("admin_pass 不能是 admin/admin，请改成强口令再启动")
	case lower == "change-me" || strings.HasPrefix(lower, "admin/change-me"):
		return errors.New("admin_pass 不能是 change-me，请改成强口令再启动")
	case len(cfg.AdminPass) < 8:
		return fmt.Errorf("admin_pass 至少需要 8 位（当前 %d 位）", len(cfg.AdminPass))
	}
	return nil
}

func (c *Config) EnsureDirs() error {
	for _, d := range []string{c.PhotosDir, c.ThumbsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
