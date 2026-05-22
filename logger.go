package main

import "fmt"

func logInfo(msg string, detail string) {
	fmt.Printf("%sINFO%s  [%s] %s %s\n", ColorGreen, ColorReset, "jellyfin-rpc", msg, detail)
}

func logWarn(msg string, detail string) {
	fmt.Printf("%sWARN%s  [%s] %s %s\n", ColorYellow, ColorReset, "jellyfin-rpc", msg, detail)
}

func logError(msg string, detail string) {
	fmt.Printf("%sERROR%s [%s] %s %s\n", ColorRed, ColorReset, "jellyfin-rpc", msg, detail)
}
