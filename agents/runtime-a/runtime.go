package runtimea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentsdk "github.com/Nene7ko/NeKiro/sdks/agent-sdk"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type platformContextKey struct{}

type runtimeEngine struct {
	runner runner.Runner
}

func newRuntimeEngine(config Config, service *nestedService) (*runtimeEngine, error) {
	if service == nil {
		return nil, errors.New("runtime-a nested service is required")
	}
	implementation := &runtimeAgent{config: config, service: service}
	frameworkRunner := runner.NewRunner(
		config.AgentID,
		implementation,
		runner.WithSessionService(inmemory.NewSessionService()),
	)
	return &runtimeEngine{runner: frameworkRunner}, nil
}

func (engine *runtimeEngine) run(ctx context.Context, platformContext agentsdk.PlatformContext, input []byte) (json.RawMessage, error) {
	if err := platformContext.Validate(); err != nil {
		return nil, err
	}
	runContext := context.WithValue(ctx, platformContextKey{}, platformContext)
	events, err := engine.runner.Run(
		runContext,
		platformContext.WorkspaceID,
		platformContext.InvocationID,
		model.NewUserMessage(string(input)),
	)
	if err != nil {
		return nil, fmt.Errorf("runtime-a runner: %w", err)
	}
	var content string
	for received := range events {
		if received == nil {
			continue
		}
		if received.IsTerminalError() {
			if received.Error != nil {
				return nil, errors.New(received.Error.Message)
			}
			return nil, errors.New("runtime-a runner returned an execution error")
		}
		if received.Response != nil {
			for _, choice := range received.Response.Choices {
				if choice.Message.Content != "" {
					content = choice.Message.Content
				}
			}
		}
		if received.IsRunnerCompletion() {
			break
		}
	}
	if content == "" {
		return nil, errors.New("runtime-a runner returned no result")
	}
	var result json.RawMessage
	if err := json.Unmarshal([]byte(content), &result); err != nil || result == nil {
		return nil, errors.New("runtime-a runner result is not JSON")
	}
	return result, nil
}

type runtimeAgent struct {
	config  Config
	service *nestedService
}

func (runtimeAgent *runtimeAgent) Run(ctx context.Context, invocation *agent.Invocation) (<-chan *event.Event, error) {
	if invocation == nil {
		return nil, errors.New("runtime-a invocation is required")
	}
	platformContext, ok := ctx.Value(platformContextKey{}).(agentsdk.PlatformContext)
	if !ok {
		return nil, errors.New("runtime-a platform context is required")
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(invocation.Message.Content), &input); err != nil || input == nil {
		return nil, errors.New("runtime-a invocation input is not a JSON object")
	}
	result, err := runtimeAgent.service.invokeWithContext(ctx, platformContext, json.RawMessage(invocation.Message.Content))
	if err != nil {
		return nil, err
	}
	combined, err := combinedResult(result)
	if err != nil {
		return nil, err
	}
	output := make(chan *event.Event, 1)
	output <- event.NewResponseEvent(invocation.InvocationID, runtimeAgent.config.AgentID, &model.Response{
		ID:      "runtime-a-response-" + invocation.InvocationID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "runtime-a-deterministic",
		Choices: []model.Choice{{Message: model.Message{Role: model.RoleAssistant, Content: string(combined)}}},
		Done:    true,
	})
	close(output)
	return output, nil
}

func (runtimeAgent *runtimeAgent) Tools() []tool.Tool { return nil }

func (runtimeAgent *runtimeAgent) Info() agent.Info {
	return agent.Info{Name: runtimeAgent.config.AgentID, Description: "Deterministic cross-runtime caller"}
}

func (runtimeAgent *runtimeAgent) SubAgents() []agent.Agent { return nil }

func (runtimeAgent *runtimeAgent) FindSubAgent(string) agent.Agent { return nil }
