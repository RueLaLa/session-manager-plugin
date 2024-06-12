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

// Package session starts the session.
package session

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/aws/session-manager-plugin/pkg/config"
	"github.com/aws/session-manager-plugin/pkg/retry"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/session-manager-plugin/pkg/datachannel"
	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/aws/session-manager-plugin/pkg/message"
	"github.com/aws/session-manager-plugin/pkg/sdkutil"
	"github.com/aws/session-manager-plugin/pkg/session/sessionutil"
	"github.com/twinj/uuid"
)

const (
	LegacyArgumentLength  = 4
	ArgumentLength        = 7
	StartSessionOperation = "StartSession"
)

var SessionRegistry = map[string]ISessionPlugin{}

type ISessionPlugin interface {
	SetSessionHandlers() error
	ProcessStreamMessagePayload(streamDataMessage message.ClientMessage) (isHandlerReady bool, err error)
	Initialize(sessionVar *Session)
	Stop()
	Name() string
}

type ISession interface {
	Execute() error
	OpenDataChannel() error
	ProcessFirstMessage(outputMessage message.ClientMessage) (isHandlerReady bool, err error)
	Stop()
	GetResumeSessionParams() (string, error)
	ResumeSessionHandler() error
	TerminateSession() error
}

func init() {
	SessionRegistry = make(map[string]ISessionPlugin)
}

func Register(session ISessionPlugin) {
	SessionRegistry[session.Name()] = session
}

type Session struct {
	DataChannel           datachannel.IDataChannel
	SessionId             string
	StreamUrl             string
	TokenValue            string
	IsAwsCliUpgradeNeeded bool
	Endpoint              string
	ClientId              string
	TargetId              string
	retryParams           retry.RepeatableExponentialRetryer
	sdk                   *ssm.Client
	SessionType           string
	SessionProperties     interface{}
	DisplayMode           sessionutil.DisplayMode
}

// startSession create the datachannel for session
var startSession = func(session *Session) error {
	return session.Execute()
}

// setSessionHandlersWithSessionType set session handlers based on session subtype
var setSessionHandlersWithSessionType = func(session *Session) error {
	// SessionType is set inside DataChannel
	sessionSubType := SessionRegistry[session.SessionType]
	sessionSubType.Initialize(session)
	return sessionSubType.SetSessionHandlers()
}

// Set up a scheduler to listen on stream data resend timeout event
var handleStreamMessageResendTimeout = func(session *Session) {
	log.Tracef("Setting up scheduler to listen on IsStreamMessageResendTimeout event.")
	go func() {
		for {
			// Repeat this loop for every 200ms
			time.Sleep(config.ResendSleepInterval)
			if <-session.DataChannel.IsStreamMessageResendTimeout() {
				log.Errorf("Terminating session %s as the stream data was not processed before timeout.", session.SessionId)
				if err := session.TerminateSession(); err != nil {
					log.Errorf("Unable to terminate session upon stream data timeout. %v", err)
				}
				return
			}
		}
	}()
}

func ValidateInputAndStartSession(response, profile, ssmEndpoint, parameters string, out io.Writer) {
	var (
		err                error
		session            Session
		startSessionOutput ssm.StartSessionOutput
	)
	uuid.SwitchFormat(uuid.FormatCanonical)

	startSessionRequest := make(map[string]interface{})
	json.Unmarshal([]byte(parameters), &startSessionRequest)
	target := startSessionRequest["Target"].(string)

	sdkutil.SetProfile(profile)
	clientId := uuid.NewV4().String()

	if err = json.Unmarshal([]byte(response), &startSessionOutput); err != nil {
		log.Errorf("Cannot perform start session: %v", err)
		return
	}

	session.SessionId = *startSessionOutput.SessionId
	session.StreamUrl = *startSessionOutput.StreamUrl
	session.TokenValue = *startSessionOutput.TokenValue
	session.Endpoint = ssmEndpoint
	session.ClientId = clientId
	session.TargetId = target
	session.DataChannel = &datachannel.DataChannel{}

	if err = startSession(&session); err != nil {
		if !session.DataChannel.IsSessionEnded() {
			log.Errorf("Cannot perform start session: %v", err)
		}
		return
	}
}

// Execute create data channel and start the session
func (s *Session) Execute() (err error) {
	log.Alwaysf("Starting session with SessionId: %s\n", s.SessionId)

	// sets the display mode
	s.DisplayMode = sessionutil.NewDisplayMode()

	if err = s.OpenDataChannel(); err != nil {
		log.Errorf("Error in Opening data channel: %v", err)
		return
	}

	handleStreamMessageResendTimeout(s)

	// The session type is set either by handshake or the first packet received.
	if !<-s.DataChannel.IsSessionTypeSet() {
		log.Errorf("unable to set SessionType for session %s", s.SessionId)
		return errors.New("unable to determine SessionType")
	} else {
		s.SessionType = s.DataChannel.GetSessionType()
		s.SessionProperties = s.DataChannel.GetSessionProperties()
		if err = setSessionHandlersWithSessionType(s); err != nil {
			if !s.DataChannel.IsSessionEnded() {
				log.Errorf("Session ending with error: %v", err)
			}
			return
		}
	}

	return
}
