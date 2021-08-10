// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package transfer

import (
	"github.com/aws/aws-sdk-go/private/protocol"
)

const (

	// ErrCodeAccessDeniedException for service response error code
	// "AccessDeniedException".
	//
	// You do not have sufficient access to perform this action.
	ErrCodeAccessDeniedException = "AccessDeniedException"

	// ErrCodeConflictException for service response error code
	// "ConflictException".
	//
	// This exception is thrown when the UpdatServer is called for a file transfer
	// protocol-enabled server that has VPC as the endpoint type and the server's
	// VpcEndpointID is not in the available state.
	ErrCodeConflictException = "ConflictException"

	// ErrCodeInternalServiceError for service response error code
	// "InternalServiceError".
	//
	// This exception is thrown when an error occurs in the Amazon Web ServicesTransfer
	// Family service.
	ErrCodeInternalServiceError = "InternalServiceError"

	// ErrCodeInvalidNextTokenException for service response error code
	// "InvalidNextTokenException".
	//
	// The NextToken parameter that was passed is invalid.
	ErrCodeInvalidNextTokenException = "InvalidNextTokenException"

	// ErrCodeInvalidRequestException for service response error code
	// "InvalidRequestException".
	//
	// This exception is thrown when the client submits a malformed request.
	ErrCodeInvalidRequestException = "InvalidRequestException"

	// ErrCodeResourceExistsException for service response error code
	// "ResourceExistsException".
	//
	// The requested resource does not exist.
	ErrCodeResourceExistsException = "ResourceExistsException"

	// ErrCodeResourceNotFoundException for service response error code
	// "ResourceNotFoundException".
	//
	// This exception is thrown when a resource is not found by the Amazon Web ServicesTransfer
	// Family service.
	ErrCodeResourceNotFoundException = "ResourceNotFoundException"

	// ErrCodeServiceUnavailableException for service response error code
	// "ServiceUnavailableException".
	//
	// The request has failed because the Amazon Web ServicesTransfer Family service
	// is not available.
	ErrCodeServiceUnavailableException = "ServiceUnavailableException"

	// ErrCodeThrottlingException for service response error code
	// "ThrottlingException".
	//
	// The request was denied due to request throttling.
	//
	// HTTP Status Code: 400
	ErrCodeThrottlingException = "ThrottlingException"
)

var exceptionFromCode = map[string]func(protocol.ResponseMetadata) error{
	"AccessDeniedException":       newErrorAccessDeniedException,
	"ConflictException":           newErrorConflictException,
	"InternalServiceError":        newErrorInternalServiceError,
	"InvalidNextTokenException":   newErrorInvalidNextTokenException,
	"InvalidRequestException":     newErrorInvalidRequestException,
	"ResourceExistsException":     newErrorResourceExistsException,
	"ResourceNotFoundException":   newErrorResourceNotFoundException,
	"ServiceUnavailableException": newErrorServiceUnavailableException,
	"ThrottlingException":         newErrorThrottlingException,
}
