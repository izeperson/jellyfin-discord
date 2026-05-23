package main

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func getTMDBPosterByID(apiKey string, tmdbID string, itemType string) string {
	if apiKey == "" || tmdbID == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("ID|%s|%s", itemType, tmdbID)
	tmdbSearchCache.RLock()
	if cached, ok := tmdbSearchCache.m[cacheKey]; ok {
		tmdbSearchCache.RUnlock()
		return cached.PosterURL
	}
	tmdbSearchCache.RUnlock()

	category := "movie"
	if itemType == "Series" || itemType == "Episode" {
		category = "tv"
	}

	logInfo("TMDB Lookup", fmt.Sprintf("Fetching poster for TMDB ID: %s (%s)", tmdbID, category))
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?api_key=%s", category, tmdbID, apiKey)
	resp, err := httpClient.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		logWarn("TMDB Lookup", fmt.Sprintf("TMDB ID lookup failed for %s: %v", tmdbID, err))
		return ""
	}
	defer resp.Body.Close()

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

		tmdbSearchCache.Lock()
		tmdbSearchCache.m[cacheKey] = struct {
			PosterURL string
			TMDBID    int
		}{PosterURL: posterURL, TMDBID: res.ID}
		tmdbSearchCache.Unlock()

		return posterURL
	}

	return ""
}

func searchTMDB(apiKey string, query string, year string, itemType string) (posterURL string, tmdbID int) {
	cacheKey := fmt.Sprintf("%s|%s|%s", itemType, query, year)
	if year == "" || year == "0" {
		cacheKey = fmt.Sprintf("%s|%s", itemType, query)
	}

	tmdbSearchCache.RLock()
	if cached, ok := tmdbSearchCache.m[cacheKey]; ok {
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
		logInfo("TMDB Search", fmt.Sprintf("Searching for %s: %s", endpoint, searchQuery))
		searchURL := fmt.Sprintf("https://api.themoviedb.org/3/search/%s?api_key=%s&query=%s", endpoint, apiKey, url.QueryEscape(searchQuery))
		if searchYear != "" && searchYear != "0" && yearParam != "" {
			searchURL += fmt.Sprintf("&%s=%s", yearParam, searchYear)
		}

		logInfo("TMDB Search", fmt.Sprintf("TMDB API request: %s", searchURL))
		resp, err := httpClient.Get(searchURL)
		if err != nil || resp.StatusCode != 200 {
			logWarn("TMDB Search", fmt.Sprintf("TMDB request failed for '%s': %v", searchQuery, err))
			return false
		}
		defer resp.Body.Close()

		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			logWarn("TMDB Search", fmt.Sprintf("TMDB JSON decode error for '%s': %v", searchQuery, err))
			return false
		}
		return len(res.Results) > 0 && res.Results[0].PosterPath != ""
	}

	found := false
	if year != "" && year != "0" {
		found = performSearch(query, year)
	}

	if !found {
		if year != "" && year != "0" {
			logInfo("TMDB Search", "Search with year failed, trying without year...")
		}
		found = performSearch(query, "")
	}

	if found {
		logInfo("TMDB Search", fmt.Sprintf("TMDB found poster: %s", res.Results[0].PosterPath))
		rawURL := "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
		posterURL = fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		tmdbID = res.Results[0].ID

		tmdbSearchCache.Lock()
		tmdbSearchCache.m[cacheKey] = struct {
			PosterURL string
			TMDBID    int
		}{PosterURL: posterURL, TMDBID: tmdbID}
		tmdbSearchCache.Unlock()

		return posterURL, tmdbID
	}
	logInfo("TMDB Search", fmt.Sprintf("TMDB found no poster for: %s", query))
	return
}

func getTMDBEpisodeStill(apiKey string, tmdbID int, seasonNum, epNum float64) string {
	if tmdbID == 0 {
		return ""
	}
	cacheKey := fmt.Sprintf("%d-S%.0fE%.0f", tmdbID, seasonNum, epNum)
	tmdbEpisodeStillCache.RLock()
	if cached, ok := tmdbEpisodeStillCache.m[cacheKey]; ok {
		tmdbEpisodeStillCache.RUnlock()
		return cached
	}
	tmdbEpisodeStillCache.RUnlock()
	logInfo("TMDB Episode Still", fmt.Sprintf("Searching for episode still: %s", cacheKey))
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%.0f/episode/%.0f/images?api_key=%s", tmdbID, seasonNum, epNum, apiKey)
	logInfo("TMDB Episode Still", fmt.Sprintf("TMDB episode still API request: %s", apiURL))
	resp, err := httpClient.Get(apiURL)
	if err != nil || resp.StatusCode != 200 {
		logWarn("TMDB Episode Still", fmt.Sprintf("TMDB episode still API request failed: %v, status %d", err, resp.StatusCode))
		return ""
	}
	defer resp.Body.Close()
	var res struct {
		Stills []struct {
			FilePath string `json:"file_path"`
		} `json:"stills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("TMDB Episode Still", fmt.Sprintf("TMDB episode still JSON decode error: %v", err))
		return ""
	}

	if len(res.Stills) > 0 && res.Stills[0].FilePath != "" {
		logInfo("TMDB Episode Still", fmt.Sprintf("TMDB episode still found for %s: %s", cacheKey, res.Stills[0].FilePath))
		rawURL := "https://image.tmdb.org/t/p/w500" + res.Stills[0].FilePath
		posterURL := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		tmdbEpisodeStillCache.Lock()
		tmdbEpisodeStillCache.m[cacheKey] = posterURL
		tmdbEpisodeStillCache.Unlock()
		return posterURL
	}
	logInfo("TMDB Episode Still", fmt.Sprintf("TMDB episode still found no image for: %s", cacheKey))
	return ""
}
