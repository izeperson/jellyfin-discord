package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"time"
)

type DiscordRPC struct {
	conn  net.Conn
	appID string
}

type Activity struct {
	Name       string     `json:"name,omitempty"`
	Details    string     `json:"details,omitempty"`
	State      string     `json:"state,omitempty"`
	Assets     Assets     `json:"assets,omitempty"`
	Timestamps Timestamps `json:"timestamps,omitempty"`
	Type       int        `json:"type,omitempty"`
	Buttons    []Button   `json:"buttons,omitempty"`
}

type Button struct {
	Label string `json:"label"`
	Url   string `json:"url"`
}

type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type Timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

func (d *DiscordRPC) send(opcode int, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, int32(opcode))
	binary.Write(buf, binary.LittleEndian, int32(len(data)))
	buf.Write(data)
	_, err = d.conn.Write(buf.Bytes())
	return err
}

func waitForDiscordReady(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})

	for {
		var header [8]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return fmt.Errorf("waiting for Discord READY event: %w", err)
		}
		opcode := binary.LittleEndian.Uint32(header[:4])
		length := binary.LittleEndian.Uint32(header[4:])
		if length > 1<<20 {
			return fmt.Errorf("Discord handshake response too large: %d bytes", length)
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			return fmt.Errorf("reading Discord handshake response: %w", err)
		}
		if opcode != 1 {
			continue
		}

		var response struct {
			Event string `json:"evt"`
			Data  struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return fmt.Errorf("decoding Discord handshake response: %w", err)
		}
		switch response.Event {
		case "READY":
			return nil
		case "ERROR":
			return fmt.Errorf("Discord handshake failed (%d): %s", response.Data.Code, response.Data.Message)
		}
	}
}

func (d *DiscordRPC) readEvents() {
	for {
		var header [8]byte
		if _, err := io.ReadFull(d.conn, header[:]); err != nil {
			return
		}
		opcode := binary.LittleEndian.Uint32(header[:4])
		length := binary.LittleEndian.Uint32(header[4:])
		if length > 1<<20 {
			logWarn("Discord RPC", fmt.Sprintf("Ignoring oversized event frame: %d bytes", length))
			return
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(d.conn, data); err != nil {
			return
		}
		if opcode != 1 {
			continue
		}

		var event struct {
			Command string `json:"cmd"`
			Event   string `json:"evt"`
			Data    struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			logWarn("Discord RPC", fmt.Sprintf("Could not decode RPC event: %v", err))
			continue
		}
		if event.Event == "ERROR" {
			logWarn("Discord RPC", fmt.Sprintf("Discord rejected %s (%d): %s", event.Command, event.Data.Code, event.Data.Message))
		}
	}
}

func (d *DiscordRPC) Connect() error {
	dirs := getIPCPaths()
	var conn net.Conn
	var err error
	for _, dir := range dirs {
		for i := 0; i < 10; i++ {
			var path string
			if runtime.GOOS == "windows" {
				path = fmt.Sprintf(`%s\discord-ipc-%d`, dir, i)
				conn, err = net.DialTimeout("npipe", path, 2*time.Second)
				if err != nil {
					conn, err = net.Dial("shared", path)
				}
			} else {
				path = fmt.Sprintf("%s/discord-ipc-%d", dir, i)
				conn, err = net.DialTimeout("unix", path, 2*time.Second)
			}
			if err == nil {
				d.conn = conn
				if err := d.send(0, map[string]string{"v": "1", "client_id": d.appID}); err != nil {
					conn.Close()
					continue
				}
				if err := waitForDiscordReady(conn); err != nil {
					conn.Close()
					continue
				}
				go d.readEvents()
				logInfo("Connected to Discord via", path)
				return nil
			}
		}
	}
	return fmt.Errorf("no active socket found: %w", err)
}

func (d *DiscordRPC) SetActivity(a *Activity) error {
	payload := map[string]interface{}{
		"cmd": "SET_ACTIVITY",
		"args": map[string]interface{}{
			"pid":      os.Getpid(),
			"activity": a,
		},
		"nonce": fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	return d.send(1, payload)
}

func (d *DiscordRPC) ClearActivity() error {
	return d.SetActivity(nil)
}

func (d *DiscordRPC) Close() {
	if d.conn != nil {
		d.conn.Close()
	}
}

func connectDiscord(appID string) (*DiscordRPC, error) {
	drpc := &DiscordRPC{appID: appID}
	err := drpc.Connect()
	if err != nil {
		drpc.Close()
		return nil, err
	}
	return drpc, err
}
