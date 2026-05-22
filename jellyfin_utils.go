package main

import (
	"fmt"
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
	if strings.Contains(jellyfinURL, "10.") || strings.Contains(jellyfinURL, "192.168.") || strings.Contains(jellyfinURL, "127.0.0.1") || strings.Contains(jellyfinURL, "localhost") {
		logWarn("Image fallback", "Jellyfin is on a local IP. Images might not show in Discord without a public URL.")
		return fullUrl
	}
	return fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(fullUrl))
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
		searchTitle = track
	default:
		lineOne = item.NowPlayingItem.Name
		lineTwo = genericText
		searchTitle = lineOne
	}
	return
}
