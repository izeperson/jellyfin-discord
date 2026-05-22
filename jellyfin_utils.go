package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// isItemAnime checks if a Jellyfin item is tagged as anime using the configured tags.
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
	// If Jellyfin is on a local IP, Discord won't be able to display it.
	// We still return the local URL, but the main logic will try external providers first.
	if strings.Contains(jellyfinURL, "10.") || strings.Contains(jellyfinURL, "192.168.") || strings.Contains(jellyfinURL, "127.0.0.1") || strings.Contains(jellyfinURL, "localhost") {
		logWarn("Jellyfin Artwork", "Jellyfin is on a local IP. Images might not show in Discord without a public URL.")
	}
	proxiedUrl := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(fullUrl))
	logInfo("Jellyfin Artwork", fmt.Sprintf("Returning Jellyfin artwork URL (proxied): %s", proxiedUrl))
	return proxiedUrl
}

// isValidImageURL performs a HEAD request to check if the URL returns a valid image.
func isValidImageURL(imageURL string) bool {
	if imageURL == "" {
		return false
	}
	// Use GET instead of HEAD as some proxies/servers handle HEAD poorly
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		logWarn("Image Validation", fmt.Sprintf("Failed to create GET request for %s: %v", imageURL, err))
		return false
	}
	req.Header.Set("User-Agent", ClientName+"/"+ClientVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		logWarn("Image Validation", fmt.Sprintf("GET request failed for %s: %v", imageURL, err))
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		contentType := resp.Header.Get("Content-Type")
		return strings.HasPrefix(contentType, "image/")
	}
	logWarn("Image Validation", fmt.Sprintf("Image URL %s returned status code %d", imageURL, resp.StatusCode))
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
