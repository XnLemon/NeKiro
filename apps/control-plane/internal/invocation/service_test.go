package invocation

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type authorizerStub struct {
	result workspace.AuthorizedInvocation
	err    error
	calls  int
}

func (stub *authorizerStub) AuthorizeInvocation(context.Context, workspace.AuthenticatedCaller, string, string, string) (workspace.AuthorizedInvocation, error) {
	stub.calls++
	return stub.result, stub.err
}

type routerStub struct {
	request contracts.DispatchInvocationRequestV4
	mode    contracts.InvocationResultMode
	result  *RouterResponse
	err     error
	calls   int
}

func (stub *routerStub) Dispatch(_ context.Context, request contracts.DispatchInvocationRequestV4, mode contracts.InvocationResultMode) (*RouterResponse, error) {
	stub.calls++
	stub.request, stub.mode = request, mode
	return stub.result, stub.err
}

type idsStub struct{ err error }

func (stub idsStub) NewRoot() (string, string, error) { return "inv-root", "task-root", stub.err }

func TestDispatchAuthorizesBeforeCreatingExactRootRouterRequest(t *testing.T) {
	digest := strings.Repeat("a", 64)
	authorizer := &authorizerStub{result: workspace.AuthorizedInvocation{AgentCardVersion: "1.2.3", AgentReleaseID: "release-root", AgentCardDigest: digest}}
	router := &routerStub{result: &RouterResponse{StatusCode: 200, ContentType: "application/json", Body: io.NopCloser(strings.NewReader(`{}`))}}
	service, err := NewService(authorizer, router, idsStub{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.Dispatch(context.Background(), workspace.AuthenticatedCaller{ID: "owner-a"}, "trace-root", "workspace-a", contracts.InvokeAgentRequest{AgentID: "agent-a", Capability: "capability.read", Stream: false}, []byte(`{"query":"x"}`), contracts.InvocationResultModeJSON)
	if err != nil || response == nil {
		t.Fatalf("dispatch result=%v err=%v", response, err)
	}
	request := router.request
	if authorizer.calls != 1 || router.calls != 1 || request.InvocationID != "inv-root" || request.RootTaskID != "task-root" || request.TraceID != "trace-root" || request.Caller != (contracts.Caller{Type: "user", ID: "owner-a"}) || request.WorkspaceID != "workspace-a" || request.TargetAgentID != "agent-a" || request.AgentCardVersion != "1.2.3" || request.AgentReleaseID != "release-root" || request.AgentCardDigest != digest || request.Capability != "capability.read" || string(request.Input) != `{"query":"x"}` || request.Stream || router.mode != contracts.InvocationResultModeJSON {
		t.Fatalf("unexpected dispatch: auth=%d router=%d request=%#v mode=%q", authorizer.calls, router.calls, request, router.mode)
	}
}

func TestDispatchStopsBeforeIDsAndRouterWhenWorkspaceRejects(t *testing.T) {
	authorizer := &authorizerStub{err: workspace.ErrCapabilityNotAllowed}
	router := &routerStub{}
	service, _ := NewService(authorizer, router, idsStub{err: errors.New("must not run")})
	_, err := service.Dispatch(context.Background(), workspace.AuthenticatedCaller{ID: "owner-a"}, "trace-root", "workspace-a", contracts.InvokeAgentRequest{AgentID: "agent-a", Capability: "capability.read"}, []byte(`{}`), contracts.InvocationResultModeJSON)
	if !errors.Is(err, workspace.ErrCapabilityNotAllowed) || router.calls != 0 {
		t.Fatalf("error=%v router calls=%d", err, router.calls)
	}
}

func TestDispatchMapsRootIDFailureToUncorrelatedInternalError(t *testing.T) {
	router := &routerStub{}
	service, err := NewService(&authorizerStub{result: workspace.AuthorizedInvocation{AgentCardVersion: "1.0.0"}}, router, idsStub{err: errors.New("entropy unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Dispatch(context.Background(), workspace.AuthenticatedCaller{ID: "owner-a"}, "trace-root", "workspace-a", contracts.InvokeAgentRequest{AgentID: "agent-a", Capability: "capability.read"}, []byte(`{}`), contracts.InvocationResultModeJSON)
	var dispatchError *DispatchError
	if !errors.As(err, &dispatchError) || dispatchError.Code != contracts.ErrorCodeInternal || dispatchError.InvocationID != "" || dispatchError.RootTaskID != "" || router.calls != 0 {
		t.Fatalf("dispatch error=%#v router calls=%d", err, router.calls)
	}
}

func TestDispatchCorrelatesRouterTransportAndDeadlineFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code contracts.PlatformErrorCode
	}{{"dependency", errors.New("offline"), contracts.ErrorCodeDependency}, {"deadline", context.DeadlineExceeded, contracts.ErrorCodeTimeout}} {
		t.Run(test.name, func(t *testing.T) {
			service, _ := NewService(&authorizerStub{result: workspace.AuthorizedInvocation{AgentCardVersion: "1.0.0"}}, &routerStub{err: test.err}, idsStub{})
			_, err := service.Dispatch(context.Background(), workspace.AuthenticatedCaller{ID: "owner-a"}, "trace-root", "workspace-a", contracts.InvokeAgentRequest{AgentID: "agent-a", Capability: "capability.read"}, []byte(`{}`), contracts.InvocationResultModeJSON)
			var dispatchError *DispatchError
			if !errors.As(err, &dispatchError) || dispatchError.Code != test.code || dispatchError.InvocationID != "inv-root" || dispatchError.RootTaskID != "task-root" {
				t.Fatalf("dispatch error = %#v", err)
			}
		})
	}
}
