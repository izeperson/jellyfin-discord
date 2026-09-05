package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func getJellyfinItem(ctx context.Context, jellyfinURL, token, userID, itemID string) (JellyfinItem, error) {
	var item JellyfinItem
	requestURL := fmt.Sprintf("%s/Items/%s?userId=%s", strings.TrimRight(jellyfinURL, "/"), url.PathEscape(itemID), url.QueryEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return item, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("MediaBrowser Client=\"%s\", Device=\"%s\", DeviceId=\"%s\", Version=\"%s\", Token=\"%s\"",
		ClientName, DeviceName, ClientName, ClientVersion, token))
	req.Header.Set("User-Agent", ClientName+"/"+ClientVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		return item, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return item, fmt.Errorf("Jellyfin item request returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return item, err
	}
	return item, nil
}

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
		proxiedUrl := fmt.Sprintf("https://images.weserv.nl/?url=%s&w=512&h=512&fit=cover&a=c", url.QueryEscape(fullUrl))
		logInfo("Jellyfin Artwork", fmt.Sprintf("Returning proxied Jellyfin artwork URL for item %s", itemID))
		return proxiedUrl
	}
	logInfo("Jellyfin Artwork", fmt.Sprintf("Returning direct Jellyfin artwork URL for item %s", itemID))
	return fullUrl
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
	if err == nil {
		valid := isImageResponse(resp)
		resp.Body.Close()
		if valid {
			return true
		}
	}

	// Some reverse proxies reject HEAD but serve the same URL with GET.
	getReq, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		logWarn("Validation", fmt.Sprintf("Failed to create GET request for %s: %v", imageURL, err))
		return false
	}
	getReq.Header.Set("User-Agent", ClientName+"/"+ClientVersion)
	getReq.Header.Set("Range", "bytes=0-0")
	getResp, err := httpClient.Do(getReq)
	if err != nil {
		logWarn("Validation", fmt.Sprintf("GET request failed for %s: %v", imageURL, err))
		return false
	}
	defer getResp.Body.Close()
	if isImageResponse(getResp) {
		return true
	}

	duration := time.Since(start).Truncate(time.Millisecond)
	logWarn("Validation", fmt.Sprintf("URL %s returned status %d with content type %q in %v", imageURL, getResp.StatusCode, getResp.Header.Get("Content-Type"), duration))
	return false
}

func isImageResponse(resp *http.Response) bool {
	return resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.HasPrefix(resp.Header.Get("Content-Type"), "image/")
}

func isMusicItem(item JellyfinSession) bool {
	return item.NowPlayingItem.Type == "Audio" ||
		item.NowPlayingItem.Type == "MusicVideo" ||
		len(item.NowPlayingItem.Artists) > 0 ||
		item.NowPlayingItem.AlbumArtist != ""
}

func getMusicArtist(item JellyfinSession) string {
	if len(item.NowPlayingItem.Artists) > 0 {
		return item.NowPlayingItem.Artists[0]
	}
	return item.NowPlayingItem.AlbumArtist
}

func getMediaDetails(item JellyfinSession, genericText string) (lineOne, lineTwo, searchTitle, prodYear string, sNum, eNum float64) {
	if item.NowPlayingItem.ProductionYear > 0 {
		prodYear = fmt.Sprintf("%.0f", item.NowPlayingItem.ProductionYear)
	}
	if isMusicItem(item) {
		artistName := getMusicArtist(item)
		album := item.NowPlayingItem.Album
		track := item.NowPlayingItem.Name
		lineOne = track
		if artistName != "" && album != "" {
			lineTwo = fmt.Sprintf("%s - %s", artistName, album)
		} else if artistName != "" {
			lineTwo = artistName
		} else {
			lineTwo = DefaultGenericItemText
		}
		if artistName != "" {
			searchTitle = fmt.Sprintf("%s - %s", artistName, track)
		} else {
			searchTitle = track
		}
		return
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
	default:
		lineOne = item.NowPlayingItem.Name
		if prodYear != "" {
			lineOne = fmt.Sprintf("%s (%s)", lineOne, prodYear)
		}
		lineTwo = genericText
		searchTitle = item.NowPlayingItem.Name
	}
	return
}
