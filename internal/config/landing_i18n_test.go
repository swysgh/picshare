package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestLandingTextFallbackChain pins the documented fallback order:
// landing_i18n[lang] → legacy single-language field → (title only) site_title.
func TestLandingTextFallbackChain(t *testing.T) {
	c := Default()
	c.SiteTitle = "站点"
	c.LandingTitle = "旧标题"
	c.LandingSubtitle = "旧副标题"
	c.LandingCTAText = "旧提示"

	// No per-language override at all: every language shows the legacy value,
	// which is exactly how the deployment behaved before this change.
	for _, lang := range Langs {
		if got := c.LandingTitleFor(lang); got != "旧标题" {
			t.Errorf("title(%s) = %q, want 旧标题", lang, got)
		}
		if got := c.LandingSubtitleFor(lang); got != "旧副标题" {
			t.Errorf("subtitle(%s) = %q, want 旧副标题", lang, got)
		}
		if got := c.LandingCTAFor(lang); got != "旧提示" {
			t.Errorf("cta(%s) = %q, want 旧提示", lang, got)
		}
	}

	// An override for one language wins there and nowhere else.
	c.LandingI18n.Subtitle = LandingTexts{Zh: "新副标题"}
	if got := c.LandingSubtitleFor("zh"); got != "新副标题" {
		t.Errorf("zh subtitle = %q, want 新副标题", got)
	}
	if got := c.LandingSubtitleFor("ko"); got != "旧副标题" {
		t.Errorf("ko subtitle = %q, want the legacy fallback", got)
	}

	// The title never resolves to "": with no legacy title either, it is the
	// site title (it is also the document <title> and the h1).
	c.LandingTitle = ""
	if got := c.LandingTitleFor("en"); got != "站点" {
		t.Errorf("title fallback = %q, want 站点", got)
	}

	// An empty config leaves subtitle/cta empty so the browser can fall back
	// to the i18n dictionary.
	empty := Default()
	empty.LandingSubtitle = ""
	empty.LandingCTAText = ""
	if got := empty.LandingSubtitleFor("zh"); got != "" {
		t.Errorf("empty subtitle = %q, want empty", got)
	}
	if got := empty.LandingCTAFor("en"); got != "" {
		t.Errorf("empty cta = %q, want empty", got)
	}
}

// TestLandingTextsJSON: the attribute payload resolves the legacy fields
// server-side and carries all three languages.
func TestLandingTextsJSON(t *testing.T) {
	c := Default()
	c.SiteTitle = "站点"
	c.LandingSubtitle = "旧副标题"
	c.LandingI18n.Title = LandingTexts{Zh: "中", Ko: "한", En: "EN"}

	var got LandingI18n
	if err := json.Unmarshal([]byte(c.LandingTextsJSON()), &got); err != nil {
		t.Fatalf("LandingTextsJSON is not valid JSON: %v", err)
	}
	if got.Title != (LandingTexts{Zh: "中", Ko: "한", En: "EN"}) {
		t.Errorf("title = %+v", got.Title)
	}
	if got.Subtitle != (LandingTexts{Zh: "旧副标题", Ko: "旧副标题", En: "旧副标题"}) {
		t.Errorf("subtitle = %+v, want the legacy value in every language", got.Subtitle)
	}
	if got.CTA != (LandingTexts{}) {
		t.Errorf("cta = %+v, want empty", got.CTA)
	}
}

// TestWithSettingsRejectsInvalid: a bad form post never produces a config the
// process could not have loaded at startup.
func TestWithSettingsRejectsInvalid(t *testing.T) {
	c := Default()
	c.AdminUser = "admin"
	c.AdminPass = "s3cret-pass"
	c.PhotosDir = "/tmp/photos"
	c.ThumbsDir = "/tmp/thumbs"

	cases := []struct {
		name string
		edit func(s *Settings)
		want string
	}{
		{"opacity high", func(s *Settings) { s.LandingBackdropOpacity = 101 }, "landing_backdrop_opacity"},
		{"opacity low", func(s *Settings) { s.LandingBackdropOpacity = -1 }, "landing_backdrop_opacity"},
		{"bad remember", func(s *Settings) { s.LandingRemember = "abc" }, "landing_remember"},
		{"negative upload", func(s *Settings) { s.MaxUploadMB = -1 }, "max_upload_mb"},
		{"long text", func(s *Settings) {
			s.LandingI18n.CTA = LandingTexts{Zh: strings.Repeat("字", MaxTextLen+1)}
		}, "landing_i18n.cta.zh"},
	}
	for _, tc := range cases {
		in := c.Settings()
		tc.edit(&in)
		if _, err := c.WithSettings(in); err == nil {
			t.Errorf("%s: want an error", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want it to mention %q", tc.name, err, tc.want)
		}
	}

	// The receiver must be untouched by a rejected apply.
	if c.LandingBackdropOpacity == 101 || c.MaxUploadMB == -1 {
		t.Error("WithSettings mutated the receiver")
	}
}
