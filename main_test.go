package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swys/picshare/internal/config"
	webui "github.com/swys/picshare/web"
)

// liveCfgFixture has a site title, a full landing_i18n block, and the fields
// the settings page must never touch.
const liveCfgFixture = `{
  "listen": "127.0.0.1:8080",
  "photos_dir": "./photos",
  "thumbs_dir": "./thumbs",
  "admin_user": "admin",
  "admin_pass": "s3cret-pass",
  "site_title": "客户站点",
  "default_lang": "zh",
  "landing_i18n": {
    "title": {"zh": "中文大标题", "ko": "한국 제목", "en": "English Title"},
    "subtitle": {"zh": "中文副标题"},
    "cta": {"zh": "点击进入"}
  },
  "landing_backdrop_opacity": 60
}`

func loadLiveCfgFixture(t *testing.T) (*liveCfg, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(liveCfgFixture), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return &liveCfg{cfg: cfg, path: path}, path
}

// TestLiveCfgApplySettings: a save swaps the in-memory config and rewrites only
// the allowlisted keys; an invalid or unpersistable save leaves both alone.
func TestLiveCfgApplySettings(t *testing.T) {
	live, path := loadLiveCfgFixture(t)

	s := live.Settings()
	s.SiteTitle = "改过的标题"
	s.MaxUploadMB = 64
	if err := live.ApplySettings(s); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := live.Get().SiteTitle; got != "改过的标题" {
		t.Errorf("live site_title = %q, want 改过的标题", got)
	}
	if got := live.Get().MaxUploadMB; got != 64 {
		t.Errorf("live max_upload_mb = %d, want 64", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"admin_pass": "s3cret-pass"`)) {
		t.Errorf("admin_pass was lost from config.json:\n%s", raw)
	}

	// Invalid values are rejected before anything is written or swapped.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := live.Settings()
	bad.LandingBackdropOpacity = 101
	if err := live.ApplySettings(bad); err == nil {
		t.Fatal("want an error for opacity 101")
	}
	if got := live.Get().LandingBackdropOpacity; got != 60 {
		t.Errorf("rejected save changed the live opacity to %d", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("rejected save modified config.json")
	}

	// No config path (e.g. started without --config) must fail loudly rather
	// than apply a change that silently disappears on restart.
	noPath := &liveCfg{cfg: live.Get()}
	if err := noPath.ApplySettings(noPath.Settings()); err == nil {
		t.Fatal("want an error when the config path is empty")
	}
}

// TestBuildWebOptsInjection covers the two rendering bugs this change fixes:
// the configured site_title must reach index.html (and survive i18n.js), and
// the per-language landing copy must reach landing.html as an escaped data
// attribute.
func TestBuildWebOptsInjection(t *testing.T) {
	live, _ := loadLiveCfgFixture(t)
	h := webui.NewHandler(buildWebOpts(live.Get(), ""))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	index := rec.Body.String()
	if !strings.Contains(index, `data-site-title="客户站点"`) {
		t.Error("index.html is missing the injected data-site-title")
	}
	if strings.Contains(index, "PICSHARE_SITE_TITLE") {
		t.Error("index.html still contains an unreplaced marker")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("index.html Cache-Control = %q, want no-store", cc)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/landing.html", nil))
	landing := rec.Body.String()
	for _, want := range []string{"中文大标题", "한국 제목", "English Title", "中文副标题", "点击进入"} {
		if !strings.Contains(landing, want) {
			t.Errorf("landing.html is missing %q", want)
		}
	}
	if strings.Contains(landing, "PICSHARE_LANDING_TEXTS") {
		t.Error("landing.html still contains the unreplaced texts marker")
	}
	const attrStart = `data-landing-texts="`
	i := strings.Index(landing, attrStart)
	if i < 0 {
		t.Fatal("landing.html is missing data-landing-texts")
	}
	rest := landing[i+len(attrStart):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("data-landing-texts attribute is not terminated")
	}
	attr := rest[:end]
	if strings.ContainsAny(attr, "<>") {
		t.Errorf("data-landing-texts contains unescaped markup: %s", attr)
	}
	if !strings.Contains(attr, "&#34;") {
		t.Errorf("data-landing-texts JSON was not HTML-escaped: %s", attr)
	}
}
