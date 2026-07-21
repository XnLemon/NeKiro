package runtimea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Nene7ko/NeKiro/contracts"
	agentsdk "github.com/Nene7ko/NeKiro/sdks/agent-sdk"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

type nestedInvoker interface {
	Invoke(context.Context, agentsdk.PlatformContext, agentsdk.NestedRequest) (*agentsdk.NestedResult, error)
}

type nestedService struct {
	config  Config
	profile contracts.A2AProfile
	invoker nestedInvoker
}

func newNestedService(config Config, invoker nestedInvoker) (*nestedService, error) {
	if invoker == nil {
		return nil, errors.New("runtime-a nested invoker is required")
	}
	profile, err := contracts.LoadA2AProfile()
	if err != nil {
		return nil, fmt.Errorf("runtime-a load A2A profile: %w", err)
	}
	return &nestedService{config: config, profile: profile, invoker: invoker}, nil
}

func (service *nestedService) invokeWithContext(ctx context.Context, platformContext agentsdk.PlatformContext, input json.RawMessage) (*agentsdk.NestedResult, error) {
	result, err := service.invoker.Invoke(ctx, platformContext, agentsdk.NestedRequest{
		TargetAgentID: service.config.TargetAgentID,
		Capability:    service.config.Capability,
		Input:         input,
		Stream:        false,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.InvocationID == "" {
		return nil, errors.New("runtime-a child invocation identity is missing")
	}
	if result.InvocationID == platformContext.InvocationID {
		return nil, errors.New("runtime-a child invocation must differ from its parent")
	}
	return result, nil
}

func (service *nestedService) platformContext(meta *a2asrv.RequestMeta) (agentsdk.PlatformContext, error) {
	if meta == nil {
		return agentsdk.PlatformContext{}, invalidParams("managed A2A request metadata is required")
	}
	traceID, err := requiredHeader(meta, service.profile.ContextHeaders.TraceID)
	if err != nil {
		return agentsdk.PlatformContext{}, err
	}
	invocationID, err := requiredHeader(meta, service.profile.ContextHeaders.InvocationID)
	if err != nil {
		return agentsdk.PlatformContext{}, err
	}
	rootTaskID, err := requiredHeader(meta, service.profile.ContextHeaders.RootTaskID)
	if err != nil {
		return agentsdk.PlatformContext{}, err
	}
	workspaceID, err := requiredHeader(meta, service.profile.ContextHeaders.WorkspaceID)
	if err != nil {
		return agentsdk.PlatformContext{}, err
	}
	contextValue := agentsdk.PlatformContext{
		InvocationID: invocationID,
		RootTaskID:   rootTaskID,
		TraceID:      traceID,
		WorkspaceID:  workspaceID,
		AgentID:      service.config.AgentID,
	}
	if err := contextValue.Validate(); err != nil {
		return agentsdk.PlatformContext{}, invalidParams(err.Error())
	}
	return contextValue, nil
}

func requiredHeader(meta *a2asrv.RequestMeta, name string) (string, error) {
	values, exists := meta.Get(name)
	if !exists || len(values) != 1 || values[0] == "" {
		return "", invalidParams(fmt.Sprintf("%s header is required exactly once", name))
	}
	return values[0], nil
}

func rootInput(message *a2a.Message) (json.RawMessage, error) {
	if message == nil {
		return nil, invalidParams("message is required")
	}
	if message.ID == "" {
		return nil, invalidParams("messageId is required")
	}
	if message.Role != a2a.MessageRoleUser {
		return nil, invalidParams("message role must be user")
	}
	if len(message.Parts) != 1 {
		return nil, invalidParams("exactly one data part is required")
	}
	part, ok := message.Parts[0].(a2a.DataPart)
	if !ok {
		return nil, invalidParams("the request part must be structured data")
	}
	if len(part.Data) != 2 {
		return nil, invalidParams("the data part must contain only fixture and value")
	}
	fixture, ok := part.Data["fixture"].(string)
	if !ok || fixture != "success" {
		return nil, invalidParams("fixture must be success")
	}
	value, exists := part.Data["value"]
	if !exists {
		return nil, invalidParams("value is required")
	}
	encodedValue, err := json.Marshal(value)
	if err != nil {
		return nil, invalidParams("value must be JSON-compatible")
	}
	input, err := json.Marshal(map[string]json.RawMessage{
		"fixture": json.RawMessage(`"success"`),
		"value":   encodedValue,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime-a encode nested input: %w", err)
	}
	return input, nil
}

func combinedResult(result *agentsdk.NestedResult) (json.RawMessage, error) {
	if result == nil {
		return nil, errors.New("runtime-a nested result is required")
	}
	if result.InvocationID == "" || result.RootTaskID == "" || result.TraceID == "" || result.Status != "succeeded" || result.Result == nil {
		return nil, errors.New("runtime-a nested result is incomplete")
	}
	encoded, err := json.Marshal(struct {
		Agent             string          `json:"agent"`
		ChildInvocationID string          `json:"childInvocationId"`
		ChildResult       json.RawMessage `json:"childResult"`
	}{
		Agent:             "runtime-a",
		ChildInvocationID: result.InvocationID,
		ChildResult:       result.Result,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime-a encode combined result: %w", err)
	}
	return encoded, nil
}

func invalidParams(message string) error {
	return fmt.Errorf("%s: %w", message, a2a.ErrInvalidParams)
}
