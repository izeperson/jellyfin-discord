package main

import (
	"encoding/json"
	"fmt"
	"os"
)

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
}

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("opening config: %w", err)
	}
	defer file.Close()
	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.JellyfinURL == "" || cfg.JellyfinToken == "" || cfg.DiscordAppID == "" || cfg.TargetUser == "" {
		return Config{}, fmt.Errorf("missing required configuration fields")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 3
	}
	if cfg.GenericItemText == "" {
		cfg.GenericItemText = "on Jellyfin"
	}
	if len(cfg.AnimeTags) == 0 {
		cfg.AnimeTags = []string{"anime", "japanese animation", "animation", "manga"}
	}
	return cfg, nil
}
