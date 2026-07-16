package a2a

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
)

func (client *Client) SendNonStreaming(ctx context.Context, dispatch contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse) (json.RawMessage, error) {
	target, err := NewTarget(resolved, dispatch.Capability)
	if err != nil {
		return nil, err
	}
	inputLimit := client.inputLimitBytes
	if target.MaxInputBytes < inputLimit {
		inputLimit = target.MaxInputBytes
	}
	if int64(len(dispatch.Input)) > inputLimit {
		return nil, classify(contracts.ErrorCodePayloadTooLarge, errors.New("dispatch input exceeds the resolved Agent limit"))
	}
	params, err := messageSendParams(dispatch)
	if err != nil {
		return nil, err
	}
	result, err := client.SendMessage(ctx, target, ContextHeaders{
		TraceID:      dispatch.TraceID,
		InvocationID: dispatch.InvocationID,
		RootTaskID:   dispatch.RootTaskID,
		WorkspaceID:  dispatch.WorkspaceID,
	}, params)
	if err != nil {
		return nil, err
	}
	switch value := result.(type) {
	case *a2ago.Message:
		if err := contracts.ValidateA2AMessageResult(value); err != nil {
			return nil, classify(contracts.ErrorCodeA2AProtocol, err)
		}
	case *a2ago.Task:
		mapping, err := contracts.ValidateA2ATask(value)
		if err != nil {
			return nil, classify(contracts.ErrorCodeA2AProtocol, err)
		}
		if mapping.Classification != contracts.A2ATaskStateTerminal {
			return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("non-streaming dispatch received a non-terminal A2A task"))
		}
		if mapping.ErrorCode != "" {
			return nil, classify(mapping.ErrorCode, errors.New("Agent returned a terminal A2A task failure"))
		}
	default:
		return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A message/send returned an unsupported result"))
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	return json.RawMessage(encoded), nil
}

func (client *Client) ValidateNonStreamingTarget(dispatch contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse) error {
	_, err := NewTarget(resolved, dispatch.Capability)
	return err
}

func (client *Client) ValidateNonStreamingInput(dispatch contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse) error {
	maxInputBytes, err := parseCardLimit(resolved.Card.Limits.MaxInputBytes.String())
	if err != nil {
		return classify(contracts.ErrorCodeA2AProtocol, err)
	}
	inputLimit := client.inputLimitBytes
	if maxInputBytes < inputLimit {
		inputLimit = maxInputBytes
	}
	if int64(len(dispatch.Input)) > inputLimit {
		return classify(contracts.ErrorCodePayloadTooLarge, errors.New("dispatch input exceeds the resolved Agent limit"))
	}
	return nil
}

func messageSendParams(dispatch contracts.DispatchInvocationRequestV3) (*a2ago.MessageSendParams, error) {
	var input map[string]json.RawMessage
	if err := json.Unmarshal(dispatch.Input, &input); err != nil {
		return nil, err
	}
	data := make(map[string]any, len(input))
	for key, value := range input {
		data[key] = value
	}
	return &a2ago.MessageSendParams{Message: &a2ago.Message{
		ID:    dispatch.InvocationID,
		Role:  a2ago.MessageRoleUser,
		Parts: []a2ago.Part{a2ago.DataPart{Data: data}},
	}}, nil
}
