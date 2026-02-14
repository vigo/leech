//go:build unix

package app

import (
	"os"
	"syscall"
	"unsafe"
)

const defaultTermWidth = 80

func terminalWidth() int {
	type winsize struct {
		Row, Col, Xpixel, Ypixel uint16
	}

	var ws winsize

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stderr.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 || ws.Col == 0 {
		return defaultTermWidth
	}

	return int(ws.Col)
}
