//go:build windows
// +build windows

package main

import (
	"os"
)

func getIPCPaths() []string {
	return []string{`\\.\pipe`}
}

func getReloadSignal() os.Signal {
	return nil
}
