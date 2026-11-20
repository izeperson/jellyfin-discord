package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	sessionStreamReconnectDelay = 5 * time.Second
	sessionStreamKeepAlive      = 30 * time.Second
)

type jellyfinWebSocketMessage struct {
	MessageType string          `json:"MessageType"`
	Data        json.RawMessage `json:"Data"`
}

func getJellyfinSessions(cfg Config) ([]JellyfinSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	requestURL := strings.TrimRight(cfg.JellyfinURL, "/") + "/Sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", jellyfinAuthorization(cfg))
	req.Header.Set("User-Agent", ClientName+"/"+ClientVersion)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jellyfin sessions request returned %s", resp.Status)
	}
	var sessions []JellyfinSession
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func jellyfinAuthorization(cfg Config) string {
	return fmt.Sprintf("MediaBrowser Client=\"%s\", Device=\"%s\", DeviceId=\"%s\", Version=\"%s\", Token=\"%s\"",
		ClientName, DeviceName, cfg.DiscordAppID, ClientVersion, cfg.JellyfinToken)
}

func jellyfinWebSocketURL(jellyfinURL string) (string, error) {
	parsed, err := url.Parse(jellyfinURL)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported Jellyfin URL scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/websocket"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func parseJellyfinSessionsMessage(payload []byte) (string, []JellyfinSession, error) {
	var message jellyfinWebSocketMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return "", nil, err
	}
	if message.MessageType != "Sessions" {
		return message.MessageType, nil, nil
	}
	data := message.Data
	var encodedData string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &encodedData); err != nil {
			return message.MessageType, nil, err
		}
		data = []byte(encodedData)
	}
	var sessions []JellyfinSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		return message.MessageType, nil, err
	}
	return message.MessageType, sessions, nil
}

func startJellyfinSessionStream(ctx context.Context, cfg Config, updates chan []JellyfinSession) {
	go func() {
		webSocketURL, err := jellyfinWebSocketURL(cfg.JellyfinURL)
		if err != nil {
			logWarn("Jellyfin WebSocket unavailable", err.Error())
			return
		}
		dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
		for ctx.Err() == nil {
			conn, resp, err := dialer.DialContext(ctx, webSocketURL, http.Header{"Authorization": []string{jellyfinAuthorization(cfg)}})
			if err != nil {
				if resp != nil {
					logWarn("Jellyfin WebSocket connection failed", fmt.Sprintf("%s: %v", resp.Status, err))
				} else if ctx.Err() == nil {
					logWarn("Jellyfin WebSocket connection failed", err.Error())
				}
				if !waitForSessionStreamRetry(ctx) {
					return
				}
				continue
			}

			logInfo("Jellyfin WebSocket connected", "")
			if err := conn.WriteJSON(jellyfinWebSocketMessage{MessageType: "SessionsStart", Data: mustJSON(fmt.Sprintf("0,%d", sessionStreamInterval(cfg)))}); err != nil {
				conn.Close()
				if !waitForSessionStreamRetry(ctx) {
					return
				}
				continue
			}
			readJellyfinSessionStream(ctx, conn, updates)
			conn.Close()
			if ctx.Err() == nil {
				logWarn("Jellyfin WebSocket disconnected", "reconnecting")
				if !waitForSessionStreamRetry(ctx) {
					return
				}
			}
		}
	}()
}

func sessionStreamInterval(cfg Config) int {
	interval := cfg.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	return int((time.Duration(interval) * time.Second) / time.Millisecond)
}

func mustJSON(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func readJellyfinSessionStream(ctx context.Context, conn *websocket.Conn, updates chan []JellyfinSession) {
	var writeMu sync.Mutex
	readDone := make(chan struct{})
	defer close(readDone)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-readDone:
		}
	}()

	writeKeepAlive := func() error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteJSON(jellyfinWebSocketMessage{MessageType: "KeepAlive"})
	}

	keepAliveDone := make(chan struct{})
	defer close(keepAliveDone)
	go func() {
		ticker := time.NewTicker(sessionStreamKeepAlive)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-keepAliveDone:
				return
			case <-ticker.C:
				if err := writeKeepAlive(); err != nil {
					return
				}
			}
		}
	}()

	for {
		if ctx.Err() != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		messageType, payload, err := conn.ReadMessage()
		if err != nil || messageType != websocket.TextMessage {
			return
		}
		typeName, sessions, err := parseJellyfinSessionsMessage(payload)
		if err != nil {
			logWarn("Invalid Jellyfin WebSocket message", err.Error())
			continue
		}
		if typeName == "ForceKeepAlive" {
			if err := writeKeepAlive(); err != nil {
				return
			}
		}
		if typeName == "Sessions" {
			select {
			case updates <- sessions:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- sessions:
				default:
				}
			}
		}
	}
}

func waitForSessionStreamRetry(ctx context.Context) bool {
	timer := time.NewTimer(sessionStreamReconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
