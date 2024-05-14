//go:build darwin || freebsd || linux || netbsd || openbsd
// +build darwin freebsd linux netbsd openbsd

// Package sessionutil contains utility methods required to start session.
package sessionutil

import (
	"os"
	"syscall"
)

// All the signals to handles interrupt
// SIGINT captures Ctrl+C
// SIGQUIT captures Ctrl+\
// SIGTSTP captures Ctrl+Z
var SignalsByteMap = map[os.Signal]byte{
	syscall.SIGINT:  '\003',
	syscall.SIGQUIT: '\x1c',
	syscall.SIGTSTP: '\032',
}

var ControlSignals = []os.Signal{syscall.SIGINT, syscall.SIGTSTP, syscall.SIGQUIT}
