package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
