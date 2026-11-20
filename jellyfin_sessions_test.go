package main

import "testing"

func TestJellyfinWebSocketURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "http://localhost:8096", want: "ws://localhost:8096/websocket"},
		{input: "https://media.example.com/jellyfin/", want: "wss://media.example.com/jellyfin/websocket"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := jellyfinWebSocketURL(test.input)
			if err != nil {
				t.Fatalf("jellyfinWebSocketURL() error = %v", err)
			}
			if got != test.want {
				t.Errorf("jellyfinWebSocketURL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseJellyfinSessionsMessage(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "raw data array",
			payload: `{"MessageType":"Sessions","Data":[{"UserName":"admin","NowPlayingItem":{"Id":"item-1"},"PlayState":{"PositionTicks":120}}]}`,
		},
		{
			name:    "encoded data array",
			payload: `{"MessageType":"Sessions","Data":"[{\"UserName\":\"admin\",\"NowPlayingItem\":{\"Id\":\"item-1\"},\"PlayState\":{\"PositionTicks\":120}}]"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messageType, sessions, err := parseJellyfinSessionsMessage([]byte(test.payload))
			if err != nil {
				t.Fatalf("parseJellyfinSessionsMessage() error = %v", err)
			}
			if messageType != "Sessions" {
				t.Fatalf("message type = %q, want Sessions", messageType)
			}
			if len(sessions) != 1 {
				t.Fatalf("len(sessions) = %d, want 1", len(sessions))
			}
			if sessions[0].UserName != "admin" || sessions[0].NowPlayingItem.Id != "item-1" || sessions[0].PlayState.PositionTicks != 120 {
				t.Errorf("parsed session = %+v, want admin playing item-1 at tick 120", sessions[0])
			}
		})
	}
}

func TestSessionStreamIntervalUsesConfiguredPollInterval(t *testing.T) {
	if got, want := sessionStreamInterval(Config{PollInterval: 3}), 3000; got != want {
		t.Errorf("sessionStreamInterval() = %d, want %d milliseconds", got, want)
	}
	if got, want := sessionStreamInterval(Config{}), 3000; got != want {
		t.Errorf("sessionStreamInterval() with unset interval = %d, want %d milliseconds", got, want)
	}
}
