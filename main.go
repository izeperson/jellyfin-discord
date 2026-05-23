package main

import (
	"context"
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
	PlaceholderImageURL  = ""
	TicksPerSecond       = 10000000
	SeekThresholdSeconds = 5
	ClientName           = "Jellyfin-Discord-RPC"
	ClientVersion        = "1.1.0"
	DeviceName           = "Go-Backend"
	CacheTTL             = 24 * time.Hour
)

var httpClient = &http.Client{
	Timeout: 3 * time.Second,
}

var tmdbSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		PosterURL string
		TMDBID    int
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	PosterURL string
	TMDBID    int
	Timestamp time.Time
})}

var tmdbEpisodeStillCache = struct {
	sync.RWMutex
	m map[string]struct {
		Value     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	Value     string
	Timestamp time.Time
})}

var anilistSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		PosterURL string
		Score     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	PosterURL string
	Score     string
	Timestamp time.Time
})}

var omdbRatingsCache = struct {
	sync.RWMutex
	m map[string]struct {
		Value     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	Value     string
	Timestamp time.Time
})}

var jellyfinArtworkCache = struct {
	sync.RWMutex
	m map[string]struct {
		Value     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	Value     string
	Timestamp time.Time
})}

func updateActivity(drpc *DiscordRPC, cfg Config, sessions []JellyfinSession, lastItemID *string, lastPlayState *bool, lastPosTicks *float64, lastUpdateTime *time.Time, lastPoster *string, lastRatings *string, lastTMDBID *int, lastBuffering *bool) {
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

	isBuffering := isPaused && posTicks < TicksPerSecond*5
	now := time.Now()
	elapsedTicks := float64(now.Sub(*lastUpdateTime).Seconds()) * TicksPerSecond
	expectedPosTicks := *lastPosTicks
	if !*lastPlayState && *lastItemID != "" {
		expectedPosTicks += elapsedTicks
	}
	posDiff := posTicks - expectedPosTicks
	skipped := (posDiff > TicksPerSecond*SeekThresholdSeconds || posDiff < -TicksPerSecond*SeekThresholdSeconds) && currentID == *lastItemID && *lastPosTicks != 0

	var startUnix, endUnix int64
	if currentID != "" && !isPaused && runTimeTicks > 0 {
		startUnix = time.Now().Unix() - int64(posTicks/TicksPerSecond)
		endUnix = startUnix + int64(runTimeTicks/TicksPerSecond)
	}

	if currentID != "" {
		if isPaused && !cfg.ShowPaused && !isBuffering {
			if *lastPlayState != isPaused || *lastBuffering != isBuffering {
				if err := drpc.ClearActivity(); err == nil {
					*lastItemID = currentID
					*lastPosTicks = 0
					*lastBuffering = isBuffering
					*lastUpdateTime = now
					*lastPlayState = isPaused
					logInfo("Playback paused (Status hidden):", lineOne)
				} else {
					logWarn("Failed to clear Discord activity (paused/hidden):", err.Error())
				}
			}
		} else if currentID != *lastItemID || isPaused != *lastPlayState || skipped || isBuffering != *lastBuffering {
			if currentID != *lastItemID {
				var poster string
				var tmdbID int
				var ratings string
				isAnime := isItemAnime(*targetItem, cfg)

				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				if isAudio {
					artistName := ""
					if len(targetItem.NowPlayingItem.Artists) > 0 {
						artistName = targetItem.NowPlayingItem.Artists[0]
					}
					albumName := targetItem.NowPlayingItem.Album

					type searchReq struct {
						fn    func(context.Context, string, string) string
						query string
					}
					var requests []searchReq
					seen := make(map[string]bool)
					addReq := func(f func(context.Context, string, string) string, q string, prefix string) {
						key := prefix + q
						if q != "" && !seen[key] {
							requests = append(requests, searchReq{f, q})
							seen[key] = true
						}
					}

					if artistName != "" && albumName != "" {
						addReq(searchiTunes, artistName+" "+albumName, "itunes:")
					}
					addReq(searchiTunes, searchTitle, "itunes:")
					if albumName != "" {
						addReq(searchiTunes, albumName, "itunes:")
					}
					addReq(searchiTunes, lineOne, "itunes:")

					if artistName != "" && albumName != "" {
						addReq(searchMusicBrainz, artistName+" "+albumName, "mb:")
					}
					addReq(searchMusicBrainz, searchTitle, "mb:")
					if albumName != "" {
						addReq(searchMusicBrainz, albumName, "mb:")
					}
					addReq(searchMusicBrainz, lineOne, "mb:")

					type res struct {
						idx int
						url string
					}
					c := make(chan res, len(requests))
					for i, req := range requests {
						go func(idx int, r searchReq) {
							c <- res{idx, r.fn(ctx, r.query, artistName)}
						}(i, req)
					}

					results := make([]string, len(requests))

					for finished := 0; finished < len(requests); {
						select {
						case r := <-c:
							results[r.idx] = r.url
							finished++
							if r.idx == 0 && r.url != "" {
								cancel()
								goto audioFound
							}
						case <-ctx.Done():
							goto audioFound
						}
					}
				audioFound:
					for _, r := range results {
						if r != "" {
							poster = r
							break
						}
					}
				} else {
					type videoResult struct {
						idx    int
						poster string
						id     int
					}
					resultsChan := make(chan videoResult, 3)
					var wg sync.WaitGroup

					if cfg.AnilistEnabled && isAnime {
						wg.Add(1)
						go func() {
							defer wg.Done()
							p, _ := searchAniList(ctx, searchTitle)
							resultsChan <- videoResult{idx: 0, poster: p}
						}()
					}

					tid, hasTID := targetItem.NowPlayingItem.ProviderIds["Tmdb"]
					if cfg.TMDBAPIKey != "" && hasTID && tid != "" && (targetItem.NowPlayingItem.Type == "Movie" || targetItem.NowPlayingItem.Type == "Series" || targetItem.NowPlayingItem.Type == "Episode") {
						wg.Add(1)
						go func() {
							defer wg.Done()
							p := getTMDBPosterByID(ctx, cfg.TMDBAPIKey, tid, targetItem.NowPlayingItem.Type)
							var id int
							fmt.Sscanf(tid, "%d", &id)
							resultsChan <- videoResult{idx: 1, poster: p, id: id}
						}()
					}

					if cfg.TMDBAPIKey != "" {
						wg.Add(1)
						go func() {
							defer wg.Done()
							p, id := searchTMDB(ctx, cfg.TMDBAPIKey, searchTitle, prodYear, targetItem.NowPlayingItem.Type)
							resultsChan <- videoResult{idx: 2, poster: p, id: id}
						}()
					}

					go func() {
						wg.Wait()
						close(resultsChan)
					}()

					videoResults := make(map[int]videoResult)

				collectVideo:
					for {
						select {
						case res, ok := <-resultsChan:
							if !ok {
								break collectVideo
							}
							videoResults[res.idx] = res
							if res.idx == 0 && res.poster != "" {
								cancel()
								break collectVideo
							}
						case <-ctx.Done():
							break collectVideo
						}
					}

					for i := 0; i <= 2; i++ {
						if res, ok := videoResults[i]; ok {
							if tmdbID == 0 {
								tmdbID = res.id
							}
							if poster == "" && res.poster != "" {
								poster = res.poster
								logInfo("Search", fmt.Sprintf("Video search successful (Source %d), poster: %s", i, poster))
							}
						}
					}

					if poster == "" && cfg.EpisodeThumbnails && sNum > 0 && eNum > 0 && tmdbID != 0 {
						if still := getTMDBEpisodeStill(ctx, cfg.TMDBAPIKey, tmdbID, sNum, eNum); still != "" {
							poster = still
						}
					}
				}

				if poster == "" {
					logInfo("Image Fetch", "External image providers failed, falling back to Jellyfin artwork.")
					imageID := currentID
					if isAudio {
						if targetItem.NowPlayingItem.AlbumId != "" {
							imageID = targetItem.NowPlayingItem.AlbumId
						} else if targetItem.NowPlayingItem.ParentId != "" {
							imageID = targetItem.NowPlayingItem.ParentId
						}
					} else if targetItem.NowPlayingItem.Type == "Episode" && targetItem.NowPlayingItem.SeriesId != "" {
						if !cfg.EpisodeThumbnails {
							imageID = targetItem.NowPlayingItem.SeriesId
						}
					}

					jellyfinArtworkCache.RLock()
					if cached, ok := jellyfinArtworkCache.m[imageID]; ok && time.Since(cached.Timestamp) < CacheTTL {
						poster = cached.Value
						jellyfinArtworkCache.RUnlock()
					} else {
						jellyfinArtworkCache.RUnlock()
						logInfo("Search", "External providers failed, falling back to Jellyfin artwork.")
						jellyfinFallbackURL := getJellyfinArtwork(cfg.JellyfinURL, cfg.JellyfinToken, imageID)
						if isValidImageURL(jellyfinFallbackURL) {
							poster = jellyfinFallbackURL
							logInfo("Search", fmt.Sprintf("Jellyfin artwork fallback successful: %s", poster))
						} else {
							logWarn("Search", fmt.Sprintf("Jellyfin artwork fallback URL (%s) is not valid or accessible.", jellyfinFallbackURL))
						}
						jellyfinArtworkCache.Lock()
						jellyfinArtworkCache.m[imageID] = struct {
							Value     string
							Timestamp time.Time
						}{Value: poster, Timestamp: time.Now()}
						jellyfinArtworkCache.Unlock()
					}
				}

				if poster == "" {
					poster = PlaceholderImageURL
				}

				if !isAudio {
					ratings = getRatings(cfg.OMDBAPIKey, searchTitle, prodYear)
				}

				if poster == "" {
					if poster != "" {
						logWarn("Search", fmt.Sprintf("No image found for: %s. Using placeholder.", lineOne))
					} else {
						logWarn("Search", fmt.Sprintf("No image found for: %s. Using Discord default.", lineOne))
					}
				}

				*lastPoster = poster
				*lastRatings = ratings
				*lastTMDBID = tmdbID
			}

			activity := Activity{
				Assets: Assets{LargeImage: *lastPoster},
				Type:   3,
			}

			if isAudio {
				activity.Type = 2
			}

			if isPaused {
				statePrefix := "Paused"
				if isBuffering {
					statePrefix = "Buffering"
				}

				activity.Details = lineOne
				if *lastRatings != "" {
					activity.State = statePrefix + " | " + *lastRatings
				} else {
					activity.State = statePrefix
				}
				activity.Assets.SmallImage = "https://images.weserv.nl/?url=" + url.QueryEscape(PauseIconURL) + "&w=64&h=64&inv"
				logInfo("Status", statePrefix+": "+lineOne)
			} else {
				activity.Details = lineOne
				if *lastRatings != "" {
					activity.State = fmt.Sprintf("%s | %s", lineTwo, *lastRatings)
				} else {
					activity.State = lineTwo
				}
				if startUnix > 0 && endUnix > 0 {
					activity.Timestamps = Timestamps{
						Start: startUnix,
						End:   endUnix,
					}
				}
				logInfo("Status", fmt.Sprintf("Playing/Skipped: %s - %s", lineOne, lineTwo))
			}

			if err := drpc.SetActivity(&activity); err == nil {
				*lastItemID, *lastPlayState, *lastPosTicks, *lastUpdateTime, *lastBuffering = currentID, isPaused, posTicks, now, isBuffering
			} else {
				logWarn("Failed to update Discord activity:", err.Error())
			}
		}
	} else if currentID == "" && *lastItemID != "" {
		if err := drpc.ClearActivity(); err == nil {
			logInfo("Status", "Playback stopped")
			*lastItemID = ""
			*lastPosTicks = 0
			*lastUpdateTime = now
			*lastPoster = ""
			*lastRatings = ""
			*lastTMDBID = 0
			*lastBuffering = false
		} else {
			logWarn("Failed to clear Discord activity (stopped):", err.Error())
		}
	} else if currentID == "" && *lastItemID == "" {
		*lastPosTicks = 0
		*lastUpdateTime = now
		*lastPoster = ""
		*lastRatings = ""
		*lastTMDBID = 0
		*lastBuffering = false
	}
}

func startCacheCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			start := time.Now()

			tmdbSearchCache.Lock()
			for k, v := range tmdbSearchCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(tmdbSearchCache.m, k)
				}
			}
			tmdbSearchCache.Unlock()

			tmdbEpisodeStillCache.Lock()
			for k, v := range tmdbEpisodeStillCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(tmdbEpisodeStillCache.m, k)
				}
			}
			tmdbEpisodeStillCache.Unlock()

			anilistSearchCache.Lock()
			for k, v := range anilistSearchCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(anilistSearchCache.m, k)
				}
			}
			anilistSearchCache.Unlock()

			omdbRatingsCache.Lock()
			for k, v := range omdbRatingsCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(omdbRatingsCache.m, k)
				}
			}
			omdbRatingsCache.Unlock()

			itunesSearchCache.Lock()
			for k, v := range itunesSearchCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(itunesSearchCache.m, k)
				}
			}
			itunesSearchCache.Unlock()

			musicbrainzSearchCache.Lock()
			for k, v := range musicbrainzSearchCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(musicbrainzSearchCache.m, k)
				}
			}
			musicbrainzSearchCache.Unlock()

			jellyfinArtworkCache.Lock()
			for k, v := range jellyfinArtworkCache.m {
				if time.Since(v.Timestamp) > CacheTTL {
					delete(jellyfinArtworkCache.m, k)
				}
			}
			jellyfinArtworkCache.Unlock()

			logInfo("Cache", fmt.Sprintf("Periodic cleanup completed in %v", time.Since(start).Truncate(time.Microsecond)))
		}
	}()
}

func main() {
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		logError("Config error", err.Error())
		os.Exit(1)
	}

	startCacheCleanup()

	sig := make(chan os.Signal, 1)

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
	var lastBuffering bool
	var lastPoster string
	var lastRatings string
	var lastTMDBID int
	lastUpdateTime := time.Now()

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

		authHeader := fmt.Sprintf("MediaBrowser Client=\"%s\", Device=\"%s\", DeviceId=\"%s\", Version=\"%s\", Token=\"%s\"",
			ClientName, DeviceName, cfg.DiscordAppID, ClientVersion, cfg.JellyfinToken)
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("User-Agent", ClientName+"/"+ClientVersion)
		req.Header.Set("Accept", "application/json")

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

		updateActivity(drpc, cfg, sessions, &lastItemID, &lastPlayState, &lastPosTicks, &lastUpdateTime, &lastPoster, &lastRatings, &lastTMDBID, &lastBuffering)
		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
