package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultPollInterval    = 3
	DefaultGenericItemText = "on Jellyfin"
)

var defaultAnimeTags = []string{"anime", "japanese animation", "animation", "manga"}

type Config struct {
	JellyfinURL       string   `json:"jellyfin_url"`
	JellyfinToken     string   `json:"jellyfin_token"`
	TMDBAPIKey        string   `json:"tmdb_api_key"`
	OMDBAPIKey        string   `json:"omdb_api_key"`
	DiscordAppID      string   `json:"discord_app_id"`
	PollInterval      int      `json:"poll_interval"`
	TargetUser        string   `json:"target_user"`
	ShowPaused        bool     `json:"show_paused"`
	EpisodeThumbnails bool     `json:"episode_thumbnails"`
	FallbackArtwork   bool     `json:"fallback_artwork"`
	GenericItemText   string   `json:"generic_item_text"`
	AnimeTags         []string `json:"anime_tags"`
	AnilistEnabled    bool     `json:"anilist_enabled"`
	EnableButtons     bool     `json:"enable_buttons"`
	PublicJellyfinURL string   `json:"public_jellyfin_url"`
	DisableMusic      bool     `json:"disable_music"`
}

func writeConfigTemplate(path string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}
	}

	template := `{
  "jellyfin_url": "http://localhost:8096",
  "jellyfin_token": "",
  "tmdb_api_key": "",
  "omdb_api_key": "",
  "discord_app_id": "",
  "target_user": "",
  "poll_interval": 3,
  "show_paused": false,
  "episode_thumbnails": false,
  "fallback_artwork": false,
  "generic_item_text": "on Jellyfin",
  "anime_tags": ["anime", "japanese animation", "animation", "manga"],
  "anilist_enabled": true,
  "enable_buttons": false,
  "public_jellyfin_url": "",
  "disable_music": false
}
`

	if err := os.WriteFile(path, []byte(template), 0o600); err != nil {
		return fmt.Errorf("writing config template: %w", err)
	}
	return nil
}

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if writeErr := writeConfigTemplate(path); writeErr != nil {
				return Config{}, fmt.Errorf("config file missing and template creation failed: %w", writeErr)
			}
			return Config{}, fmt.Errorf("config file not found at %q; a template was created there. Fill in the required values and restart", path)
		}
		return Config{}, fmt.Errorf("opening config: %w", err)
	}
	defer file.Close()
	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.JellyfinURL == "" || cfg.JellyfinToken == "" || cfg.DiscordAppID == "" || cfg.TargetUser == "" {
		return Config{}, fmt.Errorf("missing required configuration fields; check %q for jellyfin_url, jellyfin_token, discord_app_id, and target_user", path)
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.GenericItemText == "" {
		cfg.GenericItemText = DefaultGenericItemText
	}
	if len(cfg.AnimeTags) == 0 {
		cfg.AnimeTags = append([]string(nil), defaultAnimeTags...)
	}
	return cfg, nil
}
