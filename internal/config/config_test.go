package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const goodBody = `{
  "listen": "127.0.0.1:8080",
  "photos_dir": "PHOTOS",
  "thumbs_dir": "THUMBS",
  "admin_user": "admin",
  "admin_pass": "a-strong-pass",
  "default_lang": "zh",
  "web_path_prefix": "/photos"
}`

func TestValidateGood(t *testing.T) {
	dir := t.TempDir()
	body := strings.ReplaceAll(goodBody, "PHOTOS", filepath.Join(dir, "photos"))
	body = strings.ReplaceAll(body, "THUMBS", filepath.Join(dir, "thumbs"))
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("good config rejected: %v", err)
	}
	if cfg.MaxUploadMB != DefaultMaxUploadMB {
		t.Errorf("MaxUploadMB default = %d, want %d", cfg.MaxUploadMB, DefaultMaxUploadMB)
	}
}

func TestValidateListen(t *testing.T) {
	dir := t.TempDir()
	for _, listen := range []string{"", "not-a-host-port", "127.0.0.1"} {
		body := strings.ReplaceAll(goodBody, "127.0.0.1:8080", listen)
		body = strings.ReplaceAll(body, "PHOTOS", filepath.Join(dir, "photos"))
		body = strings.ReplaceAll(body, "THUMBS", filepath.Join(dir, "thumbs"))
		if _, err := Load(writeConfig(t, body)); err == nil {
			t.Errorf("listen %q should be rejected", listen)
		}
	}
}

func TestValidateWebPathPrefix(t *testing.T) {
	dir := t.TempDir()
	bad := []string{"photos", "/photos/", "/a{b", "/", "/api", "/thumb", "/i", "/admin"}
	for _, prefix := range bad {
		body := strings.ReplaceAll(goodBody, `"web_path_prefix": "/photos"`, `"web_path_prefix": "`+prefix+`"`)
		body = strings.ReplaceAll(body, "PHOTOS", filepath.Join(dir, "photos"))
		body = strings.ReplaceAll(body, "THUMBS", filepath.Join(dir, "thumbs"))
		if _, err := Load(writeConfig(t, body)); err == nil {
			t.Errorf("web_path_prefix %q should be rejected", prefix)
		}
	}
}

func TestValidateDirs(t *testing.T) {
	dir := t.TempDir()
	photos := filepath.Join(dir, "photos")
	thumbs := filepath.Join(dir, "thumbs")

	// thumbs == photos
	body := strings.ReplaceAll(goodBody, "PHOTOS", photos)
	body = strings.ReplaceAll(body, "THUMBS", photos)
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Error("thumbs_dir == photos_dir should be rejected")
	}

	// thumbs inside photos
	body = strings.ReplaceAll(goodBody, "PHOTOS", photos)
	body = strings.ReplaceAll(body, "THUMBS", filepath.Join(photos, "thumbs"))
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Error("thumbs_dir inside photos_dir should be rejected")
	}
	_ = thumbs
}

func TestValidateDefaultLang(t *testing.T) {
	dir := t.TempDir()
	body := strings.ReplaceAll(goodBody, `"default_lang": "zh"`, `"default_lang": "jp"`)
	body = strings.ReplaceAll(body, "PHOTOS", filepath.Join(dir, "photos"))
	body = strings.ReplaceAll(body, "THUMBS", filepath.Join(dir, "thumbs"))
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Error("default_lang jp should be rejected")
	}
}

func TestValidateWeakPassword(t *testing.T) {
	dir := t.TempDir()
	cases := []string{
		`"admin_user": "admin", "admin_pass": "admin"`,
		`"admin_user": "admin", "admin_pass": "change-me"`,
		`"admin_user": "root", "admin_pass": "short"`,
	}
	for _, c := range cases {
		body := `{
  "listen": "127.0.0.1:8080",
  "photos_dir": "` + filepath.Join(dir, "photos") + `",
  "thumbs_dir": "` + filepath.Join(dir, "thumbs") + `",
  ` + c + `,
  "default_lang": "zh",
  "web_path_prefix": "/photos"
}`
		_, err := Load(writeConfig(t, body))
		if err == nil {
			t.Errorf("%s should be rejected as weak", c)
		} else if !strings.Contains(err.Error(), "admin_pass") && !strings.Contains(err.Error(), "强口令") {
			t.Errorf("error should mention the password rule, got: %v", err)
		}
	}
}

func TestValidateMaxUploadMB(t *testing.T) {
	dir := t.TempDir()
	body := `{
  "listen": "127.0.0.1:8080",
  "photos_dir": "` + filepath.Join(dir, "photos") + `",
  "thumbs_dir": "` + filepath.Join(dir, "thumbs") + `",
  "admin_user": "admin",
  "admin_pass": "a-strong-pass",
  "max_upload_mb": -1
}`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Error("negative max_upload_mb should be rejected")
	}
}

func TestParseRemember(t *testing.T) {
	ok := []struct {
		in   string
		want time.Duration
	}{
		{"off", 0},
		{"OFF", 0},
		{"12h", 12 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{" 30d ", 30 * 24 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, c := range ok {
		got, err := ParseRemember(c.in)
		if err != nil {
			t.Errorf("ParseRemember(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRemember(%q) = %v, want %v", c.in, got, c.want)
		}
	}

	bad := []string{"", "0d", "-1d", "d", "1.5d", "abc", "0s", "-12h", "99999d"}
	for _, in := range bad {
		if _, err := ParseRemember(in); err == nil {
			t.Errorf("ParseRemember(%q) should be rejected", in)
		}
	}
}

func TestValidateLanding(t *testing.T) {
	dir := t.TempDir()
	base := func(extra string) string {
		body := `{
  "listen": "127.0.0.1:8080",
  "photos_dir": "` + filepath.Join(dir, "photos") + `",
  "thumbs_dir": "` + filepath.Join(dir, "thumbs") + `",
  "admin_user": "admin",
  "admin_pass": "a-strong-pass"` + extra + `
}`
		return body
	}

	// Omitted landing_* keys must keep the defaults from Default().
	cfg, err := Load(writeConfig(t, base("")))
	if err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
	if !cfg.LandingEnabled {
		t.Error("landing_enabled should default to true")
	}
	if cfg.LandingVersion != 1 {
		t.Errorf("landing_version default = %d, want 1", cfg.LandingVersion)
	}
	if cfg.LandingRememberTTL != 30*24*time.Hour {
		t.Errorf("landing_remember default TTL = %v, want 720h", cfg.LandingRememberTTL)
	}
	if cfg.LandingBackdropOpacity != 45 {
		t.Errorf("landing_backdrop_opacity default = %d, want 45", cfg.LandingBackdropOpacity)
	}

	// Explicit kill switch + off must survive loading.
	cfg, err = Load(writeConfig(t, base(`,
  "landing_enabled": false,
  "landing_remember": "off",
  "landing_version": 3`)))
	if err != nil {
		t.Fatalf("explicit landing config rejected: %v", err)
	}
	if cfg.LandingEnabled {
		t.Error("landing_enabled=false was not honored")
	}
	if cfg.LandingRememberTTL != 0 {
		t.Errorf("landing_remember=off TTL = %v, want 0", cfg.LandingRememberTTL)
	}
	if cfg.LandingVersion != 3 {
		t.Errorf("landing_version = %d, want 3", cfg.LandingVersion)
	}

	// Out-of-range backdrop and bad remember values must fail loudly.
	for _, extra := range []string{
		`,
  "landing_backdrop_opacity": 101`,
		`,
  "landing_backdrop_opacity": -1`,
		`,
  "landing_remember": "30days"`,
		`,
  "landing_remember": ""`,
	} {
		if _, err := Load(writeConfig(t, base(extra))); err == nil {
			t.Errorf("config with%s should be rejected", extra)
		}
	}
}
