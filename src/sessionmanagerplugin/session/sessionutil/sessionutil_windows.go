//go:build windows
// +build windows

// Package sessionutil provides utility for sessions.
package sessionutil

import (
	"net"
	"os"
	"syscall"

	"github.com/aws/session-manager-plugin/src/log"
	"github.com/aws/session-manager-plugin/src/message"
	"golang.org/x/sys/windows"
)

var EnvProgramFiles = os.Getenv("ProgramFiles")

type DisplayMode struct {
	handle windows.Handle
}

func (d *DisplayMode) InitDisplayMode() {
	var (
		state          uint32
		fileDescriptor int
		err            error
	)

	// gets handler for Stdout
	fileDescriptor = int(syscall.Stdout)
	d.handle = windows.Handle(fileDescriptor)

	// gets current console mode i.e. current console settings
	if err = windows.GetConsoleMode(d.handle, &state); err != nil {
		log.Errorf("error getting console mode: %v", err)
	}

	// this flag is set in order to support control character sequences
	// that control cursor movement, color/font mode
	// refer - https://docs.microsoft.com/en-us/windows/console/setconsolemode
	state |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	// sets the console with new flag
	if err = windows.SetConsoleMode(d.handle, state); err != nil {
		log.Errorf("error setting console mode: %v", err)
	}
}

// DisplayMessage function displays the output on the screen
func (d *DisplayMode) DisplayMessage(message message.ClientMessage) {
	var (
		done *uint32
		err  error
	)

	// writes data to the specified file or input/output (I/O) device
	// refer - https://docs.microsoft.com/en-us/windows/desktop/api/fileapi/nf-fileapi-writefile
	if err = windows.WriteFile(d.handle, message.Payload, done, nil); err != nil {
		log.Errorf("error occurred while writing to file: %v", err)
		return
	}
}

// NewListener starts a new socket listener on the address.
// unix sockets are not supported in older windows versions, start tcp loopback server in such cases
func NewListener(address string) (net.Listener, error) {
	if listener, err := net.Listen("unix", address); err != nil {
		log.Infof("Failed to open unix socket listener, %v. Starting TCP listener.", err)
		return net.Listen("tcp", "localhost:0")
	} else {
		return listener, err
	}
}
