package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"
)

const (
	ColorReset           = "\033[0m"
	ColorGreen           = "\033[32m"
	ColorYellow          = "\033[33m"
	ColorRed             = "\033[31m"
	PauseIconURL         = "https://raw.githubusercontent.com/google/material-design-icons/master/png/av/pause/materialicons/48dp/2x/baseline_pause_black_48dp.png"
	TicksPerSecond       = 10000000
	SeekThresholdSeconds = 5
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// Cache for TMDB search results (posterURL, tmdbID)
var tmdbSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		PosterURL string
		TMDBID    int
	}
}{m: make(map[string]struct {
	PosterURL string
	TMDBID    int
})}

// Cache for TMDB episode stills
var tmdbEpisodeStillCache = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

// Cache for AniList search results (posterURL, score)
var anilistSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		PosterURL string
		Score     string
	}
}{m: make(map[string]struct {
	PosterURL string
	Score     string
})}

// Cache for OMDb ratings
var omdbRatingsCache = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

func updateActivity(drpc *DiscordRPC, cfg Config, sessions []JellyfinSession, lastItemID *string, lastPlayState *bool, lastPosTicks *float64) {
	var lineOne, lineTwo, searchTitle, currentID, prodYear string
	var posTicks, runTimeTicks float64
	isPaused := false
	var sNum, eNum float64
	var isAudio bool
	var targetItem *JellyfinSession

	if drpc == nil {
		return
	}

	for _, item := range sessions {
		if item.UserName == cfg.TargetUser {
			targetItem = &item
			isPaused = item.PlayState.IsPaused
			posTicks = item.PlayState.PositionTicks
			currentID = item.NowPlayingItem.Id
			runTimeTicks = item.NowPlayingItem.RunTimeTicks
			isAudio = item.NowPlayingItem.Type == "Audio"
			lineOne, lineTwo, searchTitle, prodYear, sNum, eNum = getMediaDetails(item, cfg.GenericItemText)
			break
		}
	}

	diff := posTicks - *lastPosTicks
	skipped := (diff > TicksPerSecond*SeekThresholdSeconds || diff < -TicksPerSecond*SeekThresholdSeconds) && currentID == *lastItemID && *lastPosTicks != 0
	var startUnix, endUnix int64
	if currentID != "" && !isPaused && runTimeTicks > 0 {
		startUnix = time.Now().Unix() - int64(posTicks/TicksPerSecond)
		endUnix = startUnix + int64(runTimeTicks/TicksPerSecond)
	}

	if currentID != "" {
		if isPaused && !cfg.ShowPaused {
			if *lastPlayState != isPaused {
				if err := drpc.ClearActivity(); err == nil {
					*lastItemID = ""
					*lastPosTicks = 0
					*lastPlayState = isPaused
					logInfo("Playback paused (Status hidden):", lineOne)
				} else {
					logWarn("Failed to clear Discord activity (paused/hidden):", err.Error())
				}
			}
		} else if currentID != *lastItemID || isPaused != *lastPlayState || skipped {
			var poster string
			var tmdbID int

			// Determine if the item is anime based on Jellyfin tags
			isAnime := isItemAnime(*targetItem, cfg)

			if isAudio { // Audio items get artwork directly from Jellyfin
				poster = getJellyfinArtwork(cfg.JellyfinURL, cfg.JellyfinToken, currentID)
			} else { // Video items
				// Prioritize AniList if enabled and item is detected as anime
				if cfg.AnilistEnabled && isAnime {
					poster, _ = searchAniList(searchTitle)
				}

				// Fallback to TMDB if AniList is not enabled/didn't find anything, or if not anime
				if poster == "" || !cfg.AnilistEnabled || !isAnime {
					if cfg.TMDBAPIKey == "" {
						logWarn("Image issue", "TMDB API Key is missing in config.json. Images will fail to load.")
					}
					poster, tmdbID = searchTMDB(cfg.TMDBAPIKey, searchTitle)
				}
			}
			if !isAudio && cfg.EpisodeThumbnails && sNum > 0 && eNum > 0 && tmdbID != 0 {
				if still := getTMDBEpisodeStill(cfg.TMDBAPIKey, tmdbID, sNum, eNum); still != "" { // Only try if tmdbID is valid
					poster = still
				} else {
					logInfo("No TMDB episode still found for", fmt.Sprintf("%s S%.0fE%.0f", searchTitle, sNum, eNum))
				}

			}
			ratings := getRatings(cfg.OMDBAPIKey, searchTitle, prodYear)

			activity := Activity{
				Assets: Assets{LargeImage: poster},
				Type:   3,
			}

			if isAudio {
				activity.Type = 2
			}

			if isPaused {
				activity.Details = lineOne
				if ratings != "" {
					activity.State = "Paused | " + ratings
				} else {
					activity.State = "Paused"
				}
				activity.Assets.LargeText = lineOne
				activity.Assets.SmallImage = "https://images.weserv.nl/?url=" + url.QueryEscape(PauseIconURL) + "&w=64&h=64&inv"
				logInfo("Status updated (Paused):", lineOne)
			} else {
				activity.Details = lineOne
				if ratings != "" {
					activity.State = fmt.Sprintf("%s | %s", lineTwo, ratings)
				} else {
					activity.State = lineTwo
				}
				activity.Assets.LargeText = lineOne
				if startUnix > 0 && endUnix > 0 {
					activity.Timestamps = Timestamps{
						Start: startUnix,
						End:   endUnix,
					}
				}
				logInfo("Status updated (Playing/Skipped):", fmt.Sprintf("%s - %s", lineOne, lineTwo))
			}

			if err := drpc.SetActivity(&activity); err == nil {
				*lastItemID, *lastPlayState, *lastPosTicks = currentID, isPaused, posTicks
			} else {
				logWarn("Failed to update Discord activity:", err.Error())
			}
		}
	} else if currentID == "" && *lastItemID != "" {
		if err := drpc.ClearActivity(); err == nil {
			logInfo("Playback stopped", "")
			*lastItemID = ""
			*lastPosTicks = 0
		} else {
			logWarn("Failed to clear Discord activity (stopped):", err.Error())
		}
	} else if currentID == "" && *lastItemID == "" {
		*lastPosTicks = 0
	}
}

func main() {
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		logError("Config error", err.Error())
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)

	// Only notify on SIGHUP if the platform supports it
	if reloadSig := getReloadSignal(); reloadSig != nil {
		signal.Notify(sig, reloadSig)
	}

	reload := make(chan Config, 1)
	go func() {
		for range sig {
			newCfg, err := loadConfig(*configPath)
			if err != nil {
				logWarn("Reload failed", err.Error())
				continue
			}
			logInfo("Config reloaded", "")
			reload <- newCfg
		}
	}()

	drpc, err := connectDiscord(cfg.DiscordAppID)
	if err != nil {
		logWarn("Discord not found on startup:", err.Error()+". Will retry in background.")
	} else {
		logInfo("Connected to Discord!", "")
	}

	var lastItemID string
	var lastPlayState bool
	var lastPosTicks float64

	for {
		select {
		case newCfg := <-reload:
			if newCfg.DiscordAppID != cfg.DiscordAppID {
				if drpc != nil {
					drpc.Close()
				}
				drpc, err = connectDiscord(newCfg.DiscordAppID)
				if err != nil {
					logWarn("Discord reconnect failed:", err.Error())
					drpc = nil
				} else {
					logInfo("Reconnected to Discord with new App ID", "")
				}
			}
			cfg = newCfg
			logInfo("Config applied", "")
		default:
		}

		if drpc == nil {
			drpc, err = connectDiscord(cfg.DiscordAppID)
			if err == nil {
				logInfo("Late connection to Discord established", "")
				// Reset state to force an update on next poll
				lastItemID = ""
				lastPosTicks = 0
			}
		}

		req, err := http.NewRequest("GET", cfg.JellyfinURL+"/Sessions", nil)
		if err != nil {
			logError("Failed to create HTTP request for sessions", err.Error())
			time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
			continue
		}
		req.Header.Add("X-Emby-Token", cfg.JellyfinToken)
		resp, err := httpClient.Do(req)
		if err != nil {
			logWarn("Jellyfin lost, retrying...", err.Error())
			time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			logWarn("Jellyfin sessions request failed with status", resp.Status)
			time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
			continue
		}

		var sessions []JellyfinSession
		decodeErr := json.NewDecoder(resp.Body).Decode(&sessions)
		resp.Body.Close()

		if decodeErr != nil {
			logWarn("Failed to decode Jellyfin sessions response", decodeErr.Error())
			time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
			continue
		}

		updateActivity(drpc, cfg, sessions, &lastItemID, &lastPlayState, &lastPosTicks)
		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
