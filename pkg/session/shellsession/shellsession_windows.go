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

//go:build windows
// +build windows

// Package shellsession starts shell session.
package shellsession

import (
	"time"

	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/aws/session-manager-plugin/pkg/message"
	"github.com/eiannone/keyboard"
)

// Byte array for key inputs
// Note: F11 cannot be converted to byte array
var specialKeysInputMap = map[keyboard.Key][]byte{
	keyboard.KeyEsc:        {27},
	keyboard.KeyArrowUp:    {27, 79, 65},
	keyboard.KeyArrowDown:  {27, 79, 66},
	keyboard.KeyArrowRight: {27, 79, 67},
	keyboard.KeyArrowLeft:  {27, 79, 68},
	keyboard.KeyF1:         {27, 79, 80},
	keyboard.KeyF2:         {27, 79, 81},
	keyboard.KeyF3:         {27, 79, 82},
	keyboard.KeyF4:         {27, 79, 83},
	keyboard.KeyF5:         {27, 91, 49, 53, 126},
	keyboard.KeyF6:         {27, 91, 49, 55, 126},
	keyboard.KeyF7:         {27, 91, 49, 56, 126},
	keyboard.KeyF8:         {27, 91, 49, 57, 126},
	keyboard.KeyF9:         {27, 91, 50, 48, 126},
	keyboard.KeyF10:        {27, 91, 50, 49, 126},
	keyboard.KeyF12:        {27, 91, 50, 52, 126},
	keyboard.KeyHome:       {27, 91, 72},
	keyboard.KeyEnd:        {27, 91, 70},
	keyboard.KeyInsert:     {27, 91, 50, 126},
	keyboard.KeyDelete:     {27, 91, 51, 126},
	keyboard.KeyPgup:       {27, 91, 53, 126},
	keyboard.KeyPgdn:       {27, 91, 54, 126},
}

// stop restores the terminal settings and exits
func (s *ShellSession) Stop() {
	keyboard.Close()
}

// handleKeyboardInput handles input entered by customer on terminal
func (s *ShellSession) handleKeyboardInput() (err error) {
	var (
		character rune         //character input from keyboard
		key       keyboard.Key //special keys like arrows and function keys
	)

	charCH := make(chan rune)
	keyCH := make(chan keyboard.Key)
	go func(charCH chan rune, keyCH chan keyboard.Key) {
		if err = keyboard.Open(); err != nil {
			log.Errorf("Failed to load Keyboard: %v", err)
			return
		}
		for {
			if character, key, err = keyboard.GetKey(); err != nil {
				log.Errorf("Failed to get the key stroke: %v", err)
				return
			}
			if character != 0 {
				charCH <- character
			} else if key != 0 {
				keyCH <- key
			}
		}
	}(charCH, keyCH)

	for {
		select {
		case <-time.After(time.Second):
			if s.Session.DataChannel.IsSessionEnded() {
				s.Stop()
				return
			}
		case charStr := <-charCH:
			charBytes := []byte(string(charStr))
			if err = s.Session.DataChannel.SendInputDataMessage(message.Output, charBytes); err != nil {
				log.Errorf("Failed to send UTF8 char: %v", err)
				return
			}
		case keyStr := <-keyCH:
			keyBytes := []byte(string(keyStr))
			if byteValue, ok := specialKeysInputMap[key]; ok {
				keyBytes = byteValue
			}
			if err = s.Session.DataChannel.SendInputDataMessage(message.Output, keyBytes); err != nil {
				log.Errorf("Failed to send UTF8 char: %v", err)
				return
			}
		}
	}
}
