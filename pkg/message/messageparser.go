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

// message package defines data channel messages structure.
package message

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/aws/session-manager-plugin/pkg/log"
	"github.com/twinj/uuid"
)

// DeserializeClientMessage deserializes the byte array into an ClientMessage message.
// * Payload is a variable length byte data.
// * | HL|         MessageType           |Ver|  CD   |  Seq  | Flags |
// * |         MessageId                     |           Digest              | PayType | PayLen|
// * |         Payload      			|
func (clientMessage *ClientMessage) DeserializeClientMessage(input []byte) (err error) {
	clientMessage.MessageType, err = getString(input, ClientMessage_MessageTypeOffset, ClientMessage_MessageTypeLength)
	if err != nil {
		log.Errorf("Could not deserialize field MessageType with error: %v", err)
		return err
	}
	clientMessage.SchemaVersion, err = getUInteger(input, ClientMessage_SchemaVersionOffset)
	if err != nil {
		log.Errorf("Could not deserialize field SchemaVersion with error: %v", err)
		return err
	}
	clientMessage.CreatedDate, err = getULong(input, ClientMessage_CreatedDateOffset)
	if err != nil {
		log.Errorf("Could not deserialize field CreatedDate with error: %v", err)
		return err
	}
	clientMessage.SequenceNumber, err = getLong(input, ClientMessage_SequenceNumberOffset)
	if err != nil {
		log.Errorf("Could not deserialize field SequenceNumber with error: %v", err)
		return err
	}
	clientMessage.Flags, err = getULong(input, ClientMessage_FlagsOffset)
	if err != nil {
		log.Errorf("Could not deserialize field Flags with error: %v", err)
		return err
	}
	clientMessage.MessageId, err = getUuid(input, ClientMessage_MessageIdOffset)
	if err != nil {
		log.Errorf("Could not deserialize field MessageId with error: %v", err)
		return err
	}
	clientMessage.PayloadDigest, err = getBytes(input, ClientMessage_PayloadDigestOffset, ClientMessage_PayloadDigestLength)
	if err != nil {
		log.Errorf("Could not deserialize field PayloadDigest with error: %v", err)
		return err
	}
	clientMessage.PayloadType, err = getUInteger(input, ClientMessage_PayloadTypeOffset)
	if err != nil {
		log.Errorf("Could not deserialize field PayloadType with error: %v", err)
		return err
	}
	clientMessage.PayloadLength, err = getUInteger(input, ClientMessage_PayloadLengthOffset)

	headerLength, herr := getUInteger(input, ClientMessage_HLOffset)
	if herr != nil {
		log.Errorf("Could not deserialize field HeaderLength with error: %v", err)
		return err
	}

	clientMessage.HeaderLength = headerLength
	clientMessage.Payload = input[headerLength+ClientMessage_PayloadLengthLength:]

	return err
}

// getString get a string value from the byte array starting from the specified offset to the defined length.
func getString(byteArray []byte, offset int, stringLength int) (result string, err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+stringLength-1 > byteArrayLength-1 || offset < 0 {
		log.Error("getString failed: Offset is invalid.")
		return "", errors.New("offset is outside the byte array")
	}

	//remove nulls from the bytes array
	b := bytes.Trim(byteArray[offset:offset+stringLength], "\x00")

	return strings.TrimSpace(string(b)), nil
}

// getUInteger gets an unsigned integer
func getUInteger(byteArray []byte, offset int) (result uint32, err error) {
	var temp int32
	temp, err = getInteger(byteArray, offset)
	return uint32(temp), err
}

// getInteger gets an integer value from a byte array starting from the specified offset.
func getInteger(byteArray []byte, offset int) (result int32, err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+4 > byteArrayLength || offset < 0 {
		log.Error("getInteger failed: Offset is invalid.")
		return 0, errors.New("offset is bigger than the byte array")
	}
	return bytesToInteger(byteArray[offset : offset+4])
}

// bytesToInteger gets an integer from a byte array.
func bytesToInteger(input []byte) (result int32, err error) {
	var res int32
	inputLength := len(input)
	if inputLength != 4 {
		log.Error("bytesToInteger failed: input array size is not equal to 4.")
		return 0, errors.New("input array size is not equal to 4")
	}
	buf := bytes.NewBuffer(input)
	binary.Read(buf, binary.BigEndian, &res)
	return res, nil
}

// getULong gets an unsigned long integer
func getULong(byteArray []byte, offset int) (result uint64, err error) {
	var temp int64
	temp, err = getLong(byteArray, offset)
	return uint64(temp), err
}

// getLong gets a long integer value from a byte array starting from the specified offset. 64 bit.
func getLong(byteArray []byte, offset int) (result int64, err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+8 > byteArrayLength || offset < 0 {
		log.Error("getLong failed: Offset is invalid.")
		return 0, errors.New("offset is outside the byte array")
	}
	return bytesToLong(byteArray[offset : offset+8])
}

// bytesToLong gets a Long integer from a byte array.
func bytesToLong(input []byte) (result int64, err error) {
	var res int64
	inputLength := len(input)
	if inputLength != 8 {
		log.Error("bytesToLong failed: input array size is not equal to 8.")
		return 0, errors.New("input array size is not equal to 8")
	}
	buf := bytes.NewBuffer(input)
	binary.Read(buf, binary.BigEndian, &res)
	return res, nil
}

// getUuid gets the 128bit uuid from an array of bytes starting from the offset.
func getUuid(byteArray []byte, offset int) (result uuid.UUID, err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+16-1 > byteArrayLength-1 || offset < 0 {
		log.Error("getUuid failed: Offset is invalid.")
		return uuid.Nil.UUID(), errors.New("offset is outside the byte array")
	}

	leastSignificantLong, err := getLong(byteArray, offset)
	if err != nil {
		log.Error("getUuid failed: failed to get uuid LSBs Long value.")
		return uuid.Nil.UUID(), errors.New("failed to get uuid LSBs long value")
	}

	leastSignificantBytes, err := longToBytes(leastSignificantLong)
	if err != nil {
		log.Error("getUuid failed: failed to get uuid LSBs bytes value.")
		return uuid.Nil.UUID(), errors.New("failed to get uuid LSBs bytes value")
	}

	mostSignificantLong, err := getLong(byteArray, offset+8)
	if err != nil {
		log.Error("getUuid failed: failed to get uuid MSBs Long value.")
		return uuid.Nil.UUID(), errors.New("failed to get uuid MSBs long value")
	}

	mostSignificantBytes, err := longToBytes(mostSignificantLong)
	if err != nil {
		log.Error("getUuid failed: failed to get uuid MSBs bytes value.")
		return uuid.Nil.UUID(), errors.New("failed to get uuid MSBs bytes value")
	}

	uuidBytes := append(mostSignificantBytes, leastSignificantBytes...)

	return uuid.New(uuidBytes), nil
}

// longToBytes gets bytes array from a long integer.
func longToBytes(input int64) (result []byte, err error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, input)
	if buf.Len() != 8 {
		log.Error("longToBytes failed: buffer output length is not equal to 8.")
		return make([]byte, 8), errors.New("input array size is not equal to 8")
	}

	return buf.Bytes(), nil
}

// getBytes gets an array of bytes starting from the offset.
func getBytes(byteArray []byte, offset int, byteLength int) (result []byte, err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+byteLength-1 > byteArrayLength-1 || offset < 0 {
		log.Error("getBytes failed: Offset is invalid.")
		return make([]byte, byteLength), errors.New("offset is outside the byte array")
	}
	return byteArray[offset : offset+byteLength], nil
}

// Validate returns error if the message is invalid
func (clientMessage *ClientMessage) Validate() error {
	if StartPublicationMessage == clientMessage.MessageType ||
		PausePublicationMessage == clientMessage.MessageType {
		return nil
	}
	if clientMessage.HeaderLength == 0 {
		return errors.New("HeaderLength cannot be zero")
	}
	if clientMessage.MessageType == "" {
		return errors.New("MessageType is missing")
	}
	if clientMessage.CreatedDate == 0 {
		return errors.New("CreatedDate is missing")
	}
	if clientMessage.PayloadLength != 0 {
		hasher := sha256.New()
		hasher.Write(clientMessage.Payload)
		if !bytes.Equal(hasher.Sum(nil), clientMessage.PayloadDigest) {
			return errors.New("payload Hash is not valid")
		}
	}
	return nil
}

// SerializeClientMessage serializes ClientMessage message into a byte array.
// * Payload is a variable length byte data.
// * | HL|         MessageType           |Ver|  CD   |  Seq  | Flags |
// * |         MessageId                     |           Digest              |PayType| PayLen|
// * |         Payload      			|
func (clientMessage *ClientMessage) SerializeClientMessage() (result []byte, err error) {
	payloadLength := uint32(len(clientMessage.Payload))
	headerLength := uint32(ClientMessage_PayloadLengthOffset)
	// Set payload length
	clientMessage.PayloadLength = payloadLength

	totalMessageLength := headerLength + ClientMessage_PayloadLengthLength + payloadLength
	result = make([]byte, totalMessageLength)

	err = putUInteger(result, ClientMessage_HLOffset, headerLength)
	if err != nil {
		log.Errorf("Could not serialize HeaderLength with error: %v", err)
		return make([]byte, 1), err
	}

	startPosition := ClientMessage_MessageTypeOffset
	endPosition := ClientMessage_MessageTypeOffset + ClientMessage_MessageTypeLength - 1
	err = putString(result, startPosition, endPosition, clientMessage.MessageType)
	if err != nil {
		log.Errorf("Could not serialize MessageType with error: %v", err)
		return make([]byte, 1), err
	}

	err = putUInteger(result, ClientMessage_SchemaVersionOffset, clientMessage.SchemaVersion)
	if err != nil {
		log.Errorf("Could not serialize SchemaVersion with error: %v", err)
		return make([]byte, 1), err
	}

	err = putULong(result, ClientMessage_CreatedDateOffset, clientMessage.CreatedDate)
	if err != nil {
		log.Errorf("Could not serialize CreatedDate with error: %v", err)
		return make([]byte, 1), err
	}

	err = putLong(result, ClientMessage_SequenceNumberOffset, clientMessage.SequenceNumber)
	if err != nil {
		log.Errorf("Could not serialize SequenceNumber with error: %v", err)
		return make([]byte, 1), err
	}

	err = putULong(result, ClientMessage_FlagsOffset, clientMessage.Flags)
	if err != nil {
		log.Errorf("Could not serialize Flags with error: %v", err)
		return make([]byte, 1), err
	}

	err = putUuid(result, ClientMessage_MessageIdOffset, clientMessage.MessageId)
	if err != nil {
		log.Errorf("Could not serialize MessageId with error: %v", err)
		return make([]byte, 1), err
	}

	hasher := sha256.New()
	hasher.Write(clientMessage.Payload)

	startPosition = ClientMessage_PayloadDigestOffset
	endPosition = ClientMessage_PayloadDigestOffset + ClientMessage_PayloadDigestLength - 1
	err = putBytes(result, startPosition, endPosition, hasher.Sum(nil))
	if err != nil {
		log.Errorf("Could not serialize PayloadDigest with error: %v", err)
		return make([]byte, 1), err
	}

	err = putUInteger(result, ClientMessage_PayloadTypeOffset, clientMessage.PayloadType)
	if err != nil {
		log.Errorf("Could not serialize PayloadType with error: %v", err)
		return make([]byte, 1), err
	}

	err = putUInteger(result, ClientMessage_PayloadLengthOffset, clientMessage.PayloadLength)
	if err != nil {
		log.Errorf("Could not serialize PayloadLength with error: %v", err)
		return make([]byte, 1), err
	}

	startPosition = ClientMessage_PayloadOffset
	endPosition = ClientMessage_PayloadOffset + int(payloadLength) - 1
	err = putBytes(result, startPosition, endPosition, clientMessage.Payload)
	if err != nil {
		log.Errorf("Could not serialize Payload with error: %v", err)
		return make([]byte, 1), err
	}

	return result, nil
}

// putUInteger puts an unsigned integer
func putUInteger(byteArray []byte, offset int, value uint32) (err error) {
	return putInteger(byteArray, offset, int32(value))
}

// putInteger puts an integer value to a byte array starting from the specified offset.
func putInteger(byteArray []byte, offset int, value int32) (err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+4 > byteArrayLength || offset < 0 {
		log.Error("putInteger failed: Offset is invalid.")
		return errors.New("offset is outside the byte array")
	}

	bytes, err := integerToBytes(value)
	if err != nil {
		log.Error("putInteger failed: getBytesFromInteger Failed.")
		return err
	}

	copy(byteArray[offset:offset+4], bytes)
	return nil
}

// integerToBytes gets bytes array from an integer.
func integerToBytes(input int32) (result []byte, err error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, input)
	if buf.Len() != 4 {
		log.Error("integerToBytes failed: buffer output length is not equal to 4.")
		return make([]byte, 4), errors.New("input array size is not equal to 4")
	}

	return buf.Bytes(), nil
}

// putString puts a string value to a byte array starting from the specified offset.
func putString(byteArray []byte, offsetStart int, offsetEnd int, inputString string) (err error) {
	byteArrayLength := len(byteArray)
	if offsetStart > byteArrayLength-1 || offsetEnd > byteArrayLength-1 || offsetStart > offsetEnd || offsetStart < 0 {
		log.Error("putString failed: Offset is invalid.")
		return errors.New("offset is outside the byte array")
	}

	if offsetEnd-offsetStart+1 < len(inputString) {
		log.Error("putString failed: Not enough space to save the string.")
		return errors.New("not enough space to save the string")
	}

	// wipe out the array location first and then insert the new value.
	for i := offsetStart; i <= offsetEnd; i++ {
		byteArray[i] = ' '
	}

	copy(byteArray[offsetStart:offsetEnd+1], inputString)
	return nil
}

// putBytes puts bytes into the array at the correct offset.
func putBytes(byteArray []byte, offsetStart int, offsetEnd int, inputBytes []byte) (err error) {
	byteArrayLength := len(byteArray)
	if offsetStart > byteArrayLength-1 || offsetEnd > byteArrayLength-1 || offsetStart > offsetEnd || offsetStart < 0 {
		log.Error("putBytes failed: Offset is invalid.")
		return errors.New("offset is outside the byte array")
	}

	if offsetEnd-offsetStart+1 != len(inputBytes) {
		log.Error("putBytes failed: Not enough space to save the bytes.")
		return errors.New("not enough space to save the bytes")
	}

	copy(byteArray[offsetStart:offsetEnd+1], inputBytes)
	return nil
}

// putUuid puts the 128 bit uuid to an array of bytes starting from the offset.
func putUuid(byteArray []byte, offset int, input uuid.UUID) (err error) {
	if uuid.IsNil(input) {
		log.Error("putUuid failed: input is null.")
		return errors.New("putUuid failed: input is null")
	}

	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+16-1 > byteArrayLength-1 || offset < 0 {
		log.Error("putUuid failed: Offset is invalid.")
		return errors.New("offset is outside the byte array")
	}

	leastSignificantLong, err := bytesToLong(input.Bytes()[8:16])
	if err != nil {
		log.Error("putUuid failed: Failed to get leastSignificant Long value.")
		return errors.New("failed to get leastSignificant Long value")
	}

	mostSignificantLong, err := bytesToLong(input.Bytes()[0:8])
	if err != nil {
		log.Error("putUuid failed: Failed to get mostSignificantLong Long value.")
		return errors.New("failed to get mostSignificantLong Long value")
	}

	err = putLong(byteArray, offset, leastSignificantLong)
	if err != nil {
		log.Error("putUuid failed: Failed to put leastSignificantLong Long value.")
		return errors.New("failed to put leastSignificantLong Long value")
	}

	err = putLong(byteArray, offset+8, mostSignificantLong)
	if err != nil {
		log.Error("putUuid failed: Failed to put mostSignificantLong Long value.")
		return errors.New("failed to put mostSignificantLong Long value")
	}

	return nil
}

// putLong puts a long integer value to a byte array starting from the specified offset.
func putLong(byteArray []byte, offset int, value int64) (err error) {
	byteArrayLength := len(byteArray)
	if offset > byteArrayLength-1 || offset+8 > byteArrayLength || offset < 0 {
		log.Error("putInteger failed: Offset is invalid.")
		return errors.New("offset is outside the byte array")
	}

	mbytes, err := longToBytes(value)
	if err != nil {
		log.Error("putInteger failed: getBytesFromInteger Failed.")
		return err
	}

	copy(byteArray[offset:offset+8], mbytes)
	return nil
}

// putULong puts an unsigned long integer.
func putULong(byteArray []byte, offset int, value uint64) (err error) {
	return putLong(byteArray, offset, int64(value))
}

// SerializeClientMessagePayload marshals payloads for all session specific messages into bytes.
func SerializeClientMessagePayload(obj interface{}) (reply []byte, err error) {
	reply, err = json.Marshal(obj)
	if err != nil {
		log.Errorf("Could not serialize message with err: %s", err)
	}
	return
}

// SerializeClientMessageWithAcknowledgeContent marshals client message with payloads of acknowledge contents into bytes.
func SerializeClientMessageWithAcknowledgeContent(acknowledgeContent AcknowledgeContent) (reply []byte, err error) {

	acknowledgeContentBytes, err := SerializeClientMessagePayload(acknowledgeContent)
	if err != nil {
		// should not happen
		log.Errorf("Cannot marshal acknowledge content to json string: %v", acknowledgeContentBytes)
		return
	}

	uuid.SwitchFormat(uuid.FormatCanonical)
	messageId := uuid.NewV4()
	clientMessage := ClientMessage{
		MessageType:    AcknowledgeMessage,
		SchemaVersion:  1,
		CreatedDate:    uint64(time.Now().UnixNano() / 1000000),
		SequenceNumber: 0,
		Flags:          3,
		MessageId:      messageId,
		Payload:        acknowledgeContentBytes,
	}

	reply, err = clientMessage.SerializeClientMessage()
	if err != nil {
		log.Errorf("Error serializing client message with acknowledge content err: %v", err)
	}

	return
}

// DeserializeDataStreamAcknowledgeContent parses acknowledge content from payload of ClientMessage.
func (clientMessage *ClientMessage) DeserializeDataStreamAcknowledgeContent() (dataStreamAcknowledge AcknowledgeContent, err error) {
	if clientMessage.MessageType != AcknowledgeMessage {
		log.Errorf("ClientMessage is not of type AcknowledgeMessage. Found message type: %s", clientMessage.MessageType)
		return
	}

	err = json.Unmarshal(clientMessage.Payload, &dataStreamAcknowledge)
	if err != nil {
		log.Errorf("Could not deserialize rawMessage: %s", err)
	}
	return
}

// DeserializeChannelClosedMessage parses channelClosed message from payload of ClientMessage.
func (clientMessage *ClientMessage) DeserializeChannelClosedMessage() (channelClosed ChannelClosed, err error) {
	if clientMessage.MessageType != ChannelClosedMessage {
		log.Errorf("ClientMessage is not of type ChannelClosed. Found message type: %s", clientMessage.MessageType)
		return
	}

	err = json.Unmarshal(clientMessage.Payload, &channelClosed)
	if err != nil {
		log.Errorf("Could not deserialize rawMessage: %s", err)
	}
	return
}

func (clientMessage *ClientMessage) DeserializeHandshakeRequest() (handshakeRequest HandshakeRequestPayload, err error) {
	if clientMessage.PayloadType != uint32(HandshakeRequestPayloadType) {
		log.Errorf("ClientMessage PayloadType is not of type HandshakeRequestPayloadType. Found payload type: %d", clientMessage.PayloadType)
		return
	}

	err = json.Unmarshal(clientMessage.Payload, &handshakeRequest)
	if err != nil {
		log.Errorf("Could not deserialize rawMessage: %s", err)
	}
	return
}

func (clientMessage *ClientMessage) DeserializeHandshakeComplete() (handshakeComplete HandshakeCompletePayload, err error) {
	if clientMessage.PayloadType != uint32(HandshakeCompletePayloadType) {
		log.Errorf("ClientMessage PayloadType is not of type HandshakeCompletePayloadType. Found payload type: %d",
			clientMessage.PayloadType)
		return
	}

	err = json.Unmarshal(clientMessage.Payload, &handshakeComplete)
	if err != nil {
		log.Errorf("Could not deserialize rawMessage, %s : %s", clientMessage.Payload, err)
	}
	return
}
