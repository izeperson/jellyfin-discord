package main

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func searchTMDB(apiKey string, query string) (posterURL string, tmdbID int) {
	tmdbSearchCache.RLock()
	if cached, ok := tmdbSearchCache.m[query]; ok {
		tmdbSearchCache.RUnlock()
		return cached.PosterURL, cached.TMDBID
	}
	tmdbSearchCache.RUnlock()

	if apiKey == "" {
		return "", 0
	}
	searchURL := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?api_key=%s&query=%s", apiKey, url.QueryEscape(query))
	resp, err := httpClient.Get(searchURL)
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
		return "", 0
	}

	if len(res.Results) > 0 && res.Results[0].PosterPath != "" {
		rawURL := "https://image.tmdb.org/t/p/w500" + res.Results[0].PosterPath
		posterURL = fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		tmdbID = res.Results[0].ID

		tmdbSearchCache.Lock()
		tmdbSearchCache.m[query] = struct {
			PosterURL string
			TMDBID    int
		}{PosterURL: posterURL, TMDBID: tmdbID}
		tmdbSearchCache.Unlock()

		return posterURL, tmdbID
	}
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
		return ""
	}

	if len(res.Stills) > 0 && res.Stills[0].FilePath != "" {
		rawURL := "https://image.tmdb.org/t/p/w500" + res.Stills[0].FilePath
		posterURL := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512", url.QueryEscape(rawURL))
		tmdbEpisodeStillCache.Lock()
		tmdbEpisodeStillCache.m[cacheKey] = posterURL
		tmdbEpisodeStillCache.Unlock()
		return posterURL
	}
	return ""
}
