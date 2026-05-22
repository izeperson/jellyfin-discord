package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"
)

var musicbrainzSearchCache = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

func searchMusicBrainz(query string, expectedArtist string) string {
	cacheKey := query + "|" + expectedArtist
	musicbrainzSearchCache.RLock()
	if cached, ok := musicbrainzSearchCache.m[cacheKey]; ok {
		musicbrainzSearchCache.RUnlock()
		return cached
	}
	musicbrainzSearchCache.RUnlock()

	logInfo("MusicBrainz Search", fmt.Sprintf("Searching for query: %s", query))

	var mbQuery string
	if expectedArtist != "" {
		// Use advanced query syntax for strict artist matching
		mbQuery = fmt.Sprintf("artist:\"%s\" AND (release:\"%s\" OR \"%s\")", expectedArtist, query, query)
	} else {
		mbQuery = query
	}

	// Search for releases (albums) first, as they are more likely to have cover art
	searchURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release/?query=%s&fmt=json&limit=1", url.QueryEscape(mbQuery))
	logInfo("MusicBrainz Search", fmt.Sprintf("MusicBrainz API request (release): %s", searchURL))
	resp, err := httpClient.Get(searchURL)
	if err != nil || resp.StatusCode != 200 {
		logWarn("MusicBrainz Search", fmt.Sprintf("MusicBrainz release API request failed: %v, status %d", err, resp.StatusCode))
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Releases []struct {
			ID string `json:"id"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("MusicBrainz Search", fmt.Sprintf("MusicBrainz JSON decode error (release): %v", err))
		return ""
	}

	if len(res.Releases) > 0 {
		mbid := res.Releases[0].ID
		// Construct Cover Art Archive URL
		posterURL := fmt.Sprintf("https://coverartarchive.org/release/%s/front", mbid)
		logInfo("MusicBrainz Search", fmt.Sprintf("MusicBrainz found artwork via Cover Art Archive: %s", posterURL))

		// Validate the image URL before caching and returning
		// Note: Cover Art Archive redirects, so a simple HEAD request might not be enough.
		// We'll rely on Discord's ability to follow redirects for now.
		// If this still causes issues, we might need a more robust validation here.

		musicbrainzSearchCache.Lock()
		musicbrainzSearchCache.m[cacheKey] = posterURL
		musicbrainzSearchCache.Unlock()
		return posterURL
	}

	// If no release found, try searching for recordings (tracks) - less likely to have direct cover art
	// This part is commented out for now to avoid potentially irrelevant results,
	// as track-level cover art is rare and often just links to the album.
	// If needed, this can be re-enabled with logic to find the associated release MBID.

	logInfo("MusicBrainz Search", fmt.Sprintf("MusicBrainz search found no artwork for query: %s", query))
	return ""
}

// MusicBrainz API has a rate limit of 1 request per second.
// We should ensure our calls respect this. The httpClient's timeout helps,
// but explicit rate limiting might be needed if many requests are made in quick succession.
// For now, with caching, this should be sufficient.
var _ = time.Second // Placeholder to avoid unused import warning if not used elsewhere
