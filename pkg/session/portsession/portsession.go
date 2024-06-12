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
	"github.com/aws/session-manager-plugin/pkg/config"
	"github.com/aws/session-manager-plugin/pkg/jsonutil"
	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/aws/session-manager-plugin/pkg/message"
	"github.com/aws/session-manager-plugin/pkg/session"
)

const (
	LocalPortForwardingType = "LocalPortForwarding"
)

type PortSession struct {
	session.Session
	portParameters  PortParameters
	portSessionType IPortSession
}

type IPortSession interface {
	IsStreamNotSet() (status bool)
	InitializeStreams(agentVersion string) (err error)
	ReadStream() (err error)
	WriteStream(outputMessage message.ClientMessage) (err error)
	Stop()
}

type PortParameters struct {
	PortNumber          string `json:"portNumber"`
	LocalPortNumber     string `json:"localPortNumber"`
	LocalUnixSocket     string `json:"localUnixSocket"`
	LocalConnectionType string `json:"localConnectionType"`
	Type                string `json:"type"`
}

func init() {
	session.Register(&PortSession{})
}

// Name is the session name used inputStream the plugin
func (PortSession) Name() string {
	return config.PortPluginName
}

func (s *PortSession) Initialize(sessionVar *session.Session) {
	s.Session = *sessionVar
	if err := jsonutil.Remarshal(s.SessionProperties, &s.portParameters); err != nil {
		log.Errorf("Invalid format: %v", err)
	}

	if s.portParameters.Type == LocalPortForwardingType {
		s.portSessionType = &MuxPortForwarding{
			sessionId:      s.SessionId,
			portParameters: s.portParameters,
			session:        s.Session,
		}
	} else {
		s.portSessionType = &StandardStreamForwarding{
			portParameters: s.portParameters,
			session:        s.Session,
		}
	}

	s.DataChannel.RegisterOutputStreamHandler(s.ProcessStreamMessagePayload, true)
	s.DataChannel.GetWsChannel().SetOnMessage(func(input []byte) {
		if s.portSessionType.IsStreamNotSet() {
			outputMessage := &message.ClientMessage{}
			if err := outputMessage.DeserializeClientMessage(input); err != nil {
				log.Debugf("Ignore message deserialize error while stream connection had not set.")
				return
			}
			if outputMessage.MessageType == message.OutputStreamMessage {
				log.Debugf("Waiting for user to establish connection before processing incoming messages.")
				return
			} else {
				log.Infof("Received %s message while establishing connection", outputMessage.MessageType)
			}
		}
		s.DataChannel.OutputMessageHandler(s.Stop, s.SessionId, input)
	})
	log.Infof("Connected to instance[%s] on port: %s", sessionVar.TargetId, s.portParameters.PortNumber)
}

func (s *PortSession) Stop() {
	s.portSessionType.Stop()
}

// StartSession redirects inputStream/outputStream data to datachannel.
func (s *PortSession) SetSessionHandlers() (err error) {
	if err = s.portSessionType.InitializeStreams(s.DataChannel.GetAgentVersion()); err != nil {
		return err
	}

	if err = s.portSessionType.ReadStream(); err != nil {
		return err
	}
	return
}

// ProcessStreamMessagePayload writes messages received on datachannel to stdout
func (s *PortSession) ProcessStreamMessagePayload(outputMessage message.ClientMessage) (isHandlerReady bool, err error) {
	if s.portSessionType.IsStreamNotSet() {
		log.Debugf("Waiting for streams to be established before processing incoming messages.")
		return false, nil
	}
	log.Tracef("Received payload of size %d from datachannel.", outputMessage.PayloadLength)
	err = s.portSessionType.WriteStream(outputMessage)
	return true, err
}
