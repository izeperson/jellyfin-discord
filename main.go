package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	DiscordAppID  int `json:"discord_app_id"`
	PollInterval  int    `json:"poll_interval"`
	TargetUser    string `json:"target_user"` // set this to the user you want to monitor for jellyfin activity.
	ShowPaused    bool   `json:"show_paused"` // if this is false, it will temporarily kill the IPC connection to discord. if true, it will show when you have a show paused with a CSS overlay of a pause button.
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
		return fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(rawUrl))
	}
	return ""
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
	var lastPosTicks float64

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

		lineOne, lineTwo, searchTitle, currentID, prodYear := "", "", "", "", "" // set empty so that it can be filled by info taken from jellyfin.
		var startUnix, endUnix int64
		var posTicks float64
		isPaused := false

		for _, s := range sessions {
			if name, ok := s["UserName"].(string); ok && name == cfg.TargetUser {
				if playState, ok := s["PlayState"].(map[string]interface{}); ok {
					isPaused, _ = playState["IsPaused"].(bool)
					posTicks, _ = playState["PositionTicks"].(float64)

					if item, ok := s["NowPlayingItem"].(map[string]interface{}); ok {
						currentID, _ = item["Id"].(string)
						runTicks, _ := item["RunTimeTicks"].(float64)

						// Get Media Production Year
						if py, ok := item["ProductionYear"].(float64); ok {
							prodYear = strconv.Itoa(int(py))
						}

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

		diff := posTicks - lastPosTicks
		skipped := (diff > 50000000 || diff < -50000000) && currentID == lastItemID && lastPosTicks != 0

		if currentID != "" {
			if isPaused && !cfg.ShowPaused {
				if lastPlayState != isPaused {
					drpc.ClearActivity()
					logInfo("Playback paused (Status hidden):", lineOne) // this happens if you set "show_paused" in the config.json to false.
					lastPlayState = isPaused
				}
			} else if currentID != lastItemID || isPaused != lastPlayState || skipped {
				poster := getTMDBPoster(cfg.TMDBAPIKey, searchTitle)
				ratings := getRatings(cfg.OMDBAPIKey, searchTitle, prodYear)

				activity := discordrichpresence.Activity{
					Assets: discordrichpresence.Assets{LargeImage: poster},
					Type:   3,
				}

				if isPaused {
					activity.Details = lineOne
					if ratings != "" {
						activity.State = "Paused | " + ratings // this will show if you have it set to true.
					} else {
						activity.State = "Paused"
					}
					activity.Assets.LargeText = lineOne
					activity.Assets.SmallImage = "https://images.weserv.nl/?url=" + url.QueryEscape(PauseIconURL) + "&w=64&h=64&inv" // display paused icon as mentioned on line 31.
					logInfo("Status updated (Paused):", lineOne)
				} else {
					activity.Details = lineOne
					if ratings != "" {
						activity.State = fmt.Sprintf("%s | %s", lineTwo, ratings)
					} else {
						activity.State = lineTwo
					}
					activity.Assets.LargeText = lineOne
					activity.Timestamps = discordrichpresence.Timestamps{
						Start: startUnix * 1000,
						End:   endUnix * 1000,
					}
					logInfo("Status updated (Playing/Skipped):", fmt.Sprintf("%s - %s", lineOne, lineTwo))
				}

				drpc.SetActivity(activity)
				lastItemID, lastPlayState, lastPosTicks = currentID, isPaused, posTicks
			}
		} else if currentID == "" && lastItemID != "" { // checking to see if user has paused their content.
			drpc.ClearActivity()
			logInfo("Playback stopped", "")
			lastItemID = ""
			lastPosTicks = 0
		}

		lastPosTicks = posTicks
		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
