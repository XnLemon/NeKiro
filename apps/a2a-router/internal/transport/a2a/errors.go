package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
)

type jsonRPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type jsonFrame struct {
	object    bool
	expecting bool
	members   map[string]struct{}
}

type envelopeValidatingRoundTripper struct {
	base             http.RoundTripper
	maxResponseBytes int64
}

func (transport envelopeValidatingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	requestBody, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	if err := request.Body.Close(); err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	request.Body = io.NopCloser(bytes.NewReader(requestBody))

	var requestEnvelope jsonRPCEnvelope
	if err := json.Unmarshal(requestBody, &requestEnvelope); err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil || response.StatusCode != http.StatusOK {
		return response, err
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, transport.maxResponseBytes+1))
	if closeErr := response.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, classify(contracts.ErrorCodeAgentUnavailable, err)
	}
	if int64(len(responseBody)) > transport.maxResponseBytes {
		return nil, classify(contracts.ErrorCodeAgentResponseTooLarge, errors.New("A2A Agent response exceeds the configured limit"))
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A JSON-RPC response media type is invalid"))
	}
	if err := validateJSONRPCResponseEnvelope(requestEnvelope, responseBody); err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	response.Body = io.NopCloser(bytes.NewReader(responseBody))
	return response, nil
}

func validateJSONRPCResponseEnvelope(request jsonRPCEnvelope, responseBody []byte) error {
	if err := rejectDuplicateJSONMembers(responseBody); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	var response jsonRPCEnvelope
	if err := decoder.Decode(&response); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("A2A JSON-RPC response contains trailing data")
		}
		return err
	}
	if response.JSONRPC != "2.0" {
		return errors.New("A2A JSON-RPC response version is invalid")
	}
	if err := validateJSONRPCID(response.ID); err != nil {
		return err
	}
	if !equalJSONRPCID(request.ID, response.ID) {
		return errors.New("A2A JSON-RPC response id does not match the request")
	}
	hasResult := len(response.Result) > 0 && !bytes.Equal(bytes.TrimSpace(response.Result), []byte("null"))
	hasError := len(response.Error) > 0 && !bytes.Equal(bytes.TrimSpace(response.Error), []byte("null"))
	if hasResult == hasError {
		return errors.New("A2A JSON-RPC response must contain exactly one result or error")
	}
	return nil
}

func validateJSONRPCID(value json.RawMessage) error {
	if len(bytes.TrimSpace(value)) == 0 {
		return errors.New("A2A JSON-RPC response id is missing")
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return errors.New("A2A JSON-RPC response id is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("A2A JSON-RPC response id contains trailing data")
	}
	switch decoded.(type) {
	case nil, string, json.Number:
		return nil
	default:
		return errors.New("A2A JSON-RPC response id has unsupported JSON type")
	}
}

func equalJSONRPCID(left, right json.RawMessage) bool {
	decode := func(value json.RawMessage) (any, error) {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, errors.New("JSON-RPC response id contains trailing data")
			}
			return nil, err
		}
		return decoded, nil
	}
	leftValue, leftErr := decode(left)
	rightValue, rightErr := decode(right)
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftValue, rightValue)
}

func rejectDuplicateJSONMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var stack []jsonFrame
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, jsonFrame{object: true, expecting: true, members: map[string]struct{}{}})
			case '[':
				stack = append(stack, jsonFrame{})
			case '}', ']':
				if len(stack) == 0 {
					return errors.New("A2A JSON-RPC response has an unmatched closing delimiter")
				}
				stack = stack[:len(stack)-1]
				markValueConsumed(stack)
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expecting {
				current := &stack[len(stack)-1]
				if _, exists := current.members[value]; exists {
					return errors.New("A2A JSON-RPC response contains a duplicate member")
				}
				current.members[value] = struct{}{}
				current.expecting = false
			} else {
				markValueConsumed(stack)
			}
		default:
			markValueConsumed(stack)
		}
	}
}

func markValueConsumed(stack []jsonFrame) {
	if len(stack) > 0 && stack[len(stack)-1].object {
		stack[len(stack)-1].expecting = true
	}
}

// classifiedError carries the platform classification across the transport
// seam without exposing transport implementation types to the API package.
type classifiedError struct {
	code  contracts.PlatformErrorCode
	cause error
}

func (err *classifiedError) Error() string {
	return string(err.code)
}

func (err *classifiedError) Unwrap() error {
	return err.cause
}

func (err *classifiedError) PlatformErrorCode() contracts.PlatformErrorCode {
	return err.code
}

func classify(code contracts.PlatformErrorCode, cause error) error {
	if cause == nil {
		cause = errors.New(string(code))
	}
	return &classifiedError{code: code, cause: cause}
}

func classifyTransportError(err error) error {
	if err == nil {
		return nil
	}
	var alreadyClassified interface {
		error
		PlatformErrorCode() contracts.PlatformErrorCode
	}
	if errors.As(err, &alreadyClassified) {
		return alreadyClassified
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return classify(contracts.ErrorCodeTimeout, err)
	}
	var a2aError *a2ago.Error
	if errors.As(err, &a2aError) {
		return classify(contracts.ErrorCodeAgentExecutionFailed, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return classify(contracts.ErrorCodeAgentUnavailable, err)
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return classify(contracts.ErrorCodeAgentUnavailable, err)
	}
	message := err.Error()
	if strings.Contains(message, "failed to send HTTP request") || strings.Contains(message, "unexpected HTTP status") {
		return classify(contracts.ErrorCodeAgentUnavailable, err)
	}
	return classify(contracts.ErrorCodeA2AProtocol, err)
}
