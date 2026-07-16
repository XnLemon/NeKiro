package runtimeb

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/a2aproject/a2a-go/a2a"
)

type fixtureKind string

const (
	fixtureSuccess       fixtureKind = "success"
	fixtureStreamSuccess fixtureKind = "stream-success"
	fixtureFailure       fixtureKind = "failure"
	fixtureHold          fixtureKind = "hold"
)

var errFixtureFailure = errors.New("runtime-b deterministic fixture failure")

type fixtureRequest struct {
	kind  fixtureKind
	value any
}

func parseFixture(params *a2a.MessageSendParams) (fixtureRequest, error) {
	if params == nil || params.Message == nil {
		return fixtureRequest{}, invalidParams("message is required")
	}
	message := params.Message
	if message.ID == "" {
		return fixtureRequest{}, invalidParams("messageId is required")
	}
	if message.Role != a2a.MessageRoleUser {
		return fixtureRequest{}, invalidParams("message role must be user")
	}
	if len(message.Parts) != 1 {
		return fixtureRequest{}, invalidParams("exactly one data part is required")
	}
	part, ok := message.Parts[0].(a2a.DataPart)
	if !ok {
		return fixtureRequest{}, invalidParams("the request part must be structured data")
	}
	if len(part.Data) != 2 {
		return fixtureRequest{}, invalidParams("the data part must contain only fixture and value")
	}
	rawFixture, exists := part.Data["fixture"]
	if !exists {
		return fixtureRequest{}, invalidParams("fixture is required")
	}
	fixture, ok := rawFixture.(string)
	if !ok || fixture == "" {
		return fixtureRequest{}, invalidParams("fixture must be a non-empty string")
	}
	value, exists := part.Data["value"]
	if !exists {
		return fixtureRequest{}, invalidParams("value is required")
	}

	kind := fixtureKind(fixture)
	switch kind {
	case fixtureSuccess, fixtureStreamSuccess, fixtureFailure, fixtureHold:
	default:
		return fixtureRequest{}, invalidParams("fixture is not supported")
	}

	clonedValue, err := cloneJSONValue(value)
	if err != nil {
		return fixtureRequest{}, invalidParams("value must be JSON-compatible")
	}
	return fixtureRequest{kind: kind, value: clonedValue}, nil
}

func cloneJSONValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func invalidParams(message string) error {
	return fmt.Errorf("%s: %w", message, a2a.ErrInvalidParams)
}
