package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jacksonthemaster/discordrichpresence"
)

const (
	ColorReset   = "\033[0m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorRed     = "\033[31m"
	PauseIconURL = "https://raw.githubusercontent.com/google/material-design-icons/master/png/av/pause/materialicons/48dp/2x/baseline_pause_black_48dp.png"
)

type Config struct {
	JellyfinURL   string `json:"jellyfin_url"`
	JellyfinToken string `json:"jellyfin_token"`
	TMDBAPIKey    string `json:"tmdb_api_key"`
	OMDBAPIKey    string `json:"omdb_api_key"`
	DiscordAppID  string `json:"discord_app_id"`
	PollInterval  int    `json:"poll_interval"`
	TargetUser    string `json:"target_user"` // set this to the user you want to monitor for jellyfin activity.
	ShowPaused        bool `json:"show_paused"`         // if this is false, it will temporarily kill the IPC connection to discord. if true, it will show when you have a show paused with a CSS overlay of a pause button.
	EpisodeThumbnails bool `json:"episode_thumbnails"` // fetch episode-specific still from TMDB instead of series poster
	FallbackArtwork   bool `json:"fallback_artwork"`    // use Jellyfin's artwork endpoint if TMDB has no poster
}

func logInfo(msg string, detail string) {
	fmt.Printf("%sINFO%s  [%s] %s %s\n", ColorGreen, ColorReset, "jellyfin-rpc", msg, detail)
}
func logWarn(msg string, detail string) {
	fmt.Printf("%sWARN%s  [%s] %s %s\n", ColorYellow, ColorReset, "jellyfin-rpc", msg, detail)
}
func logError(msg string, detail string) {
	fmt.Printf("%sERROR%s [%s] %s %s\n", ColorRed, ColorReset, "jellyfin-rpc", msg, detail)
}

func searchTMDB(apiKey string, query string) (posterURL string, tmdbID int) {
	searchUrl := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?api_key=%s&query=%s", apiKey, url.QueryEscape(query))
	resp, err := http.Get(searchUrl)
	if err != nil || resp.StatusCode != 200 {
		return "", 0
	}
	defer resp.Body.Close()

	var res struct {
		Results []struct {
			ID         int    `json:"id"`
			PosterPath string `json:"poster_path"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	if len(res.Results) > 0 && res.Results[0].PosterPath != "" {
		rawUrl := "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
		posterURL = fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(rawUrl))
		tmdbID = res.Results[0].ID
	}
	return
}

func getTMDBEpisodeStill(apiKey string, tmdbID int, seasonNum, epNum float64) string {
	if tmdbID == 0 {
		return ""
	}
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%.0f/episode/%.0f/images?api_key=%s", tmdbID, seasonNum, epNum, apiKey)
	resp, err := http.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Stills []struct {
			FilePath string `json:"file_path"`
		} `json:"stills"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	if len(res.Stills) > 0 && res.Stills[0].FilePath != "" {
		rawUrl := "https://image.tmdb.org/t/p/w500" + res.Stills[0].FilePath
		return fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(rawUrl))
	}
	return ""
}

func getJellyfinArtwork(jellyfinURL, token, itemID string) string {
	return fmt.Sprintf("%s/Items/%s/Images/Primary?api_key=%s", jellyfinURL, itemID, token)
}

func getRatings(apiKey string, query string, year string) string {
	if apiKey == "" {
		return ""
	}
	apiURL := fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&t=%s", apiKey, url.QueryEscape(query))
	if year != "" && year != "0" {
		apiURL += fmt.Sprintf("&y=%s", year)
	}

	resp, err := http.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Ratings []struct {
			Source string `json:"Source"`
			Value  string `json:"Value"`
		} `json:"Ratings"`
	}
	json.NewDecoder(resp.Body).Decode(&res)

	var imdb, rt string
	for _, r := range res.Ratings {
		switch r.Source {
		case "Internet Movie Database":
			imdb = "⭐ " + r.Value
		case "Rotten Tomatoes":
			rt = "🍅 " + r.Value
		}
	}

	if imdb != "" && rt != "" {
		return fmt.Sprintf("%s  %s", imdb, rt)
	} else if imdb != "" || rt != "" {
		return imdb + rt
	}
	return ""
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

	if cfg.JellyfinURL == "" {
		return Config{}, fmt.Errorf("jellyfin_url is required")
	}
	if cfg.JellyfinToken == "" {
		return Config{}, fmt.Errorf("jellyfin_token is required")
	}
	if cfg.DiscordAppID == "" {
		return Config{}, fmt.Errorf("discord_app_id is required")
	}
	if cfg.TargetUser == "" {
		return Config{}, fmt.Errorf("target_user is required")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 3
	}

	return cfg, nil
}

func connectDiscord(appID string) (*discordrichpresence.Client, error) {
	drpc := discordrichpresence.NewClient(appID)
	err := drpc.Connect()
	return drpc, err
}

func updateActivity(drpc *discordrichpresence.Client, cfg Config, sessions []map[string]interface{}, lastItemID *string, lastPlayState *bool, lastPosTicks *float64) {
	lineOne, lineTwo, searchTitle, currentID, prodYear := "", "", "", "", ""
	var startUnix, endUnix int64
	var posTicks float64
	isPaused := false
	var sNum, eNum float64

	for _, s := range sessions {
		if name, ok := s["UserName"].(string); ok && name == cfg.TargetUser {
			if playState, ok := s["PlayState"].(map[string]interface{}); ok {
				isPaused, _ = playState["IsPaused"].(bool)
				posTicks, _ = playState["PositionTicks"].(float64)

				if item, ok := s["NowPlayingItem"].(map[string]interface{}); ok {
					currentID, _ = item["Id"].(string)
					runTicks, _ := item["RunTimeTicks"].(float64)

					if py, ok := item["ProductionYear"].(float64); ok {
						prodYear = strconv.Itoa(int(py))
					}

					if runTicks > 0 && !isPaused {
						startUnix = time.Now().Unix() - int64(posTicks/10000000)
						endUnix = startUnix + int64(runTicks/10000000)
					}

					itemType, _ := item["Type"].(string)
					switch itemType {
					case "Episode":
						seriesName, _ := item["SeriesName"].(string)
						epName, _ := item["Name"].(string)
						sNum, _ = item["ParentIndexNumber"].(float64)
						eNum, _ = item["IndexNumber"].(float64)
						lineOne = seriesName
						lineTwo = fmt.Sprintf("S%.0f - E%.0f: %s", sNum, eNum, epName)
						searchTitle = seriesName
					case "Audio":
						artist, _ := item["Artists"].([]interface{})
						artistName := ""
						if len(artist) > 0 {
							artistName, _ = artist[0].(string)
						}
						album, _ := item["Album"].(string)
						track, _ := item["Name"].(string)
						lineOne = track
						if artistName != "" && album != "" {
							lineTwo = fmt.Sprintf("%s - %s", artistName, album)
						} else if artistName != "" {
							lineTwo = artistName
						} else {
							lineTwo = "on Jellyfin"
						}
						searchTitle = track
					default:
						lineOne, _ = item["Name"].(string)
						lineTwo = "on Jellyfin"
						searchTitle = lineOne
					}
				}
			}
			break
		}
	}

	diff := posTicks - *lastPosTicks
	skipped := (diff > 50000000 || diff < -50000000) && currentID == *lastItemID && *lastPosTicks != 0

	if currentID != "" {
		if isPaused && !cfg.ShowPaused {
			if *lastPlayState != isPaused {
				drpc.ClearActivity()
				logInfo("Playback paused (Status hidden):", lineOne)
				*lastPlayState = isPaused
			}
		} else if currentID != *lastItemID || isPaused != *lastPlayState || skipped {
			poster, tmdbID := searchTMDB(cfg.TMDBAPIKey, searchTitle)
			if poster == "" && cfg.FallbackArtwork && currentID != "" {
				poster = getJellyfinArtwork(cfg.JellyfinURL, cfg.JellyfinToken, currentID)
			}
			if cfg.EpisodeThumbnails && sNum > 0 && eNum > 0 {
				if still := getTMDBEpisodeStill(cfg.TMDBAPIKey, tmdbID, sNum, eNum); still != "" {
					poster = still
				}
			}
			ratings := getRatings(cfg.OMDBAPIKey, searchTitle, prodYear)

			activity := discordrichpresence.Activity{
				Assets: discordrichpresence.Assets{LargeImage: poster},
				Type:   3,
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
				activity.Timestamps = discordrichpresence.Timestamps{
					Start: startUnix * 1000,
					End:   endUnix * 1000,
				}
				}
				logInfo("Status updated (Playing/Skipped):", fmt.Sprintf("%s - %s", lineOne, lineTwo))
			}

			drpc.SetActivity(activity)
			*lastItemID, *lastPlayState, *lastPosTicks = currentID, isPaused, posTicks
		}
	} else if currentID == "" && *lastItemID != "" {
		drpc.ClearActivity()
		logInfo("Playback stopped", "")
		*lastItemID = ""
		*lastPosTicks = 0
	}

	*lastPosTicks = posTicks
}

func main() {
	cfg, err := loadConfig("config.json")
	if err != nil {
		logError("Config error", err.Error())
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP)

	reload := make(chan Config, 1)
	go func() {
		for range sig {
			newCfg, err := loadConfig("config.json")
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
		logError("Discord failed", err.Error())
		os.Exit(1)
	}
	logInfo("Connected to Discord!", "")

	var lastItemID string
	var lastPlayState bool
	var lastPosTicks float64

	for {
		select {
		case newCfg := <-reload:
			if newCfg.DiscordAppID != cfg.DiscordAppID {
				drpc.Close()
				drpc, err = connectDiscord(newCfg.DiscordAppID)
				if err != nil {
					logError("Discord reconnect failed", err.Error())
					os.Exit(1)
				}
				logInfo("Reconnected to Discord with new App ID", "")
			}
			cfg = newCfg
			logInfo("Config applied", "")
		default:
		}

		req, _ := http.NewRequest("GET", cfg.JellyfinURL+"/Sessions", nil)
		req.Header.Add("X-Emby-Token", cfg.JellyfinToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logWarn("Jellyfin lost, retrying...", "")
			time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
			continue
		}

		var sessions []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&sessions)
		resp.Body.Close()

		updateActivity(drpc, cfg, sessions, &lastItemID, &lastPlayState, &lastPosTicks)
		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
