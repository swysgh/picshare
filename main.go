package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/swys/picshare/internal/api"
	"github.com/swys/picshare/internal/auth"
	"github.com/swys/picshare/internal/config"
	"github.com/swys/picshare/internal/gallery"
	"github.com/swys/picshare/internal/thumb"
	webui "github.com/swys/picshare/web"
)

// version is the build version. It is injected at build time via
//
//	-ldflags "-X main.version=<version>"
//
// and defaults to "dev" for unversioned local builds. No version number is
// hard-coded here; releases always derive it from the git tag.
var version = "dev"

func main() {
	var (
		configPath  string
		initOnly    bool
		showVersion bool
	)
	flag.StringVar(&configPath, "config", "config.json", "path to config.json")
	flag.BoolVar(&initOnly, "init", false, "initialize default config and exit")
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.Parse()

	if showVersion {
		fmt.Println("picshare v" + version)
		return
	}

	logger := log.New(os.Stderr, "[picshare] ", log.LstdFlags|log.Lmsgprefix)

	if initOnly {
		if err := initConfig(configPath); err != nil {
			logger.Fatalf("init failed: %v", err)
		}
		logger.Printf("config initialized at %s", configPath)
		return
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		// No auto-init here: Load only returns wrapped errors, so the
		// old errors.Is(os.ErrNotExist) branch was dead code — and
		// silently generating a config with a weak default password
		// and starting anyway would be worse than failing loudly.
		// Use --init to write a starter config explicitly.
		logger.Fatalf("load config: %v", err)
	}
	if err := cfg.EnsureDirs(); err != nil {
		logger.Fatalf("ensure dirs: %v", err)
	}

	thumbHandler := &thumb.Handler{
		PhotosDir:    cfg.PhotosDir,
		ThumbsDir:    cfg.ThumbsDir,
		WebPrefix:    cfg.WebPathPrefix,
		GridSize:     cfg.GridThumbSize,
		LightboxSize: cfg.LightboxSize,
		EnableExif:   cfg.EnableExifRotate,
	}

	pub := &api.PublicHandlers{
		PhotosDir:    cfg.PhotosDir,
		ThumbsDir:    cfg.ThumbsDir,
		WebPrefix:    cfg.WebPathPrefix,
		GridSize:     cfg.GridThumbSize,
		LightboxSize: cfg.LightboxSize,
	}
	adm := &api.AdminHandlers{
		PhotosDir:    cfg.PhotosDir,
		ThumbsDir:    cfg.ThumbsDir,
		WebPrefix:    cfg.WebPathPrefix,
		GridSize:     cfg.GridThumbSize,
		LightboxSize: cfg.LightboxSize,
	}

	// live is the single source of truth for every setting the admin page can
	// change. Handlers read through Get() per request, so a save applies
	// without a restart; ApplySettings writes the file first and only then
	// swaps the pointer, so a failed write never leaves memory and disk out
	// of sync.
	live := &liveCfg{cfg: cfg, path: configPath}
	adm.UploadLimit = func() int { return live.Get().MaxUploadMB }
	adm.Settings = live

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/albums", pub.ListAlbums)
	mux.HandleFunc("GET /api/albums/{path...}", pub.GetAlbum)
	mux.HandleFunc("GET /i/{path...}", pub.ServeImage)
	mux.Handle("/thumb", thumbHandler)

	if cfg.WebPathPrefix != "" && cfg.WebPathPrefix != "/i" {
		mux.HandleFunc("GET "+cfg.WebPathPrefix+"/{path...}", pub.ServeImage)
	}
	adminGuard := func(next http.HandlerFunc) http.Handler {
		// Basic Auth first, then the stateless CSRF origin check for
		// non-GET requests (see auth.CSRFCheck for why it must never
		// grow an origin allowlist).
		return auth.CSRFCheck(auth.BasicAuth(cfg.AdminUser, cfg.AdminPass, next))
	}
	mux.Handle("GET /api/admin/manage", adminGuard(adm.ListManage))
	mux.Handle("GET /api/admin/manage/{path...}", adminGuard(adm.ListAlbumManage))
	mux.Handle("POST /api/admin/mkdir", adminGuard(adm.Mkdir))
	mux.Handle("POST /api/admin/upload", adminGuard(adm.Upload))
	mux.Handle("POST /api/admin/rename", adminGuard(adm.Rename))
	mux.Handle("POST /api/admin/delete", adminGuard(adm.Delete))
	mux.Handle("POST /api/admin/setcover", adminGuard(adm.SetCover))
	mux.Handle("POST /api/admin/sethidden", adminGuard(adm.SetHidden))
	mux.Handle("POST /api/admin/setlandingbg", adminGuard(adm.SetLandingBg))
	mux.Handle("POST /api/admin/reorder", adminGuard(adm.Reorder))
	mux.Handle("GET /api/admin/settings", adminGuard(adm.GetSettings))
	mux.Handle("POST /api/admin/settings", adminGuard(adm.UpdateSettings))

	// Landing background: the admin's pick (photos/.landing_bg) wins, then
	// an explicit landing_background, then the first album cover at lightbox
	// size (see defaultLandingBackground). Only .landing_bg is resolved per
	// request; the rest comes from the config loaded at startup.
	bgSource := &landingBGSource{cfg: cfg, logger: logger}
	bgLog := "-"
	if cfg.LandingEnabled {
		bgSource.fallback = cfg.LandingBackground
		if bgSource.fallback == "" {
			bgSource.fallback = defaultLandingBackground(cfg, logger)
		}
		if bg := bgSource.url(); bg != "" {
			bgLog = bg
		}
	}

	// The gallery shell and the landing page both carry injected config, and
	// both must reflect a settings save immediately. NewHandler only builds a
	// marker replacer, so rebuilding it per request is cheap (the landing page
	// already did this for the background).
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webui.NewHandler(buildWebOpts(live.Get(), "")).ServeHTTP(w, r)
	})

	// serveLanding renders the landing page with a background resolved for
	// this request, so .landing_bg changes are immediate too.
	serveLanding := func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/landing.html"
		webui.NewHandler(buildWebOpts(live.Get(), bgSource.url())).ServeHTTP(w, r)
	}

	// The landing routes must be registered before the "/" catch-all:
	// "GET /{$}" is Go 1.22+'s exact-match pattern for "/" only, so it
	// shadows the root of spaHandler without swallowing /gallery or
	// /album/... (a more specific pattern always wins).
	mux.HandleFunc("GET /enter", landingEnter(live))
	mux.HandleFunc("GET /landing.html", serveLanding)
	mux.HandleFunc("GET /{$}", landingGate(live, spa, serveLanding))
	mux.Handle("/", spaHandler(spa, cfg.WebPathPrefix))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           withHeaders(mux),
		ReadHeaderTimeout: 30 * time.Second,
		// IdleTimeout recycles keep-alive connections held open by
		// browsers/proxies; WriteTimeout is generous (10m) because
		// thumbnail generation of a huge source can legitimately take a
		// while on the first request.
		IdleTimeout:  120 * time.Second,
		WriteTimeout: 10 * time.Minute,
	}

	go func() {
		logger.Printf("listening on %s, photos=%s, thumbs=%s", cfg.Listen, cfg.PhotosDir, cfg.ThumbsDir)
		logger.Printf("landing enabled=%v remember=%s bg=%s", cfg.LandingEnabled, cfg.LandingRemember, bgLog)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Printf("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Printf("shutdown: %v", err)
	}
}

// liveCfg is the process-wide configuration plus the write path for the admin
// settings page. Every handler reads through Get(), so a saved setting applies
// without a restart.
type liveCfg struct {
	mu   sync.RWMutex
	cfg  *config.Config
	path string
}

func (l *liveCfg) Get() *config.Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cfg
}

// Settings returns the admin-editable view of the live config.
func (l *liveCfg) Settings() config.Settings { return l.Get().Settings() }

// ApplySettings validates s, persists it to the config file and only then
// swaps the in-memory config. The order matters: a failed write must leave the
// process on the values that are still on disk.
func (l *liveCfg) ApplySettings(s config.Settings) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	next, err := l.cfg.WithSettings(s)
	if err != nil {
		return err
	}
	if err := config.SaveSettings(l.path, next.Settings()); err != nil {
		return err
	}
	l.cfg = next
	return nil
}

// buildWebOpts snapshots the current config into the embedded-HTML options. It
// runs per request (see main) so a settings save is visible on the next page
// load.
func buildWebOpts(c *config.Config, landingBG string) webui.Options {
	return webui.Options{
		DefaultLang:       c.DefaultLang,
		SiteTitle:         c.SiteTitle,
		LandingTitle:      c.LandingTitleFor(c.DefaultLang),
		LandingSubtitle:   c.LandingSubtitleFor(c.DefaultLang),
		LandingCTA:        c.LandingCTAFor(c.DefaultLang),
		LandingTextsJSON:  c.LandingTextsJSON(),
		LandingBG:         landingBG,
		LandingBGPortrait: c.LandingBackgroundPortrait,
		LandingBackdrop:   strconv.FormatFloat(float64(c.LandingBackdropOpacity)/100, 'f', -1, 64),
	}
}

func withHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		// CSP: same-origin only, with inline styles allowed (current CSS
		// relies on a few inline style attributes) and data: images for
		// placeholders. PhotoSwipe loads its UI from same-origin vendor
		// files and displays same-origin /thumb + /photos images, so it
		// is unaffected. script-src is stated explicitly (falling back to
		// default-src would mean the same thing) so no page ever needs an
		// inline <script>: the default language is injected as a data
		// attribute instead.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v\n%s", rec, debugStack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}

func debugStack() string {
	const size = 4096
	buf := make([]byte, size)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// spaHandler serves the embedded frontend. webPrefix (web_path_prefix) is
// the URL space owned by image serving / nginx direct-offload; requests
// under it must not fall through to the SPA.
func spaHandler(h http.Handler, webPrefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/thumb" {
			http.NotFound(w, r)
			return
		}
		if webPrefix != "" && webPrefix != "/" && strings.HasPrefix(r.URL.Path, webPrefix+"/") {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/gallery", "/gallery/":
			// The gallery list used to live at "/"; it now has its own
			// path so that "/" can host the landing page.
			r.URL.Path = "/index.html"
		case "/admin", "/admin/":
			r.URL.Path = "/admin.html"
		}
		if strings.HasPrefix(r.URL.Path, "/admin/album/") {
			r.URL.Path = "/admin.html"
		}
		if strings.HasPrefix(r.URL.Path, "/album/") {
			r.URL.Path = "/index.html"
		}
		h.ServeHTTP(w, r)
	})
}

const landingCookie = "picshare_landing"

func landingCookieValue(cfg *config.Config) string {
	return "v" + strconv.Itoa(cfg.LandingVersion)
}

// landingGate serves GET /. A visitor whose cookie carries the current
// landing_version goes straight to the gallery; everyone else sees the
// landing page. "?landing=1" forces the page (preview/demo). With
// landing_enabled=false the gallery is served exactly as before. Both
// switches are read from the live config, so turning the page off in the
// admin settings applies to the next request. landing renders landing.html
// with a background resolved for this request.
func landingGate(live *liveCfg, web http.Handler, landing http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		if !cfg.LandingEnabled {
			r.URL.Path = "/index.html"
			web.ServeHTTP(w, r)
			return
		}
		if r.URL.Query().Get("landing") == "1" {
			landing(w, r)
			return
		}
		if cfg.LandingRememberTTL > 0 {
			// With landing_remember=off no cookie is ever written, so a
			// stale one must not skip the page either.
			if c, err := r.Cookie(landingCookie); err == nil && c.Value == landingCookieValue(cfg) {
				http.Redirect(w, r, "/gallery", http.StatusFound)
				return
			}
		}
		landing(w, r)
	}
}

// landingEnter is the landing CTA target: it remembers the visit (unless
// landing_remember=off) and redirects to the gallery either way.
func landingEnter(live *liveCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		if cfg.LandingRememberTTL > 0 {
			http.SetCookie(w, &http.Cookie{
				Name:     landingCookie,
				Value:    landingCookieValue(cfg),
				Path:     "/",
				MaxAge:   int(cfg.LandingRememberTTL.Seconds()),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/gallery", http.StatusFound)
	}
}

// landingBGSource resolves the landing page background for a render. The
// fallback (landing_background, else the first album cover) is resolved once
// at startup; photos/.landing_bg is re-read per request so a pick made in the
// admin UI applies without a restart.
type landingBGSource struct {
	cfg      *config.Config
	fallback string
	logger   *log.Logger

	mu sync.Mutex
	// warned is the last .landing_bg value already reported as unusable, so
	// a deleted background logs once instead of on every page view.
	warned string
}

func (s *landingBGSource) url() string {
	p := gallery.ReadLandingBg(s.cfg.PhotosDir)
	if p == "" {
		return s.fallback
	}
	u, ok := gallery.LandingBgThumbURL(s.cfg.PhotosDir, p, gallery.Options{
		WebPrefix:    s.cfg.WebPathPrefix,
		GridSize:     s.cfg.GridThumbSize,
		LightboxSize: s.cfg.LightboxSize,
	})
	s.mu.Lock()
	switch {
	case ok:
		s.warned = ""
	case p != s.warned:
		s.warned = p
		s.logger.Printf("landing background: .landing_bg 指向的照片不可用（%s），回落到默认封面", p)
	}
	s.mu.Unlock()
	if !ok {
		return s.fallback
	}
	return u
}

// defaultLandingBackground picks the first album cover as the landing
// background when landing_background is empty. It asks for the
// lightbox-sized thumbnail instead of the 480px grid thumb so the
// full-screen image is not upscaled. No cover anywhere is not an error:
// the page falls back to its gradient.
func defaultLandingBackground(cfg *config.Config, logger *log.Logger) string {
	albums, err := gallery.ScanOpts(cfg.PhotosDir, gallery.Options{
		WebPrefix:    cfg.WebPathPrefix,
		GridSize:     cfg.GridThumbSize,
		LightboxSize: cfg.LightboxSize,
	})
	if err != nil {
		logger.Printf("landing background: 扫描相册失败: %v", err)
		return ""
	}
	a := firstCoveredAlbum(albums)
	if a == nil {
		logger.Printf("landing background: 没有带封面的相册，使用渐变背景")
		return ""
	}
	return lightboxThumbURL(a.CoverThumb, cfg.LightboxSize)
}

// firstCoveredAlbum walks the album tree depth-first and returns the first
// album that has a cover.
func firstCoveredAlbum(albums []*gallery.Album) *gallery.Album {
	for _, a := range albums {
		if a == nil {
			continue
		}
		if a.CoverThumb != "" {
			return a
		}
		if c := firstCoveredAlbum(a.Children); c != nil {
			return c
		}
	}
	return nil
}

// lightboxThumbURL swaps the w parameter of a cover_thumb URL for the
// lightbox size. The rest of the URL is left byte-for-byte intact so the
// existing escaping and the "&v=" cache buster keep working.
func lightboxThumbURL(thumbURL string, size int) string {
	if size <= 0 {
		return thumbURL
	}
	i := strings.LastIndex(thumbURL, "&w=")
	if i < 0 {
		return thumbURL
	}
	rest := thumbURL[i+len("&w="):]
	end := strings.IndexByte(rest, '&')
	if end < 0 {
		end = len(rest)
	}
	return thumbURL[:i] + "&w=" + strconv.Itoa(size) + rest[end:]
}

func initConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists: %s", path)
	}
	cfg := config.Default()
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0600)
}
