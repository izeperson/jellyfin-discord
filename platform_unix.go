//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func getIPCPaths() []string {
	var dirs []string
	xdg := os.Getenv("XDG_RUNTIME_DIR")
	if xdg == "" {
		xdg = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	if xdg != "" {
		dirs = append(dirs, xdg)
		dirs = append(dirs, xdg+"/app/com.discordapp.Discord")
		dirs = append(dirs, xdg+"/app/com.discordapp.DiscordCanary")
		dirs = append(dirs, xdg+"/app/com.discordapp.DiscordDevelopment")
		dirs = append(dirs, xdg+"/app/dev.vencord.Vesktop")
		dirs = append(dirs, xdg+"/app/io.github.equibop.Equibop")
		dirs = append(dirs, xdg+"/app/io.github.equibop.EquibopCanary")
		dirs = append(dirs, xdg+"/.flatpak/com.discordapp.Discord/xdg-run")
		dirs = append(dirs, xdg+"/.flatpak/com.discordapp.DiscordCanary/xdg-run")
		dirs = append(dirs, xdg+"/.flatpak/io.github.equibop.Equibop/xdg-run")
		dirs = append(dirs, xdg+"/snap.discord")
		dirs = append(dirs, xdg+"/discord")
	}
	for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(env); v != "" {
			dirs = append(dirs, v)
		}
	}
	dirs = append(dirs, "/tmp")
	return dirs
}

func getReloadSignal() os.Signal {
	return syscall.SIGHUP
}
