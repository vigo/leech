//go:build windows

package app

const defaultTermWidth = 80

func terminalWidth() int {
	return defaultTermWidth
}
