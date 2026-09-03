package main

type JellyfinSession struct {
	UserName  string `json:"UserName"`
	UserId    string `json:"UserId"`
	PlayState struct {
		IsPaused      bool    `json:"IsPaused"`
		PositionTicks float64 `json:"PositionTicks"`
	} `json:"PlayState"`
	NowPlayingItem JellyfinItem `json:"NowPlayingItem"`
}

type JellyfinItem struct {
	Id                string            `json:"Id"`
	RunTimeTicks      float64           `json:"RunTimeTicks"`
	ProductionYear    float64           `json:"ProductionYear"`
	Type              string            `json:"Type"`
	Name              string            `json:"Name"`
	SeriesName        string            `json:"SeriesName"`
	ParentId          string            `json:"ParentId"`
	SeriesId          string            `json:"SeriesId"`
	ParentIndexNumber float64           `json:"ParentIndexNumber"`
	IndexNumber       float64           `json:"IndexNumber"`
	Artists           []string          `json:"Artists"`
	AlbumArtist       string            `json:"AlbumArtist"`
	AlbumId           string            `json:"AlbumId"`
	Album             string            `json:"Album"`
	ProviderIds       map[string]string `json:"ProviderIds"`
	Tags              []string          `json:"Tags"`
}
