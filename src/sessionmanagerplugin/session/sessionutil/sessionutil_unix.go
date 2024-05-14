//go:build darwin || freebsd || linux || netbsd || openbsd
// +build darwin freebsd linux netbsd openbsd

// Package sessionutil provides utility for sessions.
package sessionutil

import (
	"fmt"
	"net"

	"github.com/aws/session-manager-plugin/src/message"
)

type DisplayMode struct {
}

func (d *DisplayMode) InitDisplayMode() {
}

// DisplayMessage function displays the output on the screen
func (d *DisplayMode) DisplayMessage(message message.ClientMessage) {
	fmt.Print(string(message.Payload))
}

// NewListener starts a new socket listener on the address.
func NewListener(address string) (net.Listener, error) {
	return net.Listen("unix", address)
}
