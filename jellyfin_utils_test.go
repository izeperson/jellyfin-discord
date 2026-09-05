package main

import "testing"

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
