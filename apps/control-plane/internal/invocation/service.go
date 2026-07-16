package invocation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type Authorizer interface {
	AuthorizeInvocation(context.Context, workspace.AuthenticatedCaller, string, string, string) (workspace.AuthorizedInvocation, error)
}

type Router interface {
	Dispatch(context.Context, contracts.DispatchInvocationRequestV3, contracts.InvocationResultMode) (*RouterResponse, error)
}

type IDGenerator interface {
	NewRoot() (invocationID string, rootTaskID string, err error)
}

type Service struct {
	authorizer Authorizer
	router     Router
	ids        IDGenerator
}

type DispatchError struct {
	Code         contracts.PlatformErrorCode
	InvocationID string
	RootTaskID   string
	Cause        error
}

func (e *DispatchError) Error() string { return e.Cause.Error() }
func (e *DispatchError) Unwrap() error { return e.Cause }

func NewService(authorizer Authorizer, router Router, ids IDGenerator) (*Service, error) {
	if authorizer == nil || router == nil || ids == nil {
		return nil, errors.New("invocation dispatch dependencies are required")
	}
	return &Service{authorizer: authorizer, router: router, ids: ids}, nil
}

func (service *Service) Dispatch(
	ctx context.Context,
	caller workspace.AuthenticatedCaller,
	traceID contracts.TraceID,
	workspaceID string,
	request contracts.InvokeAgentRequest,
	input []byte,
	mode contracts.InvocationResultMode,
) (*RouterResponse, error) {
	authorized, err := service.authorizer.AuthorizeInvocation(ctx, caller, workspaceID, request.AgentID, request.Capability)
	if err != nil {
		return nil, err
	}
	invocationID, rootTaskID, err := service.ids.NewRoot()
	if err != nil {
		return nil, &DispatchError{Code: contracts.ErrorCodeInternal, Cause: fmt.Errorf("generate root invocation correlation: %w", err)}
	}
	dispatch := contracts.DispatchInvocationRequestV3{
		InvocationID:     invocationID,
		RootTaskID:       rootTaskID,
		TraceID:          traceID,
		Caller:           contracts.Caller{Type: "user", ID: caller.ID},
		WorkspaceID:      workspaceID,
		TargetAgentID:    request.AgentID,
		AgentCardVersion: authorized.AgentCardVersion,
		Capability:       request.Capability,
		Input:            input,
		Stream:           request.Stream,
	}
	response, err := service.router.Dispatch(ctx, dispatch, mode)
	if err != nil {
		code := contracts.ErrorCodeDependency
		if errors.Is(err, context.DeadlineExceeded) {
			code = contracts.ErrorCodeTimeout
		}
		return nil, &DispatchError{Code: code, InvocationID: invocationID, RootTaskID: rootTaskID, Cause: err}
	}
	return response, nil
}

type RandomIDGenerator struct {
	source io.Reader
}

func NewRandomIDGenerator() *RandomIDGenerator { return &RandomIDGenerator{source: rand.Reader} }

func (generator *RandomIDGenerator) NewRoot() (string, string, error) {
	invocation, err := randomID(generator.source, "inv_")
	if err != nil {
		return "", "", err
	}
	task, err := randomID(generator.source, "task_")
	if err != nil {
		return "", "", err
	}
	return invocation, task, nil
}

func randomID(source io.Reader, prefix string) (string, error) {
	data := make([]byte, 16)
	if _, err := io.ReadFull(source, data); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(data), nil
}
