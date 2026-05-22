package main

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func getRatings(apiKey string, query string, year string) string {
	cacheKey := fmt.Sprintf("%s-%s", query, year)
	omdbRatingsCache.RLock()
	if cached, ok := omdbRatingsCache.m[cacheKey]; ok {
		omdbRatingsCache.RUnlock()
		return cached
	}
	omdbRatingsCache.RUnlock()
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
	if imdb != "" && rt != "" {
		return fmt.Sprintf("%s  %s", imdb, rt)
	} else if imdb != "" || rt != "" {
		result := imdb + rt
		omdbRatingsCache.Lock()
		omdbRatingsCache.m[cacheKey] = result
		omdbRatingsCache.Unlock()
		return result
	}
	return ""
}
