//go:build windows
// +build windows

// Package sessionutil contains utility methods required to start session.
package sessionutil

import (
	"os"
	"syscall"
)

// All the signals to handles interrupt
// SIGINT captures Ctrl+C
// SIGQUIT captures Ctrl+Z
var SignalsByteMap = map[os.Signal]byte{
	syscall.SIGINT:  '\003',
	syscall.SIGQUIT: '\x1c',
}

var ControlSignals = []os.Signal{syscall.SIGINT, syscall.SIGQUIT}
