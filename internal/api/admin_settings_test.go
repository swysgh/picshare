package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swys/picshare/internal/config"
)

// settingsFixture deliberately carries the fields the settings page must never
// touch: the credentials, the listen address, the data directories, and an
// unknown key a future/older build might own.
const settingsFixture = `{
  "listen": "127.0.0.1:8080",
  "photos_dir": "./photos",
  "thumbs_dir": "./thumbs",
  "admin_user": "admin",
  "admin_pass": "s3cret-pass",
  "site_title": "原站点",
  "default_lang": "zh",
  "grid_thumb_size": 480,
  "lightbox_size": 1600,
  "web_path_prefix": "/photos",
  "custom_note": "keep-me",
  "landing_backdrop_opacity": 40,
  "landing_remember": "7d"
}`

// testSettingsStore mirrors main.liveCfg: validate, persist, then swap.
type testSettingsStore struct {
	cfg  *config.Config
	path string
}

func (s *testSettingsStore) Settings() config.Settings { return s.cfg.Settings() }

func (s *testSettingsStore) ApplySettings(in config.Settings) error {
	next, err := s.cfg.WithSettings(in)
	if err != nil {
		return err
	}
	if err := config.SaveSettings(s.path, next.Settings()); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func newSettingsFixture(t *testing.T) (string, *testSettingsStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(settingsFixture), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return path, &testSettingsStore{cfg: cfg, path: path}
}

// settingsJSON builds a complete, valid settings body; mutate may corrupt one
// field per test.
func settingsJSON(t *testing.T, mutate func(m map[string]any)) string {
	t.Helper()
	body := map[string]any{
		"site_title":               "站点",
		"landing_enabled":          true,
		"landing_i18n":             emptyI18n(),
		"landing_backdrop_opacity": 45,
		"landing_remember":         "30d",
		"max_upload_mb":            512,
	}
	if mutate != nil {
		mutate(body)
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func emptyI18n() map[string]any {
	langs := func() map[string]any { return map[string]any{"zh": "", "ko": "", "en": ""} }
	return map[string]any{"title": langs(), "subtitle": langs(), "cta": langs()}
}

func postSettings(h *AdminHandlers, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdateSettings(rec, req)
	return rec
}

func readConfigMap(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse config after save: %v", err)
	}
	return m
}

// TestSettingsGetHidesSecrets: the response must expose exactly the
// allowlisted fields, and never the credentials or the deployment layout.
func TestSettingsGetHidesSecrets(t *testing.T) {
	_, store := newSettingsFixture(t)
	h := &AdminHandlers{Settings: store}

	rec := httptest.NewRecorder()
	h.GetSettings(rec, httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil))
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	for _, banned := range []string{"admin_pass", "admin_user", "listen", "photos_dir", "thumbs_dir"} {
		if _, ok := got[banned]; ok {
			t.Errorf("response must not contain %q", banned)
		}
	}
	for _, want := range config.SettingsKeys {
		if _, ok := got[want]; !ok {
			t.Errorf("response missing %q", want)
		}
	}
	if len(got) != len(config.SettingsKeys) {
		t.Errorf("response has %d fields, want %d: %v", len(got), len(config.SettingsKeys), got)
	}
	if strings.Contains(rec.Body.String(), "s3cret-pass") {
		t.Error("response leaks admin_pass")
	}
}

// TestSettingsUpdatePreservesOtherKeys: only the allowlisted keys change on
// disk; everything else (credentials, unknown keys, listen) is byte-identical,
// the file stays 0600, and the live store sees the new values immediately.
func TestSettingsUpdatePreservesOtherKeys(t *testing.T) {
	path, store := newSettingsFixture(t)
	before := readConfigMap(t, path)
	h := &AdminHandlers{Settings: store}

	in := store.cfg.Settings()
	in.SiteTitle = "新站点"
	in.LandingEnabled = false
	in.LandingBackdropOpacity = 77
	in.LandingRemember = "12h"
	in.MaxUploadMB = 32
	in.LandingI18n = config.LandingI18n{
		Title:    config.LandingTexts{Zh: "标题中文", Ko: "제목", En: "Title"},
		Subtitle: config.LandingTexts{Zh: "副标题中文"},
		CTA:      config.LandingTexts{En: "Enter"},
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	rec := postSettings(h, string(body))
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}

	after := readConfigMap(t, path)
	for _, key := range []string{
		"admin_pass", "admin_user", "listen", "photos_dir", "thumbs_dir",
		"custom_note", "default_lang", "grid_thumb_size", "lightbox_size", "web_path_prefix",
	} {
		if string(before[key]) != string(after[key]) {
			t.Errorf("%s changed by a settings save: %s -> %s", key, before[key], after[key])
		}
	}
	if got := string(after["site_title"]); got != `"新站点"` {
		t.Errorf("site_title on disk = %s, want \"新站点\"", got)
	}
	if got := string(after["max_upload_mb"]); got != "32" {
		t.Errorf("max_upload_mb on disk = %s, want 32", got)
	}
	if got := string(after["landing_enabled"]); got != "false" {
		t.Errorf("landing_enabled on disk = %s, want false", got)
	}

	live := store.Settings()
	if live.SiteTitle != "新站点" || live.MaxUploadMB != 32 || live.LandingEnabled {
		t.Errorf("live settings not updated: %+v", live)
	}
	if live.LandingI18n.Title.En != "Title" {
		t.Errorf("live landing_i18n lost: %+v", live.LandingI18n)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("config.json mode = %o, want 600", perm)
	}
}

// TestSettingsRejectsBadInput: each invalid body is a 400 with a Chinese
// reason in a JSON error body, and nothing is written to disk.
func TestSettingsRejectsBadInput(t *testing.T) {
	path, store := newSettingsFixture(t)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &AdminHandlers{Settings: store}

	cases := []struct {
		name    string
		mutate  func(m map[string]any)
		wantSub string
	}{
		{"unknown key", func(m map[string]any) { m["admin_pass"] = "hacked" }, "不支持的字段"},
		{"missing key", func(m map[string]any) { delete(m, "max_upload_mb") }, "缺少字段"},
		{"opacity 101", func(m map[string]any) { m["landing_backdrop_opacity"] = 101 }, "遮罩透明度"},
		{"opacity negative", func(m map[string]any) { m["landing_backdrop_opacity"] = -5 }, "遮罩透明度"},
		{"bad remember", func(m map[string]any) { m["landing_remember"] = "abc" }, "landing_remember"},
		{"negative mb", func(m map[string]any) { m["max_upload_mb"] = -1 }, "上传上限"},
		{"unknown language", func(m map[string]any) {
			m["landing_i18n"].(map[string]any)["title"].(map[string]any)["jp"] = "x"
		}, "不支持的语言"},
		{"unknown group", func(m map[string]any) {
			m["landing_i18n"].(map[string]any)["footer"] = map[string]any{"zh": "x"}
		}, "不支持的字段"},
		{"text too long", func(m map[string]any) {
			m["landing_i18n"].(map[string]any)["subtitle"].(map[string]any)["zh"] = strings.Repeat("字", config.MaxTextLen+1)
		}, "最长"},
		{"wrong type", func(m map[string]any) { m["site_title"] = 5 }, "字段类型"},
	}
	for _, c := range cases {
		rec := postSettings(h, settingsJSON(t, c.mutate))
		if rec.Code != 400 {
			t.Errorf("%s: want 400, got %d (body %s)", c.name, rec.Code, rec.Body.String())
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: error body is not JSON: %v", c.name, err)
			continue
		}
		if !strings.Contains(body["error"], c.wantSub) {
			t.Errorf("%s: error = %q, want it to contain %q", c.name, body["error"], c.wantSub)
		}
	}

	now, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(now) != string(original) {
		t.Error("a rejected save modified config.json")
	}
	if store.Settings().SiteTitle != "原站点" {
		t.Errorf("a rejected save changed the live config: %q", store.Settings().SiteTitle)
	}
}

// TestSettingsLandingI18nRoundTrip: all three languages survive a save/read
// cycle, and blanking one language leaves the others untouched.
func TestSettingsLandingI18nRoundTrip(t *testing.T) {
	_, store := newSettingsFixture(t)
	h := &AdminHandlers{Settings: store}

	full := config.LandingI18n{
		Title:    config.LandingTexts{Zh: "标题", Ko: "제목", En: "Title"},
		Subtitle: config.LandingTexts{Zh: "副标题", Ko: "부제목", En: "Subtitle"},
		CTA:      config.LandingTexts{Zh: "点击进入", Ko: "클릭", En: "Click"},
	}
	in := store.cfg.Settings()
	in.LandingI18n = full
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if rec := postSettings(h, string(body)); rec.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}
	if got := store.Settings().LandingI18n; got != full {
		t.Fatalf("landing_i18n round trip = %+v, want %+v", got, full)
	}

	// Blank ko only.
	partial := full
	partial.Subtitle.Ko = ""
	in = store.cfg.Settings()
	in.LandingI18n = partial
	body, err = json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if rec := postSettings(h, string(body)); rec.Code != 200 {
		t.Fatalf("want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}
	got := store.Settings().LandingI18n
	if got.Subtitle.Ko != "" {
		t.Errorf("ko subtitle = %q, want empty", got.Subtitle.Ko)
	}
	if got.Subtitle.Zh != "副标题" || got.Subtitle.En != "Subtitle" {
		t.Errorf("blanking ko changed other languages: %+v", got.Subtitle)
	}
	if got.Title != full.Title || got.CTA != full.CTA {
		t.Errorf("blanking ko changed other fields: %+v", got)
	}
}

// TestSettingsUnavailableWithoutStore: the endpoints answer 503 (not a panic)
// when the deployment did not wire a store.
func TestSettingsUnavailableWithoutStore(t *testing.T) {
	h := &AdminHandlers{}
	rec := httptest.NewRecorder()
	h.GetSettings(rec, httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET want 503, got %d", rec.Code)
	}
	if rec := postSettings(h, settingsJSON(t, nil)); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("POST want 503, got %d", rec.Code)
	}
}
