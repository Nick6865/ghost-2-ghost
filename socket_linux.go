//go:build linux || darwin

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const SO_REUSEPORT = 15

func setReuseControl(fd uintptr) error {
	if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}
	return unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
}
