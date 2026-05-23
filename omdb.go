package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

func getRatings(apiKey string, query string, year string) string {
	cacheKey := fmt.Sprintf("%s-%s", query, year)
	omdbRatingsCache.RLock()
	if cached, ok := omdbRatingsCache.m[cacheKey]; ok && time.Since(cached.Timestamp) < CacheTTL {
		omdbRatingsCache.RUnlock()
		return cached.Value
	}
	omdbRatingsCache.RUnlock()

	if apiKey == "" {
		return ""
	}
	apiURL := fmt.Sprintf("https://www.omdbapi.com/?apikey=%s&t=%s", apiKey, url.QueryEscape(query))
	if year != "" && year != "0" {
		apiURL += fmt.Sprintf("&y=%s", year)
	}
	start := time.Now()
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode != 200 {
		logWarn("OMDb", fmt.Sprintf("API request failed in %v: status %d", duration, resp.StatusCode))
		return ""
	}

	var res struct {
		Ratings []struct {
			Source string `json:"Source"`
			Value  string `json:"Value"`
		} `json:"Ratings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("OMDb", fmt.Sprintf("Failed to decode response: %v", err))
		return ""
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

	var result string
	if imdb != "" && rt != "" {
		result = fmt.Sprintf("%s  %s", imdb, rt)
	} else {
		result = imdb + rt
	}
	logInfo("OMDb", fmt.Sprintf("Ratings fetched in %v: %s", duration, result))

	omdbRatingsCache.Lock()
	omdbRatingsCache.m[cacheKey] = struct {
		Value     string
		Timestamp time.Time
	}{Value: result, Timestamp: time.Now()}
	omdbRatingsCache.Unlock()
	return result
}
