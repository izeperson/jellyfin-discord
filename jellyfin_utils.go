package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func isItemAnime(item JellyfinSession, cfg Config) bool {
	for _, tag := range item.NowPlayingItem.Tags {
		lowerTag := strings.ToLower(tag)
		for _, at := range cfg.AnimeTags {
			if lowerTag == at {
				return true
			}
		}
	}
	return false
}

func getJellyfinArtwork(jellyfinURL, token, itemID string) string {
	fullUrl := fmt.Sprintf("%s/Items/%s/Images/Primary?api_key=%s", jellyfinURL, itemID, token)
	if strings.Contains(jellyfinURL, "10.") || strings.Contains(jellyfinURL, "192.168.") || strings.Contains(jellyfinURL, "127.0.0.1") || strings.Contains(jellyfinURL, "localhost") {
		logWarn("Jellyfin Artwork", "Jellyfin is on a local IP. Images might not show in Discord without a public URL.")
	}
	proxiedUrl := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(fullUrl))
	logInfo("Jellyfin Artwork", fmt.Sprintf("Returning Jellyfin artwork URL (proxied): %s", proxiedUrl))
	return proxiedUrl
}

func isValidImageURL(imageURL string) bool {
	if imageURL == "" {
		return false
	}
	start := time.Now()
	req, err := http.NewRequest("HEAD", imageURL, nil)
	if err != nil {
		logWarn("Validation", fmt.Sprintf("Failed to create HEAD request for %s: %v", imageURL, err))
		return false
	}
	req.Header.Set("User-Agent", ClientName+"/"+ClientVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		logWarn("Validation", fmt.Sprintf("HEAD request failed for %s: %v", imageURL, err))
		return false
	}
	defer resp.Body.Close()

	duration := time.Since(start).Truncate(time.Millisecond)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		contentType := resp.Header.Get("Content-Type")
		valid := strings.HasPrefix(contentType, "image/")
		if !valid {
			logWarn("Validation", fmt.Sprintf("URL %s in %v is not an image: %s", imageURL, duration, contentType))
		}
		return valid
	}
	logWarn("Validation", fmt.Sprintf("URL %s returned status %d in %v", imageURL, resp.StatusCode, duration))
	return false
}

func getMediaDetails(item JellyfinSession, genericText string) (lineOne, lineTwo, searchTitle, prodYear string, sNum, eNum float64) {
	if item.NowPlayingItem.ProductionYear > 0 {
		prodYear = fmt.Sprintf("%.0f", item.NowPlayingItem.ProductionYear)
	}
	switch item.NowPlayingItem.Type {
	case "Episode":
		seriesName := item.NowPlayingItem.SeriesName
		epName := item.NowPlayingItem.Name
		sNum = item.NowPlayingItem.ParentIndexNumber
		eNum = item.NowPlayingItem.IndexNumber
		lineOne = seriesName
		lineTwo = fmt.Sprintf("S%.0f - E%.0f: %s", sNum, eNum, epName)
		searchTitle = seriesName
	case "Audio":
		artistName := ""
		if len(item.NowPlayingItem.Artists) > 0 {
			artistName = item.NowPlayingItem.Artists[0]
		}
		album := item.NowPlayingItem.Album
		track := item.NowPlayingItem.Name
		lineOne = track
		if artistName != "" && album != "" {
			lineTwo = fmt.Sprintf("%s - %s", artistName, album)
		} else if artistName != "" {
			lineTwo = artistName
		} else {
			lineTwo = "on Jellyfin"
		}
		if artistName != "" {
			searchTitle = fmt.Sprintf("%s - %s", artistName, track)
		} else {
			searchTitle = track
		}
	default:
		lineOne = item.NowPlayingItem.Name
		lineTwo = genericText
		searchTitle = lineOne
	}
	return
}
