package nested

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/Nene7ko/NeKiro/contracts"
)

var (
	// ErrParentNotFound indicates the parent Invocation is absent from the Ledger.
	ErrParentNotFound = errors.New("nested: parent invocation not found")
	// ErrParentNotRunning indicates the parent Invocation is not in running status.
	ErrParentNotRunning = errors.New("nested: parent invocation is not running")
	// ErrParentTargetMismatch indicates the authenticated Agent does not match the parent target.
	ErrParentTargetMismatch = errors.New("nested: authenticated agent does not match parent target")
)

// ChildContext carries the derived trusted context for a child Invocation.
// All identity and lineage fields are generated from the committed parent;
// the nested request cannot provide them.
type ChildContext struct {
	ChildInvocationID string
	ParentInvocation  contracts.InvocationRecordV4
	Caller            contracts.Caller
	WorkspaceID       string
	RootTaskID        string
	TraceID           contracts.TraceID
}

// DeriveChildContext reads the committed parent projection and derives the
// child Invocation context. It requires the parent to be running and the
// authenticated Agent to match the parent's target Agent. The child receives
// a new Invocation ID; Workspace, root Task, Trace, and caller are inherited
// from the parent.
func DeriveChildContext(parent contracts.InvocationDetailResponseV4, authenticatedAgentID string) (ChildContext, error) {
	if parent.Invocation.InvocationID == "" {
		return ChildContext{}, ErrParentNotFound
	}
	if parent.Invocation.Status != "running" {
		return ChildContext{}, ErrParentNotRunning
	}
	if parent.Invocation.TargetAgentID != authenticatedAgentID {
		return ChildContext{}, ErrParentTargetMismatch
	}

	childID, err := newInvocationID()
	if err != nil {
		return ChildContext{}, fmt.Errorf("nested: generate child invocation id: %w", err)
	}

	return ChildContext{
		ChildInvocationID: childID,
		ParentInvocation:  parent.Invocation,
		Caller: contracts.Caller{
			Type: "agent",
			ID:   authenticatedAgentID,
		},
		WorkspaceID: parent.Invocation.WorkspaceID,
		RootTaskID:  parent.Invocation.RootTaskID,
		TraceID:     parent.Invocation.TraceID,
	}, nil
}

// BuildChildDispatchRequest constructs the trusted DispatchInvocationRequestV3
// for the child Invocation from the derived context and the untrusted nested
// request fields.
func BuildChildDispatchRequest(child ChildContext, targetAgentID, capability string, input []byte, stream bool, agentCardVersion string) contracts.DispatchInvocationRequestV3 {
	return contracts.DispatchInvocationRequestV3{
		InvocationID:     child.ChildInvocationID,
		RootTaskID:       child.RootTaskID,
		TraceID:          child.TraceID,
		Caller:           child.Caller,
		WorkspaceID:      child.WorkspaceID,
		TargetAgentID:    targetAgentID,
		AgentCardVersion: agentCardVersion,
		Capability:       capability,
		Input:            input,
		Stream:           stream,
	}
}

var invocationIDSource io.Reader = rand.Reader

func newInvocationID() (string, error) {
	data := make([]byte, 16)
	if _, err := io.ReadFull(invocationIDSource, data); err != nil {
		return "", err
	}
	return "inv_" + hex.EncodeToString(data), nil
}
