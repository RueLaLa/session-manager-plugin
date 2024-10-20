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

// Package portsession starts port session.
package portsession

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/aws/session-manager-plugin/pkg/config"
	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/aws/session-manager-plugin/pkg/message"
	"github.com/aws/session-manager-plugin/pkg/session"
	"github.com/aws/session-manager-plugin/pkg/session/sessionutil"
)

// BasicPortForwarding is type of port session
// accepts one client connection at a time
type BasicPortForwarding struct {
	stream         net.Conn
	listener       net.Listener
	sessionId      string
	portParameters PortParameters
	session        session.Session
}

// IsStreamNotSet checks if stream is not set
func (p *BasicPortForwarding) IsStreamNotSet() (status bool) {
	return p.stream == nil
}

// Stop closes the stream
func (p *BasicPortForwarding) Stop() {
	p.listener.Close()
	if p.stream != nil {
		p.stream.Close()
	}
}

// InitializeStreams establishes connection and initializes the stream
func (p *BasicPortForwarding) InitializeStreams(agentVersion string) (err error) {
	p.handleControlSignals()
	if err = p.startLocalConn(); err != nil {
		return
	}
	return
}

// ReadStream reads data from the stream
func (p *BasicPortForwarding) ReadStream() (err error) {
	msg := make([]byte, config.StreamDataPayloadSize)
	for {
		numBytes, err := p.stream.Read(msg)
		if err != nil {
			log.Debugf("Reading from port %s failed with error: %v. Close this connection, listen and accept new one.",
				p.portParameters.PortNumber, err)

			// Send DisconnectToPort flag to agent when client tcp connection drops to ensure agent closes tcp connection too with server port
			if err = p.session.DataChannel.SendFlag(message.DisconnectToPort); err != nil {
				log.Errorf("Failed to send packet: %v", err)
				return err
			}

			if err = p.reconnect(); err != nil {
				return err
			}

			// continue to read from connection as it has been re-established
			continue
		}

		log.Tracef("Received message of size %d from stdin.", numBytes)
		if err = p.session.DataChannel.SendInputDataMessage(message.Output, msg[:numBytes]); err != nil {
			log.Errorf("Failed to send packet: %v", err)
			return err
		}
		// Sleep to process more data
		time.Sleep(time.Millisecond)
	}
}

// WriteStream writes data to stream
func (p *BasicPortForwarding) WriteStream(outputMessage message.ClientMessage) error {
	_, err := p.stream.Write(outputMessage.Payload)
	return err
}

// startLocalConn establishes a new local connection to forward remote server packets to
func (p *BasicPortForwarding) startLocalConn() (err error) {
	// When localPortNumber is not specified, set port number to 0 to let net.conn choose an open port at random
	localPortNumber := p.portParameters.LocalPortNumber
	if p.portParameters.LocalPortNumber == "" {
		localPortNumber = "0"
	}

	if err = p.startLocalListener(localPortNumber); err != nil {
		log.Errorf("Unable to open tcp connection to port. %v", err)
		return err
	}

	if p.stream, err = p.listener.Accept(); err != nil {
		if !p.session.DataChannel.IsSessionEnded() {
			log.Errorf("Failed to accept connection with error. %v", err)
			return err
		}
	}
	if !p.session.DataChannel.IsSessionEnded() {
		log.Infof("Connection accepted for session %s.", p.sessionId)
	}

	return
}

// startLocalListener starts a local listener to given address
func (p *BasicPortForwarding) startLocalListener(portNumber string) (err error) {
	var displayMessage string
	switch p.portParameters.LocalConnectionType {
	case "unix":
		if p.listener, err = net.Listen(p.portParameters.LocalConnectionType, p.portParameters.LocalUnixSocket); err != nil {
			return
		}
		displayMessage = fmt.Sprintf("Unix socket %s opened for sessionId %s.", p.portParameters.LocalUnixSocket, p.sessionId)
	default:
		if p.listener, err = net.Listen("tcp", "localhost:"+portNumber); err != nil {
			return
		}
		// get port number the TCP listener opened
		p.portParameters.LocalPortNumber = strconv.Itoa(p.listener.Addr().(*net.TCPAddr).Port)
		displayMessage = fmt.Sprintf("Port %s opened for sessionId %s.", p.portParameters.LocalPortNumber, p.sessionId)
	}

	log.Info(displayMessage)
	return
}

// handleControlSignals handles terminate signals
func (p *BasicPortForwarding) handleControlSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, sessionutil.ControlSignals...)
	go func() {
		<-c
		log.Info("Terminate signal received, exiting.")

		p.session.DataChannel.EndSession()

		if err := p.session.DataChannel.SendFlag(message.TerminateSession); err != nil {
			log.Errorf("Failed to send TerminateSession flag: %v", err)
		}
		log.Infof("\n\nExiting session with sessionId: %s.\n\n", p.sessionId)

		p.Stop()
	}()
}

// reconnect closes existing connection, listens to new connection and accept it
func (p *BasicPortForwarding) reconnect() (err error) {
	// close existing connection as it is in a state from which data cannot be read
	p.stream.Close()

	// wait for new connection on listener and accept it
	if p.stream, err = p.listener.Accept(); err != nil {
		if !p.session.DataChannel.IsSessionEnded() {
			log.Errorf("Failed to accept connection with error. %v", err)
			return err
		}
	}

	return
}
