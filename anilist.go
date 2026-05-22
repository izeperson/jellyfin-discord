package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func searchAniList(query string) (posterURL string, score string) {
	anilistSearchCache.RLock()
	if cached, ok := anilistSearchCache.m[query]; ok {
		anilistSearchCache.RUnlock()
		return cached.PosterURL, cached.Score
	}
	anilistSearchCache.RUnlock()

	logInfo("AniList Search", fmt.Sprintf("Searching for query: %s", query))
	jsonData := map[string]interface{}{
		"query": `
			query ($search: String) {
				Media (search: $search, type: ANIME) {
					coverImage { large }
					averageScore
				}
			}`,
		"variables": map[string]string{"search": query},
	}

	body, _ := json.Marshal(jsonData)
	logInfo("AniList Search", fmt.Sprintf("AniList GraphQL request for query: %s", query))
	req, err := http.NewRequest("POST", "https://graphql.anilist.co", bytes.NewBuffer(body))
	if err != nil {
		return "", ""
	}
	req.Header.Set("Content-Type", "application/json")

	// No specific log for request URL as it's always "https://graphql.anilist.co"
	resp, err := httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return "", ""
	}
	defer resp.Body.Close()

	var res struct {
		Data struct {
			Media struct {
				CoverImage struct {
					Large string `json:"large"`
				} `json:"coverImage"`
				AverageScore int `json:"averageScore"`
			} `json:"Media"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logWarn("AniList Search", fmt.Sprintf("AniList JSON decode error: %v", err))
		return "", ""
	}

	posterURL = res.Data.Media.CoverImage.Large
	if res.Data.Media.AverageScore > 0 {
		score = fmt.Sprintf("❤️ %d%%", res.Data.Media.AverageScore)
	}
	if posterURL != "" {
		logInfo("AniList Search", fmt.Sprintf("AniList found artwork: %s (Score: %s)", posterURL, score))
		anilistSearchCache.Lock()
		anilistSearchCache.m[query] = struct {
			PosterURL string
			Score     string
		}{PosterURL: posterURL, Score: score}
		anilistSearchCache.Unlock()
	}
	logInfo("AniList Search", fmt.Sprintf("AniList search found no artwork for query: %s", query))
	return
}
