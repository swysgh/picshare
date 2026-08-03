package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Listen            string `json:"listen"`
	PhotosDir         string `json:"photos_dir"`
	ThumbsDir         string `json:"thumbs_dir"`
	AdminUser         string `json:"admin_user"`
	AdminPass         string `json:"admin_pass"`
	SiteTitle         string `json:"site_title"`
	DefaultLang       string `json:"default_lang"`
	GridThumbSize     int    `json:"grid_thumb_size"`
	LightboxSize      int    `json:"lightbox_size"`
	EnableExifRotate  bool   `json:"enable_exif_rotation"`
	WebPathPrefix     string `json:"web_path_prefix"`
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
	if cfg.GridThumbSize <= 0 {
		cfg.GridThumbSize = 480
	}
	if cfg.LightboxSize <= 0 {
		cfg.LightboxSize = 1600
	}
	if cfg.DefaultLang == "" {
		cfg.DefaultLang = "zh"
	}
	if cfg.SiteTitle == "" {
		cfg.SiteTitle = "Product Gallery"
	}
	if cfg.WebPathPrefix == "" {
		cfg.WebPathPrefix = "/photos"
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
