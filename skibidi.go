package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/hugolgst/rich-go/client"
)

const (
	ColorReset  = "\033[0m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorRed    = "\033[31m"
	ColorCyan   = "\033[36m"
)

type Config struct {
	JellyfinURL   string `json:"jellyfin_url"`
	JellyfinToken string `json:"jellyfin_token"`
	TMDBAPIKey    string `json:"tmdb_api_key"`
	DiscordAppID  string `json:"discord_app_id"`
	PollInterval  int    `json:"poll_interval"`
	TargetUser    string `json:"target_user"`
}

func logInfo(msg string, detail string) {
	fmt.Printf("%sINFO%s  [%s] %s %s\n", ColorGreen, ColorReset, "jelly-rpc", msg, detail)
}

func logWarn(msg string, detail string) {
	fmt.Printf("%sWARN%s  [%s] %s %s\n", ColorYellow, ColorReset, "jelly-rpc", msg, detail)
}

func logError(msg string, detail string) {
	fmt.Printf("%sERROR%s [%s] %s %s\n", ColorRed, ColorReset, "jelly-rpc", msg, detail)
}

func checkJellyfinConnection(cfg Config) error {
	req, _ := http.NewRequest("GET", cfg.JellyfinURL+"/System/Info", nil)
	req.Header.Add("X-Emby-Token", cfg.JellyfinToken)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func getTMDBPoster(apiKey string, query string) string {
	url := fmt.Sprintf("https://api.themoviedb.org/3/search/movie?api_key=%s&query=%s", apiKey, query)
	resp, err := http.Get(url)
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
		return "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
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

	logInfo("Connecting to Discord", "")
	err = client.Login(cfg.DiscordAppID)
	if err != nil {
		logError("Failed to connect to arRPC", err.Error())
		os.Exit(1)
	}
	logInfo("Connected!", "")

	err = checkJellyfinConnection(cfg)
	if err != nil {
		logError("Failed to connect to Jellyfin", cfg.JellyfinURL)
		os.Exit(1)
	}
	logInfo("Successfully connected to Jellyfin", fmt.Sprintf("(User: %s)", cfg.TargetUser))

	var lastMovie string
	var sessionStartTime *time.Time

	for {
		req, _ := http.NewRequest("GET", cfg.JellyfinURL+"/Sessions", nil)
		req.Header.Add("X-Emby-Token", cfg.JellyfinToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logWarn("Jellyfin connection lost, retrying...", "")
			time.Sleep(10 * time.Second)
			continue
		}

		var sessions []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&sessions)
		resp.Body.Close()

		currentMovie := ""
		for _, s := range sessions {
			if name, ok := s["UserName"].(string); ok && name == cfg.TargetUser {
				if item, ok := s["NowPlayingItem"].(map[string]interface{}); ok {
					currentMovie = item["Name"].(string)
					break
				}
			}
		}

		if currentMovie != "" && currentMovie != lastMovie {
			poster := getTMDBPoster(cfg.TMDBAPIKey, currentMovie)
			now := time.Now()
			sessionStartTime = &now

			err := client.SetActivity(client.Activity{
				Details:    "Watching " + currentMovie,
				State:      "On Jellyfin",
				LargeImage: poster,
				LargeText:  currentMovie,
				Timestamps: &client.Timestamps{Start: sessionStartTime},
			})

			if err != nil {
				logError("Discord update failed", err.Error())
			} else {
				logInfo("Status updated:", currentMovie)
				lastMovie = currentMovie
			}
		} else if currentMovie == "" && lastMovie != "" {
			client.SetActivity(client.Activity{})
			logInfo("Playback stopped, cleared status", "")
			lastMovie = ""
			sessionStartTime = nil
		}

		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
