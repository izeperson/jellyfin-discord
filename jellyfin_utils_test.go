package main

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWaitForDiscordReady(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	serverErr := make(chan error, 1)
	go func() {
		payload := []byte(`{"evt":"READY"}`)
		frame := make([]byte, 8+len(payload))
		binary.LittleEndian.PutUint32(frame[:4], 1)
		binary.LittleEndian.PutUint32(frame[4:8], uint32(len(payload)))
		copy(frame[8:], payload)
		_, err := server.Write(frame)
		serverErr <- err
	}()

	if err := waitForDiscordReady(client); err != nil {
		t.Fatalf("waitForDiscordReady() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("mock Discord write error = %v", err)
	}
}

func TestWriteConfigTemplateCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := writeConfigTemplate(path); err != nil {
		t.Fatalf("writeConfigTemplate() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "\"jellyfin_url\"") {
		t.Fatalf("config template missing jellyfin_url: %s", content)
	}
	if !strings.Contains(content, "\"discord_app_id\"") {
		t.Fatalf("config template missing discord_app_id: %s", content)
	}
}

func TestMissingConfigFieldsAreReportedClearly(t *testing.T) {
	cfg := Config{}
	missing := missingConfigFields(cfg)
	if len(missing) != 4 {
		t.Fatalf("len(missing) = %d, want 4", len(missing))
	}
	if !containsString(missing, "jellyfin_url") {
		t.Fatalf("missing fields = %v, want jellyfin_url listed", missing)
	}
	if !containsString(missing, "jellyfin_token") {
		t.Fatalf("missing fields = %v, want jellyfin_token listed", missing)
	}
}

func TestGetMediaDetailsIncludesProductionYear(t *testing.T) {
	var item JellyfinSession
	item.NowPlayingItem.Name = "Example Film"
	item.NowPlayingItem.Type = "Movie"
	item.NowPlayingItem.ProductionYear = 2016

	lineOne, lineTwo, searchTitle, prodYear, _, _ := getMediaDetails(item, DefaultGenericItemText)
	if lineOne != "Example Film (2016)" {
		t.Fatalf("lineOne = %q, want %q", lineOne, "Example Film (2016)")
	}
	if lineTwo != DefaultGenericItemText {
		t.Fatalf("lineTwo = %q, want %q", lineTwo, DefaultGenericItemText)
	}
	if searchTitle != "Example Film" {
		t.Fatalf("searchTitle = %q, want %q", searchTitle, "Example Film")
	}
	if prodYear != "2016" {
		t.Fatalf("prodYear = %q, want %q", prodYear, "2016")
	}
}

func TestGetMediaDetailsUsesAlbumArtistForMusic(t *testing.T) {
	var item JellyfinSession
	item.NowPlayingItem.Name = "Example Track"
	item.NowPlayingItem.Type = "Movie"
	item.NowPlayingItem.AlbumArtist = "Example Artist"
	item.NowPlayingItem.Album = "Example Album"

	lineOne, lineTwo, searchTitle, _, _, _ := getMediaDetails(item, DefaultGenericItemText)
	if !isMusicItem(item) {
		t.Fatal("item was not recognized as music")
	}
	if lineOne != "Example Track" {
		t.Fatalf("lineOne = %q, want %q", lineOne, "Example Track")
	}
	if lineTwo != "Example Artist - Example Album" {
		t.Fatalf("lineTwo = %q, want %q", lineTwo, "Example Artist - Example Album")
	}
	if searchTitle != "Example Artist - Example Track" {
		t.Fatalf("searchTitle = %q, want %q", searchTitle, "Example Artist - Example Track")
	}
}

func TestGetMusicArtworkItemID(t *testing.T) {
	tests := []struct {
		name    string
		track   string
		album   string
		albumID string
		wantID  string
	}{
		{name: "album track uses album ID", track: "Example Track", album: "Example Album", albumID: "album-id", wantID: "album-id"},
		{name: "single uses track ID", track: "Example Single", album: "Example Single", albumID: "single-release-id", wantID: "track-id"},
		{name: "missing album ID uses track ID", track: "Example Track", album: "Example Album", wantID: "track-id"},
		{name: "missing album uses track ID", track: "Example Single", wantID: "track-id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var item JellyfinSession
			item.NowPlayingItem.Name = test.track
			item.NowPlayingItem.Album = test.album
			item.NowPlayingItem.AlbumId = test.albumID
			if got := getMusicArtworkItemID(item, "track-id"); got != test.wantID {
				t.Errorf("getMusicArtworkItemID() = %q, want %q", got, test.wantID)
			}
		})
	}
}

func TestApplyMusicActivityText(t *testing.T) {
	var item JellyfinSession
	item.NowPlayingItem.Name = "Example Track"
	item.NowPlayingItem.Artists = []string{"Example Artist"}
	item.NowPlayingItem.Album = "Example Album"

	tests := []struct {
		name          string
		display       string
		album         string
		wantName      string
		wantDetails   string
		wantState     string
		wantImageText string
	}{
		{name: "default mode", display: DefaultMusicDisplay, album: "Example Album", wantName: "Example Artist", wantDetails: "Example Track", wantState: "Example Artist", wantImageText: "Example Album"},
		{name: "single-character album omits hover text", display: DefaultMusicDisplay, album: "U", wantName: "Example Artist", wantDetails: "Example Track", wantState: "Example Artist"},
		{name: "artist and song", display: "artist_and_song", album: "Example Album", wantName: "Example Track", wantDetails: "Example Track", wantState: "Example Artist - Example Track", wantImageText: "Example Track"},
		{name: "song only", display: "song_only", album: "Example Album", wantName: "Example Track", wantDetails: "Example Track", wantImageText: "Example Track"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item.NowPlayingItem.Album = test.album
			activity := Activity{
				Name:   item.NowPlayingItem.Name,
				Assets: Assets{LargeText: item.NowPlayingItem.Name},
			}
			applyMusicActivityText(&activity, item, test.display)
			if activity.Name != test.wantName {
				t.Errorf("name = %q, want %q", activity.Name, test.wantName)
			}
			if activity.Details != test.wantDetails {
				t.Errorf("details = %q, want %q", activity.Details, test.wantDetails)
			}
			if activity.State != test.wantState {
				t.Errorf("state = %q, want %q", activity.State, test.wantState)
			}
			if activity.Assets.LargeText != test.wantImageText {
				t.Errorf("large text = %q, want %q", activity.Assets.LargeText, test.wantImageText)
			}
		})
	}
}

func TestDiscordActivityUsesMediaName(t *testing.T) {
	activity := Activity{
		Name:    "Example Film",
		Details: "Example Film",
		State:   DefaultGenericItemText,
		Type:    3,
		Assets:  Assets{LargeText: "Example Film"},
	}
	payload, err := json.Marshal(activity)
	if err != nil {
		t.Fatalf("json.Marshal(activity) error = %v", err)
	}
	if !strings.Contains(string(payload), "\"name\":\"Example Film\"") {
		t.Fatalf("activity payload = %s, want media title in name field", payload)
	}
}
