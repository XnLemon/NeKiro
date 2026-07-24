package clientsdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

const traceHeader = "x-nek-trace-id"

func safeIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for index, character := range []byte(value) {
		allowed := character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == ':' || character == '-'
		if !allowed || index == 0 && (character == '.' || character == '_' || character == ':' || character == '-') {
			return false
		}
	}
	return true
}

func validateJSONObject(data json.RawMessage) error {
	if data == nil {
		return errors.New("clientsdk: input is required")
	}
	if err := rejectDuplicateJSONMembers(data); err != nil {
		return errors.New("clientsdk: input is invalid")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("clientsdk: input must be a JSON object")
	}
	return nil
}

func encodeInvocationRequest(request InvokeRequest, stream bool, limit int64) ([]byte, error) {
	prefix := []byte(`{"agentId":"` + request.AgentID + `","capability":"` + request.Capability + `","input":`)
	suffix := []byte(`,"stream":false}`)
	if stream {
		suffix = []byte(`,"stream":true}`)
	}
	total := int64(len(prefix)) + int64(len(request.Input)) + int64(len(suffix))
	if total > limit {
		return nil, errors.New("clientsdk: invocation request exceeds the configured limit")
	}
	if err := validateJSONObject(request.Input); err != nil {
		return nil, err
	}
	payload := make([]byte, 0, int(total))
	payload = append(payload, prefix...)
	payload = append(payload, request.Input...)
	payload = append(payload, suffix...)
	return payload, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("response exceeds the configured limit")
	}
	return data, nil
}

func readAndCloseBounded(body io.ReadCloser, limit int64) ([]byte, error) {
	data, readErr := readBounded(body, limit)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	return data, nil
}

func closeWithError(body io.ReadCloser, err error) error {
	return errors.Join(err, body.Close())
}

func requireSingleHeader(header http.Header, name string) (string, error) {
	values := header.Values(name)
	if len(values) != 1 || values[0] == "" {
		return "", fmt.Errorf("clientsdk: response must contain exactly one %s header", name)
	}
	return values[0], nil
}

func requireTraceHeader(header http.Header) (contracts.TraceID, error) {
	value, err := requireSingleHeader(header, traceHeader)
	if err != nil {
		return "", err
	}
	traceID, err := contracts.ParseTraceID(value)
	if err != nil {
		return "", errors.New("clientsdk: response Trace header is invalid")
	}
	return traceID, nil
}

func requireMediaType(header http.Header, expected string) error {
	value, err := requireSingleHeader(header, "Content-Type")
	if err != nil {
		return err
	}
	if value != expected {
		return errors.New("clientsdk: response Content-Type is invalid")
	}
	return nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, isDelimiter := token.(json.Delim)
		if !isDelimiter {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object member name is invalid")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON object member %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	if err := walk(); err != nil {
		return err
	}
	return requireEOF(decoder)
}
