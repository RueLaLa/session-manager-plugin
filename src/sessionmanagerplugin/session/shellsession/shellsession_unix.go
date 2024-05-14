// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing
// permissions and limitations under the License.

//go:build darwin || freebsd || linux || netbsd || openbsd
// +build darwin freebsd linux netbsd openbsd

// Package shellsession starts shell session.
package shellsession

import (
	"bufio"
	"os"
	"time"

	"github.com/aws/session-manager-plugin/src/log"
	"github.com/aws/session-manager-plugin/src/message"
	"golang.org/x/term"
)

// stop restores the terminal settings and exits
func (s *ShellSession) Stop() {
	term.Restore(int(os.Stdin.Fd()), s.originalTermState)
}

// handleKeyboardInput handles input entered by customer on terminal
func (s *ShellSession) handleKeyboardInput() (err error) {
	var (
		stdinBytesLen int
	)

	s.originalTermState, err = term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Errorf("Error switching terminal to raw mode: %s", err)
		return
	}

	ch := make(chan []byte)
	go func(ch chan []byte) {
		reader := bufio.NewReader(os.Stdin)
		for {
			stdinBytes := make([]byte, StdinBufferLimit)
			stdinBytesLen, _ = reader.Read(stdinBytes)
			ch <- stdinBytes
		}
	}(ch)

	for {
		select {
		case <-time.After(time.Second):
			if s.Session.DataChannel.IsSessionEnded() {
				return
			}
		case stdinBytes := <-ch:
			if err = s.Session.DataChannel.SendInputDataMessage(message.Output, stdinBytes[:stdinBytesLen]); err != nil {
				return
			}
		}
	}
}
