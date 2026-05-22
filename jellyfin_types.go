package main

type JellyfinSession struct {
	UserName  string `json:"UserName"`
	PlayState struct {
		IsPaused      bool    `json:"IsPaused"`
		PositionTicks float64 `json:"PositionTicks"`
	} `json:"PlayState"`
	NowPlayingItem struct {
		Id                string   `json:"Id"`
		RunTimeTicks      float64  `json:"RunTimeTicks"`
		ProductionYear    float64  `json:"ProductionYear"`
		Type              string   `json:"Type"`
		Name              string   `json:"Name"`
		SeriesName        string   `json:"SeriesName"`
		ParentIndexNumber float64  `json:"ParentIndexNumber"`
		IndexNumber       float64  `json:"IndexNumber"`
		Artists           []string `json:"Artists"`
		Album             string   `json:"Album"`
		Tags              []string `json:"Tags"` // Added to support anime detection
	} `json:"NowPlayingItem"`
}
