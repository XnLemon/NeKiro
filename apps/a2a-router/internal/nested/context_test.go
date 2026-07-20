package nested

import (
	"strings"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

func runningParent() contracts.InvocationDetailResponseV4 {
	return contracts.InvocationDetailResponseV4{
		Invocation: contracts.InvocationRecordV4{
			InvocationID:     "inv_parent123",
			RootTaskID:       "task_root456",
			TraceID:          "trc_abc123_1",
			WorkspaceID:      "ws_test789",
			TargetAgentID:    "agent_caller01",
			AgentCardVersion: "1.0.0",
			Capability:       "process",
			Status:           "running",
			Caller:           contracts.Caller{Type: "user", ID: "user01"},
		},
	}
}

func TestDeriveChildContextSuccess(t *testing.T) {
	parent := runningParent()
	child, err := DeriveChildContext(parent, "agent_caller01")
	if err != nil {
		t.Fatalf("DeriveChildContext() error = %v", err)
	}
	if child.ChildInvocationID == "" {
		t.Error("expected non-empty child invocation ID")
	}
	if !strings.HasPrefix(child.ChildInvocationID, "inv_") {
		t.Errorf("child invocation ID should start with inv_, got %s", child.ChildInvocationID)
	}
	if child.ChildInvocationID == parent.Invocation.InvocationID {
		t.Error("child invocation ID must differ from parent")
	}
	if child.WorkspaceID != parent.Invocation.WorkspaceID {
		t.Errorf("workspace mismatch: got %s, want %s", child.WorkspaceID, parent.Invocation.WorkspaceID)
	}
	if child.RootTaskID != parent.Invocation.RootTaskID {
		t.Errorf("root task mismatch: got %s, want %s", child.RootTaskID, parent.Invocation.RootTaskID)
	}
	if child.TraceID != parent.Invocation.TraceID {
		t.Errorf("trace mismatch: got %s, want %s", child.TraceID, parent.Invocation.TraceID)
	}
	if child.Caller.Type != "agent" {
		t.Errorf("caller type should be agent, got %s", child.Caller.Type)
	}
	if child.Caller.ID != "agent_caller01" {
		t.Errorf("caller ID should be agent_caller01, got %s", child.Caller.ID)
	}
}

func TestDeriveChildContextParentNotFound(t *testing.T) {
	parent := contracts.InvocationDetailResponseV4{}
	_, err := DeriveChildContext(parent, "agent01")
	if err != ErrParentNotFound {
		t.Errorf("expected ErrParentNotFound, got %v", err)
	}
}

func TestDeriveChildContextParentNotRunning(t *testing.T) {
	parent := runningParent()
	parent.Invocation.Status = "succeeded"
	_, err := DeriveChildContext(parent, "agent_caller01")
	if err != ErrParentNotRunning {
		t.Errorf("expected ErrParentNotRunning, got %v", err)
	}

	parent.Invocation.Status = "failed"
	_, err = DeriveChildContext(parent, "agent_caller01")
	if err != ErrParentNotRunning {
		t.Errorf("expected ErrParentNotRunning, got %v", err)
	}

	parent.Invocation.Status = "pending"
	_, err = DeriveChildContext(parent, "agent_caller01")
	if err != ErrParentNotRunning {
		t.Errorf("expected ErrParentNotRunning, got %v", err)
	}
}

func TestDeriveChildContextTargetMismatch(t *testing.T) {
	parent := runningParent()
	_, err := DeriveChildContext(parent, "agent_different")
	if err != ErrParentTargetMismatch {
		t.Errorf("expected ErrParentTargetMismatch, got %v", err)
	}
}

func TestBuildChildDispatchRequest(t *testing.T) {
	parent := runningParent()
	child, err := DeriveChildContext(parent, "agent_caller01")
	if err != nil {
		t.Fatalf("DeriveChildContext() error = %v", err)
	}

	input := []byte(`{"query":"test"}`)
	dispatchReq := BuildChildDispatchRequest(child, "agent_target02", "summarize", input, false, "2.0.0")

	if dispatchReq.InvocationID != child.ChildInvocationID {
		t.Errorf("invocation ID mismatch: got %s, want %s", dispatchReq.InvocationID, child.ChildInvocationID)
	}
	if dispatchReq.RootTaskID != child.RootTaskID {
		t.Errorf("root task ID mismatch: got %s, want %s", dispatchReq.RootTaskID, child.RootTaskID)
	}
	if dispatchReq.TraceID != child.TraceID {
		t.Errorf("trace ID mismatch: got %s, want %s", dispatchReq.TraceID, child.TraceID)
	}
	if dispatchReq.WorkspaceID != child.WorkspaceID {
		t.Errorf("workspace ID mismatch: got %s, want %s", dispatchReq.WorkspaceID, child.WorkspaceID)
	}
	if dispatchReq.Caller.Type != "agent" {
		t.Errorf("caller type should be agent, got %s", dispatchReq.Caller.Type)
	}
	if dispatchReq.Caller.ID != "agent_caller01" {
		t.Errorf("caller ID should be agent_caller01, got %s", dispatchReq.Caller.ID)
	}
	if dispatchReq.TargetAgentID != "agent_target02" {
		t.Errorf("target agent ID mismatch: got %s, want agent_target02", dispatchReq.TargetAgentID)
	}
	if dispatchReq.AgentCardVersion != "2.0.0" {
		t.Errorf("agent card version mismatch: got %s, want 2.0.0", dispatchReq.AgentCardVersion)
	}
	if dispatchReq.Capability != "summarize" {
		t.Errorf("capability mismatch: got %s, want summarize", dispatchReq.Capability)
	}
	if string(dispatchReq.Input) != string(input) {
		t.Errorf("input mismatch: got %s, want %s", dispatchReq.Input, input)
	}
	if dispatchReq.Stream {
		t.Error("stream should be false")
	}
}

func TestNewInvocationIDUniqueness(t *testing.T) {
	ids := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id, err := newInvocationID()
		if err != nil {
			t.Fatalf("newInvocationID() error = %v", err)
		}
		if !strings.HasPrefix(id, "inv_") {
			t.Errorf("invocation ID should start with inv_, got %s", id)
		}
		if len(id) != 36 { // "inv_" + 32 hex chars
			t.Errorf("invocation ID should be 36 chars, got %d: %s", len(id), id)
		}
		if _, exists := ids[id]; exists {
			t.Errorf("duplicate invocation ID: %s", id)
		}
		ids[id] = struct{}{}
	}
}
