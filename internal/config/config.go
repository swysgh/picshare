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
)

// DefaultMaxUploadMB is the per-request upload body cap (in MB) when
// max_upload_mb is unset. 0 explicitly disables the limit.
const DefaultMaxUploadMB = 512

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
	LandingEnabled            bool   `json:"landing_enabled"`
	LandingTitle              string `json:"landing_title"`
	LandingSubtitle           string `json:"landing_subtitle"`
	LandingCTAText            string `json:"landing_cta_text"`
	LandingBackground         string `json:"landing_background"`
	LandingBackgroundPortrait string `json:"landing_background_portrait"`
	LandingBackdropOpacity    int    `json:"landing_backdrop_opacity"`
	LandingRemember           string `json:"landing_remember"`
	LandingVersion            int    `json:"landing_version"`

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
