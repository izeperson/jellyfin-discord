package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var lastfmSearchCache = struct {
	sync.RWMutex
	m map[string]struct {
		Value     string
		Timestamp time.Time
	}
}{m: make(map[string]struct {
	Value     string
	Timestamp time.Time
})}

func searchLastFM(ctx context.Context, apiKey string, artist string, track string) string {
	if artist == "" || track == "" {
		return ""
	}

	cacheKey := artist + "|" + track
	lastfmSearchCache.RLock()
	if cached, ok := lastfmSearchCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		lastfmSearchCache.RUnlock()
		return cached.Value
	}
	lastfmSearchCache.RUnlock()

	start := time.Now()
	var posterURL string

	if apiKey == "" {
		// Scrape fallback if no API key is provided
		searchURL := fmt.Sprintf("https://www.last.fm/music/%s/_/%s",
			url.PathEscape(artist), url.PathEscape(track))

		req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
		if err != nil {
			return ""
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			html := string(body)

			// Look for Open Graph image meta tag
			const ogTag = `<meta property="og:image" content="`
			if idx := strings.Index(html, ogTag); idx != -1 {
				startIdx := idx + len(ogTag)
				endIdx := strings.Index(html[startIdx:], `"`)
				if endIdx != -1 {
					posterURL = html[startIdx : startIdx+endIdx]
					// Last.fm usually provides a 300x300 image in og:image,
					// we can try to get a larger one by modifying the URL.
					posterURL = strings.Replace(posterURL, "/300x300/", "/512x512/", 1)
				}
			}
		}

		duration := time.Since(start).Truncate(time.Millisecond)
		if posterURL != "" {
			logInfo("Last.fm", fmt.Sprintf("Found artwork via scraping in %v: %s", duration, posterURL))
		} else {
			logWarn("Last.fm", fmt.Sprintf("Scraping failed to find artwork in %v", duration))
		}

		lastfmSearchCache.Lock()
		lastfmSearchCache.m[cacheKey] = struct {
			Value     string
			Timestamp time.Time
		}{Value: posterURL, Timestamp: time.Now()}
		lastfmSearchCache.Unlock()
		return posterURL
	}

	searchURL := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=track.getInfo&api_key=%s&artist=%s&track=%s&format=json",
		apiKey, url.QueryEscape(artist), url.QueryEscape(track))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("Last.fm", fmt.Sprintf("API request failed: %v", err))
		}
		return ""
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("Last.fm", fmt.Sprintf("API request failed in %v: status %d", duration, resp.StatusCode))
		return ""
	}

	var res struct {
		Track struct {
			Album struct {
				Image []struct {
					Text string `json:"#text"`
					Size string `json:"size"`
				} `json:"image"`
			} `json:"album"`
		} `json:"track"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if ctx.Err() == nil {
			logWarn("Last.fm", fmt.Sprintf("JSON decode error: %v", err))
		}
		return ""
	}

	images := res.Track.Album.Image
	if len(images) > 0 {
		for i := len(images) - 1; i >= 0; i-- {
			if images[i].Text != "" {
				posterURL = images[i].Text
				break
			}
		}
	}

	lastfmSearchCache.Lock()
	lastfmSearchCache.m[cacheKey] = struct {
		Value     string
		Timestamp time.Time
	}{Value: posterURL, Timestamp: time.Now()}
	lastfmSearchCache.Unlock()

	if posterURL != "" {
		logInfo("Last.fm", fmt.Sprintf("Found artwork in %v: %s", duration, posterURL))
	}
	return posterURL
}
