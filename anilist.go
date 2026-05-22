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
	req, err := http.NewRequest("POST", "https://graphql.anilist.co", bytes.NewBuffer(body))
	if err != nil {
		return "", ""
	}
	req.Header.Set("Content-Type", "application/json")

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
		return "", ""
	}

	posterURL = res.Data.Media.CoverImage.Large
	if res.Data.Media.AverageScore > 0 {
		score = fmt.Sprintf("❤️ %d%%", res.Data.Media.AverageScore)
		anilistSearchCache.Lock()
		anilistSearchCache.m[query] = struct {
			PosterURL string
			Score     string
		}{PosterURL: posterURL, Score: score}
		anilistSearchCache.Unlock()
		return posterURL, score
	}
	return
}
