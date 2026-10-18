package web

import (
	"embed"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed *.html css/*.css js/*.js vendor/photoswipe/*.css vendor/photoswipe/*.js
var files embed.FS

// Options carries every value that is injected into the embedded HTML at
// request time. The values are plain strings because the templates are
// static files: <!--PICSHARE_*--> markers are replaced in place, and each
// replacement is HTML-escaped (thumbnail URLs contain "&w=...&v=...").
type Options struct {
	DefaultLang string
	SiteTitle   string

	LandingCTA      string
	// LandingTextsJSON is the per-language landing copy as compact JSON,
	// injected into landing.html's data-landing-texts attribute so the page
	// can pick the right language without a round trip.
	LandingTextsJSON  string
	LandingBG         string
	LandingBGPortrait string
	LandingBackdrop   string
}

func NewHandler(opts Options) http.Handler {
	sub, _ := fs.Sub(files, ".")
	replacer := newReplacer(opts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}
		if strings.HasPrefix(p, "/") {
			p = p[1:]
		}
		f, err := sub.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		ext := path.Ext(p)
		ct := mime.TypeByExtension(ext)
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		switch {
		case strings.HasSuffix(p, "admin.html"):
			// The admin shell is dynamic (auth-protected); never let a
			// shared browser cache serve it after logout.
			w.Header().Set("Cache-Control", "no-store")
		case p == "landing.html":
			// The landing page carries the remembered-entry decision: a
			// cached copy could be shown to a visitor whose cookie is
			// already set, or keep a stale background URL alive.
			w.Header().Set("Cache-Control", "no-store")
		case p == "index.html":
			// The gallery shell carries the injected site_title and the
			// landing decision; a cached copy would keep showing the old
			// title for up to an hour after a settings save.
			w.Header().Set("Cache-Control", "no-store")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		if ext == ".html" {
			data, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, "read error", 500)
				return
			}
			s := replacer.Replace(string(data))
			w.Header().Set("Content-Length", itoa(int64(len(s))))
			_, _ = io.WriteString(w, s)
			return
		}
		w.Header().Set("Content-Length", itoa(stat.Size()))
		_, _ = io.Copy(w, f)
	})
}

// newReplacer builds the marker table once per handler. Empty optional
// values intentionally replace the marker with an empty string: the page
// then falls back to i18n copy (data-i18n) or to the gradient background.
func newReplacer(opts Options) *strings.Replacer {
	return strings.NewReplacer(
		"<!--PICSHARE_DEFAULT_LANG-->", html.EscapeString(opts.DefaultLang),
		"<!--PICSHARE_SITE_TITLE-->", html.EscapeString(opts.SiteTitle),
		"<!--PICSHARE_LANDING_CTA-->", html.EscapeString(opts.LandingCTA),
		"<!--PICSHARE_LANDING_TEXTS-->", html.EscapeString(opts.LandingTextsJSON),
		"<!--PICSHARE_LANDING_BG-->", html.EscapeString(opts.LandingBG),
		"<!--PICSHARE_LANDING_BG_PORTRAIT-->", html.EscapeString(opts.LandingBGPortrait),
		"<!--PICSHARE_LANDING_BACKDROP-->", html.EscapeString(opts.LandingBackdrop),
	)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
