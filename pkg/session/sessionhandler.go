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
	"context"
	"math/rand"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/session-manager-plugin/pkg/config"
	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/aws/session-manager-plugin/pkg/message"
	"github.com/aws/session-manager-plugin/pkg/retry"
	"github.com/aws/session-manager-plugin/pkg/sdkutil"
)

// OpenDataChannel initializes datachannel
func (s *Session) OpenDataChannel() (err error) {
	s.retryParams = retry.RepeatableExponentialRetryer{
		GeometricRatio:      config.RetryBase,
		InitialDelayInMilli: rand.Intn(config.DataChannelRetryInitialDelayMillis) + config.DataChannelRetryInitialDelayMillis,
		MaxDelayInMilli:     config.DataChannelRetryMaxIntervalMillis,
		MaxAttempts:         config.DataChannelNumMaxRetries,
	}

	s.DataChannel.Initialize(s.ClientId, s.SessionId, s.TargetId, s.IsAwsCliUpgradeNeeded)
	s.DataChannel.SetWebsocket(s.StreamUrl, s.TokenValue)
	s.DataChannel.GetWsChannel().SetOnMessage(
		func(input []byte) {
			s.DataChannel.OutputMessageHandler(s.Stop, s.SessionId, input)
		})
	s.DataChannel.RegisterOutputStreamHandler(s.ProcessFirstMessage, false)

	if err = s.DataChannel.Open(); err != nil {
		log.Errorf("Retrying connection for data channel id: %s failed with error: %s", s.SessionId, err)
		s.retryParams.CallableFunc = func() (err error) { return s.DataChannel.Reconnect() }
		if err = s.retryParams.Call(); err != nil {
			log.Error(err.Error())
		}
	}

	s.DataChannel.GetWsChannel().SetOnError(
		func(err error) {
			log.Errorf("Trying to reconnect the session: %v with seq num: %d", s.StreamUrl, s.DataChannel.GetStreamDataSequenceNumber())
			s.retryParams.CallableFunc = func() (err error) { return s.ResumeSessionHandler() }
			if err = s.retryParams.Call(); err != nil {
				log.Error(err.Error())
			}
		})

	// Scheduler for resending of data
	s.DataChannel.ResendStreamDataMessageScheduler()

	return nil
}

// ProcessFirstMessage only processes messages with PayloadType Output to determine the
// sessionType of the session to be launched. This is a fallback for agent versions that do not support handshake, they
// immediately start sending shell output.
func (s *Session) ProcessFirstMessage(outputMessage message.ClientMessage) (isHandlerReady bool, err error) {
	// Immediately deregister self so that this handler is only called once, for the first message
	s.DataChannel.DeregisterOutputStreamHandler(s.ProcessFirstMessage)
	// Only set session type if the session type has not already been set. Usually session type will be set
	// by handshake protocol which would be the first message but older agents may not perform handshake
	if s.SessionType == "" {
		if outputMessage.PayloadType == uint32(message.Output) {
			log.Info("Setting session type to shell based on PayloadType!")
			s.DataChannel.SetSessionType(config.ShellPluginName)
			s.DisplayMode.DisplayMessage(outputMessage)
		}
	}
	return true, nil
}

// Stop will end the session
func (s *Session) Stop() {}

// GetResumeSessionParams calls ResumeSession API and gets tokenvalue for reconnecting
func (s *Session) GetResumeSessionParams() (string, error) {
	var (
		resumeSessionOutput *ssm.ResumeSessionOutput
		err                 error
	)

	s.sdk = ssm.NewFromConfig(sdkutil.GetSDKConfig())

	resumeSessionInput := ssm.ResumeSessionInput{
		SessionId: &s.SessionId,
	}

	log.Debugf("Resume Session input parameters: %v", resumeSessionInput)
	if resumeSessionOutput, err = s.sdk.ResumeSession(context.TODO(), &resumeSessionInput); err != nil {
		log.Errorf("Resume Session failed: %v", err)
		return "", err
	}

	if resumeSessionOutput.TokenValue == nil {
		return "", nil
	}

	return *resumeSessionOutput.TokenValue, nil
}

// ResumeSessionHandler gets token value and tries to Reconnect to datachannel
func (s *Session) ResumeSessionHandler() (err error) {
	s.TokenValue, err = s.GetResumeSessionParams()
	if err != nil {
		log.Errorf("Failed to get token: %v", err)
		return
	} else if s.TokenValue == "" {
		log.Debugf("Session: %s timed out", s.SessionId)
		return
	}
	s.DataChannel.GetWsChannel().SetChannelToken(s.TokenValue)
	err = s.DataChannel.Reconnect()
	return
}

// TerminateSession calls TerminateSession API
func (s *Session) TerminateSession() error {
	var (
		err error
	)

	s.sdk = ssm.NewFromConfig(sdkutil.GetSDKConfig())

	terminateSessionInput := ssm.TerminateSessionInput{
		SessionId: &s.SessionId,
	}

	log.Debugf("Terminate Session input parameters: %v", terminateSessionInput)
	if _, err = s.sdk.TerminateSession(context.TODO(), &terminateSessionInput); err != nil {
		log.Errorf("Terminate Session failed: %v", err)
		return err
	}
	return nil
}
