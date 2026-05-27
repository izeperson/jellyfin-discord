package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	return drpc, err
}
