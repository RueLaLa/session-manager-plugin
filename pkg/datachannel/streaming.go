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

// datachannel package implement data channel for interactive sessions.
package datachannel

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/session-manager-plugin/pkg/communicator"
	"github.com/aws/session-manager-plugin/pkg/config"
	"github.com/aws/session-manager-plugin/pkg/encryption"
	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/aws/session-manager-plugin/pkg/message"
	"github.com/aws/session-manager-plugin/pkg/service"
	"github.com/aws/session-manager-plugin/pkg/version"
	"github.com/gorilla/websocket"
	"github.com/twinj/uuid"
)

type IDataChannel interface {
	Initialize(clientId string, sessionId string, targetId string, isAwsCliUpgradeNeeded bool)
	SetWebsocket(streamUrl string, tokenValue string)
	Reconnect() error
	SendFlag(flagType message.PayloadTypeFlag) error
	Open() error
	Close() error
	FinalizeDataChannelHandshake(tokenValue string) error
	SendInputDataMessage(payloadType message.PayloadType, inputData []byte) error
	ResendStreamDataMessageScheduler() error
	ProcessAcknowledgedMessage(acknowledgeMessageContent message.AcknowledgeContent) error
	OutputMessageHandler(stopHandler Stop, sessionID string, rawMessage []byte) error
	SendAcknowledgeMessage(clientMessage message.ClientMessage) error
	AddDataToOutgoingMessageBuffer(streamMessage StreamingMessage)
	RemoveDataFromOutgoingMessageBuffer(streamMessageElement *list.Element)
	AddDataToIncomingMessageBuffer(streamMessage StreamingMessage)
	RemoveDataFromIncomingMessageBuffer(sequenceNumber int64)
	CalculateRetransmissionTimeout(streamingMessage StreamingMessage)
	SendMessage(input []byte, inputType int) error
	RegisterOutputStreamHandler(handler OutputStreamDataMessageHandler, isSessionSpecificHandler bool)
	DeregisterOutputStreamHandler(handler OutputStreamDataMessageHandler)
	IsSessionTypeSet() chan bool
	EndSession() error
	IsSessionEnded() bool
	IsStreamMessageResendTimeout() chan bool
	GetSessionType() string
	SetSessionType(sessionType string)
	GetSessionProperties() interface{}
	GetWsChannel() communicator.IWebSocketChannel
	SetWsChannel(wsChannel communicator.IWebSocketChannel)
	GetStreamDataSequenceNumber() int64
	GetAgentVersion() string
	SetAgentVersion(agentVersion string)
}

// DataChannel used for communication between the mgs and the cli.
type DataChannel struct {
	wsChannel             communicator.IWebSocketChannel
	Role                  string
	ClientId              string
	SessionId             string
	TargetId              string
	IsAwsCliUpgradeNeeded bool
	//records sequence number of last acknowledged message received over data channel
	ExpectedSequenceNumber int64
	//records sequence number of last stream data message sent over data channel
	StreamDataSequenceNumber int64
	//buffer to store outgoing stream messages until acknowledged
	//using linked list for this buffer as access to oldest message is required and it support faster deletion from any position of list
	OutgoingMessageBuffer ListMessageBuffer
	//buffer to store incoming stream messages if received out of sequence
	//using map for this buffer as incoming messages can be out of order and retrieval would be faster by sequenceId
	IncomingMessageBuffer MapMessageBuffer
	//round trip time of latest acknowledged message
	RoundTripTime float64
	//round trip time variation of latest acknowledged message
	RoundTripTimeVariation float64
	//timeout used for resending unacknowledged message
	RetransmissionTimeout time.Duration
	// Encrypter to encrypt/decrypt if agent requests encryption
	encryption        encryption.IEncrypter
	encryptionEnabled bool

	// SessionType
	sessionType       string
	isSessionTypeSet  chan bool
	sessionProperties interface{}

	isSessionEnded bool

	// Used to detect if resending a streaming message reaches timeout
	isStreamMessageResendTimeout chan bool

	// Handles data on output stream. Output stream is data outputted by the SSM agent and received here.
	outputStreamHandlers        []OutputStreamDataMessageHandler
	isSessionSpecificHandlerSet bool

	// AgentVersion received during handshake
	agentVersion string
}

type ListMessageBuffer struct {
	Messages *list.List
	Capacity int
	Mutex    *sync.Mutex
}

type MapMessageBuffer struct {
	Messages map[int64]StreamingMessage
	Capacity int
	Mutex    *sync.Mutex
}

type StreamingMessage struct {
	Content        []byte
	SequenceNumber int64
	LastSentTime   time.Time
	ResendAttempt  *int
}

type OutputStreamDataMessageHandler func(streamDataMessage message.ClientMessage) (bool, error)

type Stop func()

var SendAcknowledgeMessageCall = func(dataChannel *DataChannel, streamDataMessage message.ClientMessage) error {
	return dataChannel.SendAcknowledgeMessage(streamDataMessage)
}

var ProcessAcknowledgedMessageCall = func(dataChannel *DataChannel, acknowledgeMessage message.AcknowledgeContent) error {
	return dataChannel.ProcessAcknowledgedMessage(acknowledgeMessage)
}

var SendMessageCall = func(dataChannel *DataChannel, input []byte, inputType int) error {
	return dataChannel.SendMessage(input, inputType)
}

var GetRoundTripTime = func(streamingMessage StreamingMessage) time.Duration {
	return time.Since(streamingMessage.LastSentTime)
}

var newEncrypter = func(kmsKeyId string, encryptionConext map[string]string) (encryption.IEncrypter, error) {
	return encryption.NewEncrypter(kmsKeyId, encryptionConext)
}

// Initialize populates the data channel object with the correct values.
func (dataChannel *DataChannel) Initialize(clientId string, sessionId string, targetId string, isAwsCliUpgradeNeeded bool) {
	//open data channel as publish_subscribe
	log.Debugf("Calling Initialize Datachannel for role: %s", config.RolePublishSubscribe)

	dataChannel.Role = config.RolePublishSubscribe
	dataChannel.ClientId = clientId
	dataChannel.SessionId = sessionId
	dataChannel.TargetId = targetId
	dataChannel.ExpectedSequenceNumber = 0
	dataChannel.StreamDataSequenceNumber = 0
	dataChannel.OutgoingMessageBuffer = ListMessageBuffer{
		list.New(),
		config.OutgoingMessageBufferCapacity,
		&sync.Mutex{},
	}
	dataChannel.IncomingMessageBuffer = MapMessageBuffer{
		make(map[int64]StreamingMessage),
		config.IncomingMessageBufferCapacity,
		&sync.Mutex{},
	}
	dataChannel.RoundTripTime = float64(config.DefaultRoundTripTime)
	dataChannel.RoundTripTimeVariation = config.DefaultRoundTripTimeVariation
	dataChannel.RetransmissionTimeout = config.DefaultTransmissionTimeout
	dataChannel.wsChannel = &communicator.WebSocketChannel{}
	dataChannel.encryptionEnabled = false
	dataChannel.isSessionTypeSet = make(chan bool, 1)
	dataChannel.isSessionEnded = false
	dataChannel.isStreamMessageResendTimeout = make(chan bool, 1)
	dataChannel.sessionType = ""
	dataChannel.IsAwsCliUpgradeNeeded = isAwsCliUpgradeNeeded
}

// SetWebsocket function populates websocket channel object
func (dataChannel *DataChannel) SetWebsocket(channelUrl string, channelToken string) {
	dataChannel.wsChannel.Initialize(channelUrl, channelToken)
}

// FinalizeHandshake sends the token for service to acknowledge the connection.
func (dataChannel *DataChannel) FinalizeDataChannelHandshake(tokenValue string) (err error) {
	uuid.SwitchFormat(uuid.FormatCanonical)
	uid := uuid.NewV4().String()

	log.Infof("Sending token through data channel %s to acknowledge connection", dataChannel.wsChannel.GetStreamUrl())
	openDataChannelInput := service.OpenDataChannelInput{
		MessageSchemaVersion: aws.String(config.MessageSchemaVersion),
		RequestId:            aws.String(uid),
		TokenValue:           aws.String(tokenValue),
		ClientId:             aws.String(dataChannel.ClientId),
		ClientVersion:        aws.String(version.Version),
	}

	var openDataChannelInputBytes []byte

	if openDataChannelInputBytes, err = json.Marshal(openDataChannelInput); err != nil {
		log.Errorf("Error serializing openDataChannelInput: %s", err)
		return
	}
	return dataChannel.SendMessage(openDataChannelInputBytes, websocket.TextMessage)
}

// SendMessage sends a message to the service through datachannel
func (dataChannel *DataChannel) SendMessage(input []byte, inputType int) error {
	return dataChannel.wsChannel.SendMessage(input, inputType)
}

// Open opens websocket connects and does final handshake to acknowledge connection
func (dataChannel *DataChannel) Open() (err error) {
	if err = dataChannel.wsChannel.Open(); err != nil {
		return fmt.Errorf("failed to open data channel with error: %v", err)
	}

	if err = dataChannel.FinalizeDataChannelHandshake(dataChannel.wsChannel.GetChannelToken()); err != nil {
		return fmt.Errorf("error sending token for handshake: %v", err)
	}
	return
}

// Close closes datachannel - its web socket connection
func (dataChannel *DataChannel) Close() error {
	log.Infof("Closing datachannel with url %s", dataChannel.wsChannel.GetStreamUrl())
	return dataChannel.wsChannel.Close()
}

// Reconnect calls ResumeSession API to reconnect datachannel when connection is lost
func (dataChannel *DataChannel) Reconnect() (err error) {

	if err = dataChannel.Close(); err != nil {
		log.Debugf("Closing datachannel failed with error: %v", err)
	}

	if err = dataChannel.Open(); err != nil {
		return fmt.Errorf("failed to reconnect data channel %s with error: %v", dataChannel.wsChannel.GetStreamUrl(), err)
	}

	log.Infof("Successfully reconnected to data channel: %s", dataChannel.wsChannel.GetStreamUrl())
	return
}

// SendFlag sends a data message with PayloadType as given flag.
func (dataChannel *DataChannel) SendFlag(
	flagType message.PayloadTypeFlag) (err error) {

	flagBuf := new(bytes.Buffer)
	binary.Write(flagBuf, binary.BigEndian, flagType)
	return dataChannel.SendInputDataMessage(message.Flag, flagBuf.Bytes())
}

// SendInputDataMessage sends a data message in a form of ClientMessage.
func (dataChannel *DataChannel) SendInputDataMessage(
	payloadType message.PayloadType,
	inputData []byte) (err error) {

	var (
		flag uint64 = 0
		msg  []byte
	)

	messageId := uuid.NewV4()

	// today 'enter' is taken as 'next line' in winpty shell. so hardcoding 'next line' byte to actual 'enter' byte
	if bytes.Equal(inputData, []byte{10}) {
		inputData = []byte{13}
	}

	// Encrypt if encryption is enabled and payload type is Output
	if dataChannel.encryptionEnabled && payloadType == message.Output {
		inputData, err = dataChannel.encryption.Encrypt(inputData)
		if err != nil {
			return err
		}
	}

	clientMessage := message.ClientMessage{
		MessageType:    message.InputStreamMessage,
		SchemaVersion:  1,
		CreatedDate:    uint64(time.Now().UnixNano() / 1000000),
		Flags:          flag,
		MessageId:      messageId,
		PayloadType:    uint32(payloadType),
		Payload:        inputData,
		SequenceNumber: dataChannel.StreamDataSequenceNumber,
	}

	if msg, err = clientMessage.SerializeClientMessage(); err != nil {
		log.Errorf("Cannot serialize StreamData message with error: %v", err)
		return
	}

	log.Tracef("Sending message with seq number: %d", dataChannel.StreamDataSequenceNumber)
	if err = SendMessageCall(dataChannel, msg, websocket.BinaryMessage); err != nil {
		log.Errorf("Error sending stream data message %v", err)
		return
	}

	streamingMessage := StreamingMessage{
		msg,
		dataChannel.StreamDataSequenceNumber,
		time.Now(),
		new(int),
	}
	dataChannel.AddDataToOutgoingMessageBuffer(streamingMessage)
	dataChannel.StreamDataSequenceNumber = dataChannel.StreamDataSequenceNumber + 1

	return
}

// ResendStreamDataMessageScheduler spawns a separate go thread which keeps checking OutgoingMessageBuffer at fixed interval
// and resends first message if time elapsed since lastSentTime of the message is more than acknowledge wait time
func (dataChannel *DataChannel) ResendStreamDataMessageScheduler() (err error) {
	go func() {
		for {
			time.Sleep(config.ResendSleepInterval)
			dataChannel.OutgoingMessageBuffer.Mutex.Lock()
			streamMessageElement := dataChannel.OutgoingMessageBuffer.Messages.Front()
			dataChannel.OutgoingMessageBuffer.Mutex.Unlock()

			if streamMessageElement == nil {
				continue
			}

			streamMessage := streamMessageElement.Value.(StreamingMessage)
			if time.Since(streamMessage.LastSentTime) > dataChannel.RetransmissionTimeout {
				log.Debugf("Resend stream data message %d for the %d attempt.", streamMessage.SequenceNumber, *streamMessage.ResendAttempt)
				if *streamMessage.ResendAttempt >= config.ResendMaxAttempt {
					log.Warnf("Message %d was resent over %d times.", streamMessage.SequenceNumber, config.ResendMaxAttempt)
					dataChannel.isStreamMessageResendTimeout <- true
				}
				*streamMessage.ResendAttempt++
				if err = SendMessageCall(dataChannel, streamMessage.Content, websocket.BinaryMessage); err != nil {
					log.Errorf("Unable to send stream data message: %s", err)
				}
				streamMessage.LastSentTime = time.Now()
			}
		}
	}()

	return
}

// ProcessAcknowledgedMessage processes acknowledge messages by deleting them from OutgoingMessageBuffer
func (dataChannel *DataChannel) ProcessAcknowledgedMessage(acknowledgeMessageContent message.AcknowledgeContent) error {
	acknowledgeSequenceNumber := acknowledgeMessageContent.SequenceNumber
	for streamMessageElement := dataChannel.OutgoingMessageBuffer.Messages.Front(); streamMessageElement != nil; streamMessageElement = streamMessageElement.Next() {
		streamMessage := streamMessageElement.Value.(StreamingMessage)
		if streamMessage.SequenceNumber == acknowledgeSequenceNumber {

			//Calculate retransmission timeout based on latest round trip time of message
			dataChannel.CalculateRetransmissionTimeout(streamMessage)

			dataChannel.RemoveDataFromOutgoingMessageBuffer(streamMessageElement)
			break
		}
	}
	return nil
}

// SendAcknowledgeMessage sends acknowledge message for stream data over data channel
func (dataChannel *DataChannel) SendAcknowledgeMessage(streamDataMessage message.ClientMessage) (err error) {
	dataStreamAcknowledgeContent := message.AcknowledgeContent{
		MessageType:         streamDataMessage.MessageType,
		MessageId:           streamDataMessage.MessageId.String(),
		SequenceNumber:      streamDataMessage.SequenceNumber,
		IsSequentialMessage: true,
	}

	var msg []byte
	if msg, err = message.SerializeClientMessageWithAcknowledgeContent(dataStreamAcknowledgeContent); err != nil {
		log.Errorf("Cannot serialize Acknowledge message err: %v", err)
		return
	}

	if err = SendMessageCall(dataChannel, msg, websocket.BinaryMessage); err != nil {
		log.Errorf("Error sending acknowledge message %v", err)
		return
	}
	return
}

// OutputMessageHandler gets output on the data channel
func (dataChannel *DataChannel) OutputMessageHandler(stopHandler Stop, sessionID string, rawMessage []byte) error {
	outputMessage := &message.ClientMessage{}
	err := outputMessage.DeserializeClientMessage(rawMessage)
	if err != nil {
		log.Errorf("Cannot deserialize raw message: %s, err: %v.", string(rawMessage), err)
		return err
	}
	if err = outputMessage.Validate(); err != nil {
		log.Errorf("Invalid outputMessage: %v, err: %v.", *outputMessage, err)
		return err
	}

	log.Tracef("Processing stream data message of type: %s", outputMessage.MessageType)
	switch outputMessage.MessageType {
	case message.OutputStreamMessage:
		return dataChannel.HandleOutputMessage(*outputMessage, rawMessage)
	case message.AcknowledgeMessage:
		return dataChannel.HandleAcknowledgeMessage(*outputMessage)
	case message.ChannelClosedMessage:
		dataChannel.HandleChannelClosedMessage(stopHandler, sessionID, *outputMessage)
	case message.StartPublicationMessage, message.PausePublicationMessage:
		return nil
	default:
		log.Warnf("Invalid message type received: %s", outputMessage.MessageType)
	}

	return nil
}

// handleHandshakeRequest is the handler for payloads of type HandshakeRequest
func (dataChannel *DataChannel) handleHandshakeRequest(clientMessage message.ClientMessage) error {

	handshakeRequest, err := clientMessage.DeserializeHandshakeRequest()
	if err != nil {
		log.Errorf("Deserialize Handshake Request failed: %s", err)
		return err
	}

	dataChannel.agentVersion = handshakeRequest.AgentVersion

	var errorList []error
	var handshakeResponse message.HandshakeResponsePayload
	handshakeResponse.ClientVersion = version.Version
	handshakeResponse.ProcessedClientActions = []message.ProcessedClientAction{}
	for _, action := range handshakeRequest.RequestedClientActions {
		processedAction := message.ProcessedClientAction{}
		switch action.ActionType {
		case message.KMSEncryption:
			processedAction.ActionType = action.ActionType
			err := dataChannel.ProcessKMSEncryptionHandshakeAction(action.ActionParameters)
			if err != nil {
				processedAction.ActionStatus = message.Failed
				processedAction.Error = fmt.Sprintf("Failed to process action %s: %s",
					message.KMSEncryption, err)
				errorList = append(errorList, err)
			} else {
				processedAction.ActionStatus = message.Success
				processedAction.ActionResult = message.KMSEncryptionResponse{
					KMSCipherTextKey: dataChannel.encryption.GetEncryptedDataKey(),
				}
				dataChannel.encryptionEnabled = true
			}
		case message.SessionType:
			processedAction.ActionType = action.ActionType
			err := dataChannel.ProcessSessionTypeHandshakeAction(action.ActionParameters)
			if err != nil {
				processedAction.ActionStatus = message.Failed
				processedAction.Error = fmt.Sprintf("Failed to process action %s: %s",
					message.SessionType, err)
				errorList = append(errorList, err)
			} else {
				processedAction.ActionStatus = message.Success
			}

		default:
			processedAction.ActionType = action.ActionType
			processedAction.ActionResult = message.Unsupported
			processedAction.Error = fmt.Sprintf("Unsupported action %s", action.ActionType)
			errorList = append(errorList, errors.New(processedAction.Error))
		}
		handshakeResponse.ProcessedClientActions = append(handshakeResponse.ProcessedClientActions, processedAction)
	}
	for _, x := range errorList {
		handshakeResponse.Errors = append(handshakeResponse.Errors, x.Error())
	}
	err = dataChannel.sendHandshakeResponse(handshakeResponse)
	return err
}

// handleHandshakeComplete is the handler for when the payload type is HandshakeComplete. This will trigger
// the plugin to start.
func (dataChannel *DataChannel) handleHandshakeComplete(clientMessage message.ClientMessage) error {
	var err error
	var handshakeComplete message.HandshakeCompletePayload
	handshakeComplete, err = clientMessage.DeserializeHandshakeComplete()
	if err != nil {
		return err
	}

	// SessionType would be set when handshake request is received
	if dataChannel.sessionType != "" {
		dataChannel.isSessionTypeSet <- true
	} else {
		dataChannel.isSessionTypeSet <- false
	}

	log.Debugf("Handshake Complete. Handshake time to complete is: %f seconds",
		handshakeComplete.HandshakeTimeToComplete.Seconds())

	if handshakeComplete.CustomerMessage != "" {
		log.Debug(handshakeComplete.CustomerMessage)
	}

	return err
}

// handleEncryptionChallengeRequest receives EncryptionChallenge and responds.
func (dataChannel *DataChannel) handleEncryptionChallengeRequest(clientMessage message.ClientMessage) error {
	var err error
	var encChallengeReq message.EncryptionChallengeRequest
	err = json.Unmarshal(clientMessage.Payload, &encChallengeReq)
	if err != nil {
		return fmt.Errorf("could not deserialize rawMessage, %s : %s", clientMessage.Payload, err)
	}
	challenge := encChallengeReq.Challenge
	challenge, err = dataChannel.encryption.Decrypt(challenge)
	if err != nil {
		return err
	}
	challenge, err = dataChannel.encryption.Encrypt(challenge)
	if err != nil {
		return err
	}
	encChallengeResp := message.EncryptionChallengeResponse{
		Challenge: challenge,
	}

	err = dataChannel.sendEncryptionChallengeResponse(encChallengeResp)
	return err
}

// sendEncryptionChallengeResponse sends EncryptionChallengeResponse
func (dataChannel *DataChannel) sendEncryptionChallengeResponse(response message.EncryptionChallengeResponse) error {
	var resultBytes, err = json.Marshal(response)
	if err != nil {
		return fmt.Errorf("could not serialize EncChallengeResponse message: %v, err: %s", response, err)
	}

	log.Tracef("Sending EncChallengeResponse message.")
	if err := dataChannel.SendInputDataMessage(message.EncChallengeResponse, resultBytes); err != nil {
		return err
	}
	return nil

}

// sendHandshakeResponse sends HandshakeResponse
func (dataChannel *DataChannel) sendHandshakeResponse(response message.HandshakeResponsePayload) error {

	var resultBytes, err = json.Marshal(response)
	if err != nil {
		log.Errorf("Could not serialize HandshakeResponse message: %v, err: %s", response, err)
	}

	log.Tracef("Sending HandshakeResponse message.")
	if err := dataChannel.SendInputDataMessage(message.HandshakeResponsePayloadType, resultBytes); err != nil {
		return err
	}
	return nil
}

// RegisterOutputStreamHandler register a handler for messages of type OutputStream. This is usually called by the plugin.
func (dataChannel *DataChannel) RegisterOutputStreamHandler(handler OutputStreamDataMessageHandler, isSessionSpecificHandler bool) {
	dataChannel.isSessionSpecificHandlerSet = isSessionSpecificHandler
	dataChannel.outputStreamHandlers = append(dataChannel.outputStreamHandlers, handler)
}

// DeregisterOutputStreamHandler deregisters a handler previously registered using RegisterOutputStreamHandler
func (dataChannel *DataChannel) DeregisterOutputStreamHandler(handler OutputStreamDataMessageHandler) {
	// Find and remove "handler"
	for i, v := range dataChannel.outputStreamHandlers {
		if reflect.ValueOf(v).Pointer() == reflect.ValueOf(handler).Pointer() {
			dataChannel.outputStreamHandlers = append(dataChannel.outputStreamHandlers[:i], dataChannel.outputStreamHandlers[i+1:]...)
			break
		}
	}
}

func (dataChannel *DataChannel) processOutputMessageWithHandlers(message message.ClientMessage) (isHandlerReady bool, err error) {
	// Return false if sessionType is known but session specific handler is not set
	if dataChannel.sessionType != "" && !dataChannel.isSessionSpecificHandlerSet {
		return false, nil
	}
	for _, handler := range dataChannel.outputStreamHandlers {
		isHandlerReady, err = handler(message)
		// Break the processing of message and return if session specific handler is not ready
		if err != nil || !isHandlerReady {
			break
		}
	}
	return isHandlerReady, err
}

// handleOutputMessage handles incoming stream data message by processing the payload and updating expectedSequenceNumber
func (dataChannel *DataChannel) HandleOutputMessage(
	outputMessage message.ClientMessage,
	rawMessage []byte) (err error) {

	// On receiving expected stream data message, send acknowledgement, process it and increment expected sequence number by 1.
	// Further process messages from IncomingMessageBuffer
	if outputMessage.SequenceNumber == dataChannel.ExpectedSequenceNumber {

		switch message.PayloadType(outputMessage.PayloadType) {
		case message.HandshakeRequestPayloadType:
			{
				if err = SendAcknowledgeMessageCall(dataChannel, outputMessage); err != nil {
					return err
				}

				// PayloadType is HandshakeRequest so we call our own handler instead of the provided handler
				log.Debugf("Processing HandshakeRequest message %v", outputMessage)
				if err = dataChannel.handleHandshakeRequest(outputMessage); err != nil {
					log.Errorf("Unable to process incoming data payload, MessageType %s, "+
						"PayloadType HandshakeRequestPayloadType, err: %s.", outputMessage.MessageType, err)
					return err
				}
			}
		case message.HandshakeCompletePayloadType:
			{
				if err = SendAcknowledgeMessageCall(dataChannel, outputMessage); err != nil {
					return err
				}

				if err = dataChannel.handleHandshakeComplete(outputMessage); err != nil {
					log.Errorf("Unable to process incoming data payload, MessageType %s, "+
						"PayloadType HandshakeCompletePayloadType, err: %s.", outputMessage.MessageType, err)
					return err
				}
			}
		case message.EncChallengeRequest:
			{
				if err = SendAcknowledgeMessageCall(dataChannel, outputMessage); err != nil {
					return err
				}

				if err = dataChannel.handleEncryptionChallengeRequest(outputMessage); err != nil {
					log.Errorf("Unable to process incoming data payload, MessageType %s, "+
						"PayloadType EncChallengeRequest, err: %s.", outputMessage.MessageType, err)
					return err
				}
			}
		default:

			log.Tracef("Process new incoming stream data message. Sequence Number: %d", outputMessage.SequenceNumber)

			// Decrypt if encryption is enabled and payload type is output
			if dataChannel.encryptionEnabled &&
				(outputMessage.PayloadType == uint32(message.Output) ||
					outputMessage.PayloadType == uint32(message.StdErr) ||
					outputMessage.PayloadType == uint32(message.ExitCode)) {
				outputMessage.Payload, err = dataChannel.encryption.Decrypt(outputMessage.Payload)
				if err != nil {
					log.Errorf("Unable to decrypt incoming data payload, MessageType %s, "+
						"PayloadType %d, err: %s.", outputMessage.MessageType, outputMessage.PayloadType, err)
					return err
				}
			}

			isHandlerReady, err := dataChannel.processOutputMessageWithHandlers(outputMessage)
			if err != nil {
				log.Errorf("Failed to process stream data message: %s", err.Error())
				return err
			}
			if !isHandlerReady {
				log.Warnf("Stream data message with sequence number %d is not processed as session handler is not ready.", outputMessage.SequenceNumber)
				return nil
			} else {
				// Acknowledge outputMessage only if session specific handler is ready
				if err := SendAcknowledgeMessageCall(dataChannel, outputMessage); err != nil {
					return err
				}
			}
		}
		dataChannel.ExpectedSequenceNumber = dataChannel.ExpectedSequenceNumber + 1
		return dataChannel.ProcessIncomingMessageBufferItems(outputMessage)
	} else {
		log.Debugf("Unexpected sequence message received. Received Sequence Number: %d. Expected Sequence Number: %d",
			outputMessage.SequenceNumber, dataChannel.ExpectedSequenceNumber)

		// If incoming message sequence number is greater then expected sequence number and IncomingMessageBuffer has capacity,
		// add message to IncomingMessageBuffer and send acknowledgement
		if outputMessage.SequenceNumber > dataChannel.ExpectedSequenceNumber {
			log.Debugf("Received Sequence Number %d is higher than Expected Sequence Number %d, adding to IncomingMessageBuffer",
				outputMessage.SequenceNumber, dataChannel.ExpectedSequenceNumber)
			if len(dataChannel.IncomingMessageBuffer.Messages) < dataChannel.IncomingMessageBuffer.Capacity {
				if err = SendAcknowledgeMessageCall(dataChannel, outputMessage); err != nil {
					return err
				}

				streamingMessage := StreamingMessage{
					rawMessage,
					outputMessage.SequenceNumber,
					time.Now(),
					new(int),
				}

				//Add message to buffer for future processing
				dataChannel.AddDataToIncomingMessageBuffer(streamingMessage)
			}
		}
	}
	return nil
}

// processIncomingMessageBufferItems check if new expected sequence stream data is present in IncomingMessageBuffer.
// If so process it and increment expected sequence number.
// Repeat until expected sequence stream data is not found in IncomingMessageBuffer.
func (dataChannel *DataChannel) ProcessIncomingMessageBufferItems(
	outputMessage message.ClientMessage) (err error) {

	for {
		bufferedStreamMessage := dataChannel.IncomingMessageBuffer.Messages[dataChannel.ExpectedSequenceNumber]
		if bufferedStreamMessage.Content != nil {
			log.Debugf("Process stream data message from IncomingMessageBuffer. "+
				"Sequence Number: %d", bufferedStreamMessage.SequenceNumber)

			if err := outputMessage.DeserializeClientMessage(bufferedStreamMessage.Content); err != nil {
				log.Errorf("Cannot deserialize raw message with err: %v.", err)
				return err
			}

			// Decrypt if encryption is enabled and payload type is output
			if dataChannel.encryptionEnabled &&
				(outputMessage.PayloadType == uint32(message.Output) ||
					outputMessage.PayloadType == uint32(message.StdErr) ||
					outputMessage.PayloadType == uint32(message.ExitCode)) {
				outputMessage.Payload, err = dataChannel.encryption.Decrypt(outputMessage.Payload)
				if err != nil {
					log.Errorf("Unable to decrypt buffered message data payload, MessageType %s, "+
						"PayloadType %d, err: %s.", outputMessage.MessageType, outputMessage.PayloadType, err)
					return err
				}
			}

			dataChannel.processOutputMessageWithHandlers(outputMessage)

			dataChannel.ExpectedSequenceNumber = dataChannel.ExpectedSequenceNumber + 1
			dataChannel.RemoveDataFromIncomingMessageBuffer(bufferedStreamMessage.SequenceNumber)
		} else {
			break
		}
	}
	return
}

// handleAcknowledgeMessage deserialize acknowledge content and process it
func (dataChannel *DataChannel) HandleAcknowledgeMessage(
	outputMessage message.ClientMessage) (err error) {

	var acknowledgeMessage message.AcknowledgeContent
	if acknowledgeMessage, err = outputMessage.DeserializeDataStreamAcknowledgeContent(); err != nil {
		log.Errorf("Cannot deserialize payload to AcknowledgeMessage with error: %v.", err)
		return err
	}

	err = ProcessAcknowledgedMessageCall(dataChannel, acknowledgeMessage)
	return err
}

// handleChannelClosedMessage exits the shell
func (dataChannel *DataChannel) HandleChannelClosedMessage(stopHandler Stop, sessionId string, outputMessage message.ClientMessage) {
	var (
		channelClosedMessage message.ChannelClosed
		err                  error
	)
	if channelClosedMessage, err = outputMessage.DeserializeChannelClosedMessage(); err != nil {
		log.Errorf("Cannot deserialize payload to ChannelClosedMessage: %v.", err)
	}

	if channelClosedMessage.Output == "" {
		log.Alwaysf("Exiting session with sessionId: %s.", sessionId)
	} else {
		log.Alwaysf("SessionId: %s : %s", sessionId, channelClosedMessage.Output)
	}
	dataChannel.EndSession()
	dataChannel.Close()

	stopHandler()
}

// AddDataToOutgoingMessageBuffer removes first message from OutgoingMessageBuffer if capacity is full and adds given message at the end
func (dataChannel *DataChannel) AddDataToOutgoingMessageBuffer(streamMessage StreamingMessage) {
	if dataChannel.OutgoingMessageBuffer.Messages.Len() == dataChannel.OutgoingMessageBuffer.Capacity {
		dataChannel.RemoveDataFromOutgoingMessageBuffer(dataChannel.OutgoingMessageBuffer.Messages.Front())
	}
	dataChannel.OutgoingMessageBuffer.Mutex.Lock()
	dataChannel.OutgoingMessageBuffer.Messages.PushBack(streamMessage)
	dataChannel.OutgoingMessageBuffer.Mutex.Unlock()
}

// RemoveDataFromOutgoingMessageBuffer removes given element from OutgoingMessageBuffer
func (dataChannel *DataChannel) RemoveDataFromOutgoingMessageBuffer(streamMessageElement *list.Element) {
	dataChannel.OutgoingMessageBuffer.Mutex.Lock()
	dataChannel.OutgoingMessageBuffer.Messages.Remove(streamMessageElement)
	dataChannel.OutgoingMessageBuffer.Mutex.Unlock()
}

// AddDataToIncomingMessageBuffer adds given message to IncomingMessageBuffer if it has capacity
func (dataChannel *DataChannel) AddDataToIncomingMessageBuffer(streamMessage StreamingMessage) {
	if len(dataChannel.IncomingMessageBuffer.Messages) == dataChannel.IncomingMessageBuffer.Capacity {
		return
	}
	dataChannel.IncomingMessageBuffer.Mutex.Lock()
	dataChannel.IncomingMessageBuffer.Messages[streamMessage.SequenceNumber] = streamMessage
	dataChannel.IncomingMessageBuffer.Mutex.Unlock()
}

// RemoveDataFromIncomingMessageBuffer removes given sequence number message from IncomingMessageBuffer
func (dataChannel *DataChannel) RemoveDataFromIncomingMessageBuffer(sequenceNumber int64) {
	dataChannel.IncomingMessageBuffer.Mutex.Lock()
	delete(dataChannel.IncomingMessageBuffer.Messages, sequenceNumber)
	dataChannel.IncomingMessageBuffer.Mutex.Unlock()
}

// CalculateRetransmissionTimeout calculates message retransmission timeout value based on round trip time on given message
func (dataChannel *DataChannel) CalculateRetransmissionTimeout(streamingMessage StreamingMessage) {
	newRoundTripTime := float64(GetRoundTripTime(streamingMessage))

	dataChannel.RoundTripTimeVariation = ((1 - config.RTTVConstant) * dataChannel.RoundTripTimeVariation) +
		(config.RTTVConstant * math.Abs(dataChannel.RoundTripTime-newRoundTripTime))

	dataChannel.RoundTripTime = ((1 - config.RTTConstant) * dataChannel.RoundTripTime) +
		(config.RTTConstant * newRoundTripTime)

	dataChannel.RetransmissionTimeout = time.Duration(dataChannel.RoundTripTime +
		math.Max(float64(config.ClockGranularity), float64(4*dataChannel.RoundTripTimeVariation)))

	// Ensure RetransmissionTimeout do not exceed maximum timeout defined
	if dataChannel.RetransmissionTimeout > config.MaxTransmissionTimeout {
		dataChannel.RetransmissionTimeout = config.MaxTransmissionTimeout
	}
}

// ProcessKMSEncryptionHandshakeAction sets up the encrypter and calls KMS to generate a new data key. This is triggered
// when encryption is specified in HandshakeRequest
func (dataChannel *DataChannel) ProcessKMSEncryptionHandshakeAction(actionParams json.RawMessage) (err error) {

	if dataChannel.IsAwsCliUpgradeNeeded {
		return errors.New("installed version of CLI does not support Session Manager encryption feature. Please upgrade to the latest version of your CLI (e.g., AWS CLI)")
	}
	kmsEncRequest := message.KMSEncryptionRequest{}
	json.Unmarshal(actionParams, &kmsEncRequest)
	kmsKeyId := kmsEncRequest.KMSKeyID

	encryptionContext := map[string]string{"aws:ssm:SessionId": dataChannel.SessionId, "aws:ssm:TargetId": dataChannel.TargetId}
	dataChannel.encryption, err = newEncrypter(kmsKeyId, encryptionContext)
	return
}

// ProcessSessionTypeHandshakeAction processes session type action in HandshakeRequest. This sets the session type in the datachannel.
func (dataChannel *DataChannel) ProcessSessionTypeHandshakeAction(actionParams json.RawMessage) (err error) {
	sessTypeReq := message.SessionTypeRequest{}
	json.Unmarshal(actionParams, &sessTypeReq)
	switch sessTypeReq.SessionType {
	// This switch-case is just so that we can fail early if an unknown session type is passed in.
	case config.ShellPluginName, config.InteractiveCommandsPluginName, config.NonInteractiveCommandsPluginName:
		dataChannel.sessionType = config.ShellPluginName
		dataChannel.sessionProperties = sessTypeReq.Properties
		return nil
	case config.PortPluginName:
		dataChannel.sessionType = sessTypeReq.SessionType
		dataChannel.sessionProperties = sessTypeReq.Properties
		return nil
	default:
		return fmt.Errorf("unknown session type %s", sessTypeReq.SessionType)
	}
}

// IsSessionTypeSet check has data channel sessionType been set
func (dataChannel *DataChannel) IsSessionTypeSet() chan bool {
	return dataChannel.isSessionTypeSet
}

// IsSessionEnded check if session has ended
func (dataChannel *DataChannel) IsSessionEnded() bool {
	return dataChannel.isSessionEnded
}

// IsSessionEnded check if session has ended
func (dataChannel *DataChannel) EndSession() error {
	dataChannel.isSessionEnded = true
	return nil
}

// IsStreamMessageResendTimeout checks if resending a streaming message reaches timeout
func (dataChannel *DataChannel) IsStreamMessageResendTimeout() chan bool {
	return dataChannel.isStreamMessageResendTimeout
}

// SetSessionType set session type
func (dataChannel *DataChannel) SetSessionType(sessionType string) {
	dataChannel.sessionType = sessionType
	dataChannel.isSessionTypeSet <- true
}

// GetSessionType returns SessionType of the dataChannel
func (dataChannel *DataChannel) GetSessionType() string {
	return dataChannel.sessionType
}

// GetSessionProperties returns SessionProperties of the dataChannel
func (dataChannel *DataChannel) GetSessionProperties() interface{} {
	return dataChannel.sessionProperties
}

// GetWsChannel returns WsChannel of the dataChannel
func (dataChannel *DataChannel) GetWsChannel() communicator.IWebSocketChannel {
	return dataChannel.wsChannel
}

// SetWsChannel set WsChannel of the dataChannel
func (dataChannel *DataChannel) SetWsChannel(wsChannel communicator.IWebSocketChannel) {
	dataChannel.wsChannel = wsChannel
}

// GetStreamDataSequenceNumber returns StreamDataSequenceNumber of the dataChannel
func (dataChannel *DataChannel) GetStreamDataSequenceNumber() int64 {
	return dataChannel.StreamDataSequenceNumber
}

// GetAgentVersion returns agent version of the target instance
func (dataChannel *DataChannel) GetAgentVersion() string {
	return dataChannel.agentVersion
}

// SetAgentVersion set agent version of the target instance
func (dataChannel *DataChannel) SetAgentVersion(agentVersion string) {
	dataChannel.agentVersion = agentVersion
}
