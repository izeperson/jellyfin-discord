package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func getTMDBPosterByID(ctx context.Context, apiKey string, tmdbID string, itemType string) string {
	if apiKey == "" || tmdbID == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("ID|%s|%s", itemType, tmdbID)
	tmdbSearchCache.RLock()
	if cached, ok := tmdbSearchCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		tmdbSearchCache.RUnlock()
		return cached.PosterURL
	}
	tmdbSearchCache.RUnlock()

	category := "movie"
	if itemType == "Series" || itemType == "Episode" {
		category = "tv"
	}

	start := time.Now()
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?api_key=%s", category, tmdbID, apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("TMDB", fmt.Sprintf("ID lookup failed for %s: %v", tmdbID, err))
		}
		return ""
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("TMDB", fmt.Sprintf("ID lookup failed in %v for %s: status %d", duration, tmdbID, resp.StatusCode))
		return ""
	}

	var res struct {
		PosterPath string `json:"poster_path"`
		ID         int    `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}

	if res.PosterPath != "" {
		rawURL := "https://image.tmdb.org/t/p/w500" + res.PosterPath
		posterURL := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		if ctx.Err() != nil {
			return ""
		}
		logInfo("TMDB", fmt.Sprintf("ID lookup successful in %v for %s: %s", duration, tmdbID, posterURL))

		tmdbSearchCache.Lock()
		tmdbSearchCache.m[cacheKey] = struct {
			PosterURL string
			TMDBID    int
			Timestamp time.Time
		}{PosterURL: posterURL, TMDBID: res.ID, Timestamp: time.Now()}
		tmdbSearchCache.Unlock()

		return posterURL
	}

	return ""
}

func searchTMDB(ctx context.Context, apiKey string, query string, year string, itemType string) (posterURL string, tmdbID int) {
	cacheKey := fmt.Sprintf("%s|%s|%s", itemType, query, year)
	if year == "" || year == "0" {
		cacheKey = fmt.Sprintf("%s|%s", itemType, query)
	}

	tmdbSearchCache.RLock()
	if cached, ok := tmdbSearchCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		tmdbSearchCache.RUnlock()
		return cached.PosterURL, cached.TMDBID
	}
	tmdbSearchCache.RUnlock()

	if apiKey == "" {
		return "", 0
	}

	endpoint := "multi"
	yearParam := ""
	switch itemType {
	case "Movie":
		endpoint = "movie"
		yearParam = "primary_release_year"
	case "Series", "Episode":
		endpoint = "tv"
		yearParam = "first_air_date_year"
	}

	var res struct {
		Results []struct {
			ID         int    `json:"id"`
			PosterPath string `json:"poster_path"`
		} `json:"results"`
	}

	performSearch := func(searchQuery string, searchYear string) bool {
		searchURL := fmt.Sprintf("https://api.themoviedb.org/3/search/%s?api_key=%s&query=%s", endpoint, apiKey, url.QueryEscape(searchQuery))
		if searchYear != "" && searchYear != "0" && yearParam != "" {
			searchURL += fmt.Sprintf("&%s=%s", yearParam, searchYear)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
		if err != nil {
			return false
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			if ctx.Err() == nil {
				logWarn("TMDB", fmt.Sprintf("Search request failed for '%s': %v", searchQuery, err))
			}
			return false
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			logWarn("TMDB", fmt.Sprintf("Search request failed for '%s': status %d", searchQuery, resp.StatusCode))
			return false
		}

		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			logWarn("TMDB", fmt.Sprintf("JSON decode error for '%s': %v", searchQuery, err))
			return false
		}
		return len(res.Results) > 0 && res.Results[0].PosterPath != ""
	}

	start := time.Now()
	found := false
	if year != "" && year != "0" {
		found = performSearch(query, year)
	}

	if !found {
		if year != "" && year != "0" {
			logInfo("TMDB", "Search with year failed, trying without year...")
		}
		found = performSearch(query, "")
	}

	duration := time.Since(start).Truncate(time.Millisecond)
	if found {
		rawURL := "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
		posterURL = fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		tmdbID = res.Results[0].ID
		if ctx.Err() != nil {
			return "", 0
		}
		logInfo("TMDB", fmt.Sprintf("Search successful in %v: %s", duration, posterURL))

		tmdbSearchCache.Lock()
		tmdbSearchCache.m[cacheKey] = struct {
			PosterURL string
			TMDBID    int
			Timestamp time.Time
		}{PosterURL: posterURL, TMDBID: tmdbID, Timestamp: time.Now()}
		tmdbSearchCache.Unlock()

		return posterURL, tmdbID
	}
	if ctx.Err() != nil {
		return "", 0
	}
	logInfo("TMDB", fmt.Sprintf("No poster found in %v for: %s", duration, query))
	return
}

func getTMDBEpisodeStill(ctx context.Context, apiKey string, tmdbID int, seasonNum, epNum float64) string {
	if tmdbID == 0 {
		return ""
	}
	cacheKey := fmt.Sprintf("%d-S%.0fE%.0f", tmdbID, seasonNum, epNum)
	tmdbEpisodeStillCache.RLock()
	if cached, ok := tmdbEpisodeStillCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		tmdbEpisodeStillCache.RUnlock()
		return cached.Value
	}
	tmdbEpisodeStillCache.RUnlock()
	start := time.Now()
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%.0f/episode/%.0f/images?api_key=%s", tmdbID, seasonNum, epNum, apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			logWarn("TMDB", fmt.Sprintf("Episode still API request failed: %v", err))
		}
		return ""
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("TMDB", fmt.Sprintf("Episode still API request failed in %v: status %d", duration, resp.StatusCode))
		return ""
	}
	var res struct {
		Stills []struct {
			FilePath string `json:"file_path"`
		} `json:"stills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		if ctx.Err() == nil {
			logWarn("TMDB", fmt.Sprintf("Episode still JSON decode error: %v", err))
		}
		return ""
	}

	if len(res.Stills) > 0 && res.Stills[0].FilePath != "" {
		rawURL := "https://image.tmdb.org/t/p/w500" + res.Stills[0].FilePath
		posterURL := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		if ctx.Err() != nil {
			return ""
		}
		logInfo("TMDB", fmt.Sprintf("Episode still found in %v for %s: %s", duration, cacheKey, posterURL))
		tmdbEpisodeStillCache.Lock()
		tmdbEpisodeStillCache.m[cacheKey] = struct {
			Value     string
			Timestamp time.Time
		}{Value: posterURL, Timestamp: time.Now()}
		tmdbEpisodeStillCache.Unlock()
		return posterURL
	}
	if ctx.Err() != nil {
		return ""
	}
	logInfo("TMDB", fmt.Sprintf("No episode still found in %v for: %s", duration, cacheKey))
	return ""
}
