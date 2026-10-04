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
	"strings"
	"syscall"
	"time"

	"github.com/swys/picshare/internal/api"
	"github.com/swys/picshare/internal/auth"
	"github.com/swys/picshare/internal/config"
	"github.com/swys/picshare/internal/thumb"
	webui "github.com/swys/picshare/web"
)

// version is the build version. It is injected at build time via
//   -ldflags "-X main.version=<version>"
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
		if errors.Is(err, os.ErrNotExist) {
			if err := initConfig(configPath); err != nil {
				logger.Fatalf("auto-init failed: %v", err)
			}
			logger.Printf("config not found, wrote default to %s — please edit and restart", configPath)
			return
		}
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
		PhotosDir: cfg.PhotosDir,
		ThumbsDir: cfg.ThumbsDir,
		WebPrefix: cfg.WebPathPrefix,
	}
	adm := &api.AdminHandlers{
		PhotosDir:    cfg.PhotosDir,
		ThumbsDir:    cfg.ThumbsDir,
		WebPrefix:    cfg.WebPathPrefix,
		ThumbHandler: thumbHandler,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/albums", pub.ListAlbums)
	mux.HandleFunc("GET /api/albums/{path...}", pub.GetAlbum)
	mux.HandleFunc("GET /i/{path...}", pub.ServeImage)
	mux.Handle("/thumb", thumbHandler)

	if cfg.WebPathPrefix != "" && cfg.WebPathPrefix != "/i" {
		mux.HandleFunc("GET "+cfg.WebPathPrefix+"/{path...}", pub.ServeImage)
	}

	adminGuard := func(next http.HandlerFunc) http.Handler {
		return auth.BasicAuth(cfg.AdminUser, cfg.AdminPass, next)
	}
	mux.Handle("GET /api/admin/manage", adminGuard(adm.ListManage))
	mux.Handle("GET /api/admin/manage/{path...}", adminGuard(adm.ListAlbumManage))
	mux.Handle("POST /api/admin/mkdir", adminGuard(adm.Mkdir))
	mux.Handle("POST /api/admin/upload", adminGuard(adm.Upload))
	mux.Handle("POST /api/admin/rename", adminGuard(adm.Rename))
	mux.Handle("POST /api/admin/delete", adminGuard(adm.Delete))
	mux.Handle("POST /api/admin/setcover", adminGuard(adm.SetCover))
	mux.Handle("POST /api/admin/sethidden", adminGuard(adm.SetHidden))
	mux.Handle("POST /api/admin/reorder", adminGuard(adm.Reorder))

	webHandler := webui.NewHandler(cfg.DefaultLang)
	mux.Handle("/", spaHandler(webHandler))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           withHeaders(mux),
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s, photos=%s, thumbs=%s", cfg.Listen, cfg.PhotosDir, cfg.ThumbsDir)
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

func withHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
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

func spaHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/thumb" {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/photos/") {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/":
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
