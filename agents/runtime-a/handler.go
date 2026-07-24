package runtimea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"time"

	agentsdk "github.com/Nene7ko/NeKiro/sdks/agent-sdk"
	"github.com/Nene7ko/NeKiro/sdks/agent-sdk/routerauth"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

// Handler is the Runtime A A2A adapter. Runtime framework values never leave
// this package and the only nested destination is the injected SDK client.
type Handler struct {
	config  Config
	service *nestedService
	runtime *runtimeEngine
}

var _ a2asrv.RequestHandler = (*Handler)(nil)

// NewHandler creates a Runtime A handler with the given HTTP transport.
func NewHandler(config Config, doer agentsdk.HTTPDoer) (*Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	sdk, err := agentsdk.NewClient(doer, config.RouterURL, config.RouterToken, config.ResponseLimit, config.EventLimit)
	if err != nil {
		return nil, fmt.Errorf("runtime-a create Agent SDK client: %w", err)
	}
	return newHandlerWithInvoker(config, sdk)
}

func newHandlerWithInvoker(config Config, invoker nestedInvoker) (*Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	service, err := newNestedService(config, invoker)
	if err != nil {
		return nil, err
	}
	runtimeEngine, err := newRuntimeEngine(config, service)
	if err != nil {
		return nil, err
	}
	return &Handler{config: config, service: service, runtime: runtimeEngine}, nil
}

// NewHTTPHandler exposes only the active JSON-RPC A2A boundary.
func NewHTTPHandler(handler *Handler) http.Handler {
	if handler == nil {
		panic("runtime-a handler is required")
	}
	authentication, err := routerauth.NewMiddleware(handler.config.RouterAuth, time.Now)
	if err != nil {
		panic(err)
	}
	jsonRPCHandler := a2asrv.NewJSONRPCHandler(handler)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", authentication.Handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		jsonRPCHandler.ServeHTTP(writer, request)
	})))
	return mux
}

func (handler *Handler) OnSendMessage(ctx context.Context, params *a2a.MessageSendParams) (a2a.SendMessageResult, error) {
	if params == nil || params.Message == nil {
		return nil, invalidParams("message is required")
	}
	callContext, ok := a2asrv.CallContextFrom(ctx)
	if !ok {
		return nil, invalidParams("managed A2A call context is required")
	}
	platformContext, err := handler.service.platformContext(callContext.RequestMeta())
	if err != nil {
		return nil, err
	}
	input, err := rootInput(params.Message)
	if err != nil {
		return nil, err
	}
	result, err := handler.runtime.run(ctx, platformContext, input)
	if err != nil {
		return nil, safeRuntimeFailure(err)
	}
	combined, err := combinedData(result)
	if err != nil {
		return nil, errors.New("runtime-a combined result is invalid")
	}
	return &a2a.Message{
		ID:        "runtime-a-result-" + params.Message.ID,
		ContextID: params.Message.ContextID,
		Role:      a2a.MessageRoleAgent,
		Parts:     []a2a.Part{a2a.DataPart{Data: combined}},
	}, nil
}

func combinedData(result json.RawMessage) (map[string]any, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(result, &fields); err != nil || fields == nil {
		return nil, errors.New("combined result must be an object")
	}
	agent, ok := fields["agent"]
	var agentName string
	if !ok || json.Unmarshal(agent, &agentName) != nil || agentName != "runtime-a" {
		return nil, errors.New("combined result agent marker is invalid")
	}
	childInvocationID, ok := fields["childInvocationId"]
	var childID string
	if !ok || json.Unmarshal(childInvocationID, &childID) != nil || childID == "" {
		return nil, errors.New("combined result child invocation is missing")
	}
	childResult, ok := fields["childResult"]
	if !ok || len(childResult) == 0 || !json.Valid(childResult) {
		return nil, errors.New("combined result child result is missing")
	}
	return map[string]any{
		"agent":             json.RawMessage(agent),
		"childInvocationId": json.RawMessage(childInvocationID),
		"childResult":       json.RawMessage(childResult),
	}, nil
}

func safeRuntimeFailure(err error) error {
	var routerError *agentsdk.RouterError
	if errors.As(err, &routerError) {
		return fmt.Errorf("runtime-a nested Router failure: %s", routerError.Code)
	}
	return errors.New("runtime-a nested invocation failure (unknown category)")
}

func (*Handler) OnSendMessageStream(context.Context, *a2a.MessageSendParams) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(nil, a2a.ErrUnsupportedOperation)
	}
}

func (*Handler) OnGetTask(context.Context, *a2a.TaskQueryParams) (*a2a.Task, error) {
	return nil, a2a.ErrUnsupportedOperation
}

func (*Handler) OnCancelTask(context.Context, *a2a.TaskIDParams) (*a2a.Task, error) {
	return nil, a2a.ErrUnsupportedOperation
}

func (*Handler) OnListTasks(context.Context, *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	return nil, a2a.ErrUnsupportedOperation
}

func (*Handler) OnResubscribeToTask(context.Context, *a2a.TaskIDParams) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) { yield(nil, a2a.ErrUnsupportedOperation) }
}

func (*Handler) OnGetTaskPushConfig(context.Context, *a2a.GetTaskPushConfigParams) (*a2a.TaskPushConfig, error) {
	return nil, a2a.ErrUnsupportedOperation
}

func (*Handler) OnListTaskPushConfig(context.Context, *a2a.ListTaskPushConfigParams) ([]*a2a.TaskPushConfig, error) {
	return nil, a2a.ErrUnsupportedOperation
}

func (*Handler) OnSetTaskPushConfig(context.Context, *a2a.TaskPushConfig) (*a2a.TaskPushConfig, error) {
	return nil, a2a.ErrUnsupportedOperation
}

func (*Handler) OnDeleteTaskPushConfig(context.Context, *a2a.DeleteTaskPushConfigParams) error {
	return a2a.ErrUnsupportedOperation
}

func (*Handler) OnGetExtendedAgentCard(context.Context) (*a2a.AgentCard, error) {
	return nil, a2a.ErrUnsupportedOperation
}
