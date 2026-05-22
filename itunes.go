package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

var itunesSearchCache = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

func searchiTunes(query string, expectedArtist string) string {
	cacheKey := query + "|" + expectedArtist
	itunesSearchCache.RLock()
	if cached, ok := itunesSearchCache.m[cacheKey]; ok {
		itunesSearchCache.RUnlock()
		return cached
	}
	itunesSearchCache.RUnlock()
	logInfo("iTunes Search", fmt.Sprintf("Searching for query: %s", query))
	searchURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&entity=song&limit=1", url.QueryEscape(query))
	logInfo("iTunes Search", fmt.Sprintf("iTunes API request: %s", searchURL))
	resp, err := httpClient.Get(searchURL)
	if err != nil || resp.StatusCode != 200 {
		logWarn("iTunes Search", fmt.Sprintf("iTunes API request failed: %v, status %d", err, resp.StatusCode))
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Results []struct {
			ArtistName    string `json:"artistName"`
			ArtworkUrl100 string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("iTunes Search", fmt.Sprintf("iTunes JSON decode error: %v", err))
		return ""
	}

	if len(res.Results) > 0 {
		if expectedArtist != "" && !strings.EqualFold(res.Results[0].ArtistName, expectedArtist) {
			logWarn("iTunes Search", fmt.Sprintf("Artist mismatch for query '%s': expected %s, found %s", query, expectedArtist, res.Results[0].ArtistName))
			return ""
		}

		artwork := res.Results[0].ArtworkUrl100
		// Get higher resolution artwork (512x512)
		posterURL := strings.Replace(artwork, "100x100bb", "512x512bb", 1) // Attempt to get higher resolution
		logInfo("iTunes Search", fmt.Sprintf("iTunes found artwork: %s", posterURL))

		itunesSearchCache.Lock()
		itunesSearchCache.m[cacheKey] = posterURL
		itunesSearchCache.Unlock()
		return posterURL
	}
	logInfo("iTunes Search", fmt.Sprintf("iTunes search found no artwork for query: %s", query))
	return ""
}
