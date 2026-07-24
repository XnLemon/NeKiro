package clientsdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

// PlatformError contains only validated, stable Gateway failure context. It
// deliberately does not retain the fixed wire message or raw response body.
type PlatformError struct {
	StatusCode   int
	Code         contracts.PlatformErrorCode
	TraceID      contracts.TraceID
	InvocationID string
	RootTaskID   string
}

// Error returns only the HTTP status and stable platform code.
func (platformError *PlatformError) Error() string {
	return fmt.Sprintf("clientsdk: Gateway returned status %d (%s)", platformError.StatusCode, platformError.Code)
}

// Correlated reports whether Gateway returned the complete accepted
// Invocation and root Task identity pair.
func (platformError *PlatformError) Correlated() bool {
	return platformError.InvocationID != "" && platformError.RootTaskID != ""
}

func (client *Client) decodePlatformError(response *http.Response) error {
	invalid := errors.New("clientsdk: Gateway error response is invalid")
	if err := requireMediaType(response.Header, "application/json"); err != nil {
		return closeWithError(response.Body, invalid)
	}
	headerTrace, err := requireTraceHeader(response.Header)
	if err != nil {
		return closeWithError(response.Body, invalid)
	}
	body, err := readAndCloseBounded(response.Body, client.responseLimitBytes)
	if err != nil {
		return fmt.Errorf("clientsdk: read Gateway error response: %w", err)
	}
	if err := rejectDuplicateJSONMembers(body); err != nil {
		return invalid
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil || members == nil {
		return invalid
	}
	_, hasInvocation := members["invocationId"]
	_, hasRootTask := members["rootTaskId"]
	if hasInvocation != hasRootTask {
		return invalid
	}

	result := &PlatformError{StatusCode: response.StatusCode}
	if hasInvocation {
		if err := client.runtime.ValidateCorrelatedPlatformErrorV4JSON(body); err != nil {
			return invalid
		}
		var value contracts.CorrelatedPlatformErrorV4
		if err := json.Unmarshal(body, &value); err != nil || value.TraceID != headerTrace {
			return invalid
		}
		result.Code, result.TraceID = value.Code, value.TraceID
		result.InvocationID, result.RootTaskID = value.InvocationID, value.RootTaskID
	} else {
		if err := client.runtime.ValidatePreCorrelationPlatformErrorV4JSON(body); err != nil {
			return invalid
		}
		var value contracts.PreCorrelationPlatformErrorV4
		if err := json.Unmarshal(body, &value); err != nil || value.TraceID != headerTrace {
			return invalid
		}
		result.Code, result.TraceID = value.Code, value.TraceID
	}
	if !validPlatformErrorStatus(response.StatusCode, result.Code, result.Correlated()) {
		return invalid
	}
	return result
}

func validPlatformErrorStatus(status int, code contracts.PlatformErrorCode, correlated bool) bool {
	switch status {
	case http.StatusBadRequest:
		return !correlated && code == contracts.ErrorCodeValidationError
	case http.StatusUnauthorized:
		return !correlated && code == contracts.ErrorCodeUnauthenticated
	case http.StatusForbidden:
		return !correlated && (code == contracts.ErrorCodeForbidden || code == contracts.ErrorCodeCapabilityNotAllowed)
	case http.StatusNotFound:
		return !correlated && (code == contracts.ErrorCodeNotFound || code == contracts.ErrorCodeAgentNotInstalled)
	case http.StatusNotAcceptable:
		return !correlated && code == contracts.ErrorCodeNotAcceptable
	case http.StatusConflict:
		return code == contracts.ErrorCodeConflict || code == contracts.ErrorCodeInstallationDisabled || code == contracts.ErrorCodeAgentDisabled || code == contracts.ErrorCodeAgentReleaseUnpublished || code == contracts.ErrorCodeAgentReleaseSuspended || code == contracts.ErrorCodeAgentReleaseRevoked || code == contracts.ErrorCodeCanceled
	case http.StatusRequestEntityTooLarge:
		return !correlated && code == contracts.ErrorCodePayloadTooLarge
	case http.StatusInternalServerError:
		return code == contracts.ErrorCodeInternal
	case http.StatusBadGateway:
		return correlated && (code == contracts.ErrorCodeAgentAuthUnsupported || code == contracts.ErrorCodeAgentResponseTooLarge || code == contracts.ErrorCodeAgentExecutionFailed || code == contracts.ErrorCodeA2AProtocol)
	case http.StatusServiceUnavailable:
		return code == contracts.ErrorCodeRouteNotFound || code == contracts.ErrorCodeAgentUnavailable || code == contracts.ErrorCodeDependency
	case http.StatusGatewayTimeout:
		return code == contracts.ErrorCodeTimeout
	default:
		return false
	}
}
