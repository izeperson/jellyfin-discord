package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
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

type Config struct {
	JellyfinURL       string `json:"jellyfin_url"`
	JellyfinToken     string `json:"jellyfin_token"`
	TMDBAPIKey        string `json:"tmdb_api_key"`
	OMDBAPIKey        string `json:"omdb_api_key"`
	DiscordAppID      string `json:"discord_app_id"`
	PollInterval      int    `json:"poll_interval"`
	TargetUser        string `json:"target_user"`
	ShowPaused        bool   `json:"show_paused"`
	EpisodeThumbnails bool   `json:"episode_thumbnails"`
	FallbackArtwork   bool   `json:"fallback_artwork"`
	GenericItemText   string `json:"generic_item_text"`
}

type JellyfinSession struct {
	UserName  string `json:"UserName"`
	PlayState struct {
		IsPaused      bool    `json:"IsPaused"`
		PositionTicks float64 `json:"PositionTicks"`
	} `json:"PlayState"`
	NowPlayingItem struct {
		Id                string   `json:"Id"`
		RunTimeTicks      float64  `json:"RunTimeTicks"`
		ProductionYear    float64  `json:"ProductionYear"`
		Type              string   `json:"Type"`
		Name              string   `json:"Name"`
		SeriesName        string   `json:"SeriesName"`
		ParentIndexNumber float64  `json:"ParentIndexNumber"`
		IndexNumber       float64  `json:"IndexNumber"`
		Artists           []string `json:"Artists"`
		Album             string   `json:"Album"`
	} `json:"NowPlayingItem"`
}

type DiscordRPC struct {
	conn  net.Conn
	appID string
}

type Activity struct {
	Details    string     `json:"details,omitempty"`
	State      string     `json:"state,omitempty"`
	Assets     Assets     `json:"assets,omitempty"`
	Timestamps Timestamps `json:"timestamps,omitempty"`
	Type       int        `json:"type,omitempty"`
}

type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type Timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

func (d *DiscordRPC) send(opcode int, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, int32(opcode))
	binary.Write(buf, binary.LittleEndian, int32(len(data)))
	buf.Write(data)
	_, err = d.conn.Write(buf.Bytes())
	return err
}

func (d *DiscordRPC) Connect() error {
	var dirs []string
	if runtime.GOOS != "windows" {
		xdg := os.Getenv("XDG_RUNTIME_DIR")
		if xdg == "" {
			xdg = fmt.Sprintf("/run/user/%d", os.Getuid())
		}
		if xdg != "" {
			dirs = append(dirs, xdg)
			dirs = append(dirs, xdg+"/app/com.discordapp.Discord")
			dirs = append(dirs, xdg+"/app/com.discordapp.DiscordCanary")
			dirs = append(dirs, xdg+"/app/com.discordapp.DiscordDevelopment")
			dirs = append(dirs, xdg+"/app/dev.vencord.Vesktop")
			dirs = append(dirs, xdg+"/app/io.github.equibop.Equibop")
			dirs = append(dirs, xdg+"/app/io.github.equibop.EquibopCanary")
			dirs = append(dirs, xdg+"/.flatpak/com.discordapp.Discord/xdg-run")
			dirs = append(dirs, xdg+"/.flatpak/com.discordapp.DiscordCanary/xdg-run")
			dirs = append(dirs, xdg+"/.flatpak/io.github.equibop.Equibop/xdg-run")
			dirs = append(dirs, xdg+"/snap.discord")
			dirs = append(dirs, xdg+"/discord")
		}
		for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
			if v := os.Getenv(env); v != "" {
				dirs = append(dirs, v)
			}
		}
		dirs = append(dirs, "/tmp")
	} else {
		dirs = append(dirs, `\\.\pipe`)
	}

	var conn net.Conn
	var err error
	for _, dir := range dirs {
		for i := 0; i < 10; i++ {
			var path string
			if runtime.GOOS == "windows" {
				path = fmt.Sprintf(`%s\discord-ipc-%d`, dir, i)
				conn, err = net.DialTimeout("npipe", path, 2*time.Second)
				if err != nil {
					conn, err = net.Dial("shared", path)
				}
			} else {
				path = fmt.Sprintf("%s/discord-ipc-%d", dir, i)
				conn, err = net.DialTimeout("unix", path, 2*time.Second)
			}

			if err == nil {
				d.conn = conn
				if err := d.send(0, map[string]string{"v": "1", "client_id": d.appID}); err != nil {
					conn.Close()
					continue
				}
				logInfo("Connected to Discord via", path)
				return nil
			}
		}
	}
	return fmt.Errorf("no active socket found: %w", err)
}

func (d *DiscordRPC) SetActivity(a *Activity) error {
	payload := map[string]interface{}{
		"cmd": "SET_ACTIVITY",
		"args": map[string]interface{}{
			"pid":      os.Getpid(),
			"activity": a,
		},
		"nonce": fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	return d.send(1, payload)
}

func (d *DiscordRPC) ClearActivity() error {
	return d.SetActivity(nil)
}

func (d *DiscordRPC) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
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
	if apiKey == "" {
		return "", 0
	}
	searchUrl := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?api_key=%s&query=%s", apiKey, url.QueryEscape(query))
	resp, err := httpClient.Get(searchUrl)
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
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("Failed to decode TMDB search response", err.Error())
	}
	if len(res.Results) > 0 && res.Results[0].PosterPath != "" {
		rawUrl := "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
		posterURL = fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawUrl))
		tmdbID = res.Results[0].ID
	}
	return
}

func getTMDBEpisodeStill(apiKey string, tmdbID int, seasonNum, epNum float64) string {
	if tmdbID == 0 {
		return ""
	}
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%.0f/episode/%.0f/images?api_key=%s", tmdbID, seasonNum, epNum, apiKey)
	resp, err := httpClient.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Stills []struct {
			FilePath string `json:"file_path"`
		} `json:"stills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("Failed to decode TMDB episode still response", err.Error())
	}
	if len(res.Stills) > 0 && res.Stills[0].FilePath != "" {
		rawUrl := "https://image.tmdb.org/t/p/w500" + res.Stills[0].FilePath
		return fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawUrl))
	}
	return ""
}
func getJellyfinArtwork(jellyfinURL, token, itemID string) string {
	fullUrl := fmt.Sprintf("%s/Items/%s/Images/Primary?api_key=%s", jellyfinURL, itemID, token)

	if strings.Contains(jellyfinURL, "10.") || strings.Contains(jellyfinURL, "192.168.") || strings.Contains(jellyfinURL, "127.0.0.1") || strings.Contains(jellyfinURL, "localhost") {
		logWarn("Image fallback", "Jellyfin is on a local IP. Images might not show in Discord without a public URL.")
		return fullUrl
	}

	return fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(fullUrl))
}

func getMediaDetails(item JellyfinSession, genericText string) (lineOne, lineTwo, searchTitle, prodYear string, sNum, eNum float64) {
	if item.NowPlayingItem.ProductionYear > 0 {
		prodYear = fmt.Sprintf("%.0f", item.NowPlayingItem.ProductionYear)
	}

	switch item.NowPlayingItem.Type {
	case "Episode":
		seriesName := item.NowPlayingItem.SeriesName
		epName := item.NowPlayingItem.Name
		sNum = item.NowPlayingItem.ParentIndexNumber
		eNum = item.NowPlayingItem.IndexNumber
		lineOne = seriesName
		lineTwo = fmt.Sprintf("S%.0f - E%.0f: %s", sNum, eNum, epName)
		searchTitle = seriesName
	case "Audio":
		artistName := ""
		if len(item.NowPlayingItem.Artists) > 0 {
			artistName = item.NowPlayingItem.Artists[0]
		}
		album := item.NowPlayingItem.Album
		track := item.NowPlayingItem.Name
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
		lineOne = item.NowPlayingItem.Name
		lineTwo = genericText
		searchTitle = lineOne
	}
	return
}

func getRatings(apiKey string, query string, year string) string {
	if apiKey == "" {
		return ""
	}
	apiURL := fmt.Sprintf("https://www.omdbapi.com/?apikey=%s&t=%s", apiKey, url.QueryEscape(query))
	if year != "" && year != "0" {
		apiURL += fmt.Sprintf("&y=%s", year)
	}

	resp, err := httpClient.Get(apiURL)
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
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("Failed to decode OMDb ratings response", err.Error())
	}

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
	if cfg.GenericItemText == "" {
		cfg.GenericItemText = "on Jellyfin"
	}

	return cfg, nil
}

func connectDiscord(appID string) (*DiscordRPC, error) {
	drpc := &DiscordRPC{appID: appID}
	err := drpc.Connect()
	return drpc, err
}

func updateActivity(drpc *DiscordRPC, cfg Config, sessions []JellyfinSession, lastItemID *string, lastPlayState *bool, lastPosTicks *float64) {
	var lineOne, lineTwo, searchTitle, currentID, prodYear string
	var posTicks, runTimeTicks float64
	isPaused := false
	var sNum, eNum float64
	var isAudio bool

	for _, item := range sessions {
		if item.UserName == cfg.TargetUser {
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

			if isAudio {
				poster = getJellyfinArtwork(cfg.JellyfinURL, cfg.JellyfinToken, currentID)
			} else {
				if cfg.TMDBAPIKey == "" {
					logWarn("Image issue", "TMDB API Key is missing in config.json. Images will fail to load.")
				}
				poster, tmdbID = searchTMDB(cfg.TMDBAPIKey, searchTitle)
			}

			if !isAudio && cfg.EpisodeThumbnails && sNum > 0 && eNum > 0 {
				if still := getTMDBEpisodeStill(cfg.TMDBAPIKey, tmdbID, sNum, eNum); still != "" {
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
