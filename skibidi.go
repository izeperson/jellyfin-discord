package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
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
	DiscordAppID  string `json:"discord_app_id"`
	PollInterval  int    `json:"poll_interval"`
	TargetUser    string `json:"target_user"`
	ShowPaused    bool   `json:"show_paused"`
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

func getTMDBPoster(apiKey string, query string) string {
	searchUrl := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?api_key=%s&query=%s", apiKey, url.QueryEscape(query))
	resp, err := http.Get(searchUrl)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Results []struct {
			PosterPath string `json:"poster_path"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	if len(res.Results) > 0 && res.Results[0].PosterPath != "" {
		rawUrl := "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
		return fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=t", url.QueryEscape(rawUrl))
	}
	return ""
}

func main() {
	file, err := os.Open("config.json")
	if err != nil {
		logError("Failed to load config.json", "")
		os.Exit(1)
	}
	var cfg Config
	json.NewDecoder(file).Decode(&cfg)
	file.Close()

	logInfo("Connecting to Discord...", "")
	drpc := discordrichpresence.NewClient(cfg.DiscordAppID)
	err = drpc.Connect()
	if err != nil {
		logError("Discord failed", err.Error())
		os.Exit(1)
	}
	logInfo("Connected!", "")

	var lastItemID string
	var lastPlayState bool

	for {
		req, _ := http.NewRequest("GET", cfg.JellyfinURL+"/Sessions", nil)
		req.Header.Add("X-Emby-Token", cfg.JellyfinToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logWarn("Jellyfin lost", "")
			time.Sleep(10 * time.Second)
			continue
		}

		var sessions []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&sessions)
		resp.Body.Close()

		lineOne, lineTwo, searchTitle, currentID := "", "", "", ""
		var startUnix, endUnix int64
		isPaused := false

		for _, s := range sessions {
			if name, ok := s["UserName"].(string); ok && name == cfg.TargetUser {
				if playState, ok := s["PlayState"].(map[string]interface{}); ok {
					isPaused, _ = playState["IsPaused"].(bool)
					posTicks, _ := playState["PositionTicks"].(float64)

					if item, ok := s["NowPlayingItem"].(map[string]interface{}); ok {
						currentID, _ = item["Id"].(string)
						runTicks, _ := item["RunTimeTicks"].(float64)

						if runTicks > 0 && !isPaused {
							startUnix = time.Now().Unix() - int64(posTicks/10000000)
							endUnix = startUnix + int64(runTicks/10000000)
						}

						itemType, _ := item["Type"].(string)
						if itemType == "Episode" {
							seriesName, _ := item["SeriesName"].(string)
							epName, _ := item["Name"].(string)
							sNum, _ := item["ParentIndexNumber"].(float64)
							eNum, _ := item["IndexNumber"].(float64)
							lineOne = seriesName
							lineTwo = fmt.Sprintf("S%.0f - E%.0f: %s", sNum, eNum, epName)
							searchTitle = seriesName
						} else {
							lineOne, _ = item["Name"].(string)
							lineTwo = "on Jellyfin"
							searchTitle = lineOne
						}
					}
				}
				break
			}
		}

		if currentID != "" {
			if isPaused && !cfg.ShowPaused {
				if lastPlayState != isPaused {
					drpc.ClearActivity()
					logInfo("Playback paused (Status hidden):", lineOne)
					lastPlayState = isPaused
				}
			} else if currentID != lastItemID || isPaused != lastPlayState {
				poster := getTMDBPoster(cfg.TMDBAPIKey, searchTitle)
				activity := discordrichpresence.Activity{
					Assets: discordrichpresence.Assets{LargeImage: poster},
					Type:   3,
				}

				if isPaused {
					activity.Details = lineOne
					activity.State = "Paused"
					activity.Assets.LargeText = lineOne
					activity.Assets.SmallImage = "https://images.weserv.nl/?url=" + url.QueryEscape(PauseIconURL) + "&w=64&h=64&inv"
					logInfo("Status updated (Paused):", lineOne)
				} else {
					activity.Details = lineOne
					activity.State = lineTwo
					activity.Assets.LargeText = lineOne
					activity.Timestamps = discordrichpresence.Timestamps{
						Start: startUnix * 1000,
						End:   endUnix * 1000,
					}
					logInfo("Status updated (Playing):", fmt.Sprintf("%s - %s", lineOne, lineTwo))
				}

				drpc.SetActivity(activity)
				lastItemID, lastPlayState = currentID, isPaused
			}
		} else if currentID == "" && lastItemID != "" {
			drpc.ClearActivity()
			logInfo("Playback stopped", "")
			lastItemID = ""
		}
		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
