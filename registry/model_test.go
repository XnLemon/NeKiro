package registry

import (
	"errors"
	"fmt"
	"testing"
)

func TestReleaseTargetExactFieldsAndValidation(t *testing.T) {
	input := validTargetInput()
	target, err := NewReleaseTarget(input)
	if err != nil {
		t.Fatalf("NewReleaseTarget: %v", err)
	}
	if target.AgentID() != input.AgentID ||
		target.AgentCardVersion() != input.AgentCardVersion ||
		target.ReleaseID() != input.ReleaseID ||
		target.CardDigest() != input.CardDigest ||
		target.CanonicalEndpoint() != input.CanonicalEndpoint ||
		target.Audience() != input.Audience {
		t.Fatal("ReleaseTarget did not preserve all six exact fields")
	}
	copyTarget, err := NewReleaseTarget(input)
	if err != nil {
		t.Fatalf("NewReleaseTarget copy: %v", err)
	}
	if !target.Equal(copyTarget) {
		t.Fatal("byte-identical targets are not equal")
	}

	invalid := input
	invalid.AgentID = " agent-a"
	if _, err := NewReleaseTarget(invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("leading whitespace error = %v, want invalid", err)
	}
	invalid = input
	invalid.CardDigest = "A" + input.CardDigest[1:]
	if _, err := NewReleaseTarget(invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("upper-case digest error = %v, want invalid", err)
	}
	invalid = input
	invalid.Audience += "/"
	if _, err := NewReleaseTarget(invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("audience path error = %v, want invalid", err)
	}
	for name, endpoint := range map[string]string{
		"query":         "https://agent.example/a2a?redirect=other",
		"fragment":      "https://agent.example/a2a#fragment",
		"upper host":    "https://AGENT.example/a2a",
		"default port":  "https://agent.example:443/a2a",
		"escaped path":  "https://agent.example/a%32a",
		"root no slash": "https://agent.example",
	} {
		t.Run("endpoint_"+name, func(t *testing.T) {
			invalid := input
			invalid.CanonicalEndpoint = endpoint
			if _, err := NewReleaseTarget(invalid); !errors.Is(err, ErrInvalid) {
				t.Fatalf("endpoint %q error = %v, want invalid", endpoint, err)
			}
		})
	}
}

func TestImmutableModelsCopyAllSlicesMapsAndPointers(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	zone := "zone-a"
	weight := 0
	metadata := map[string]string{"target_ref_uid": "uid-a"}
	endpoints := []NetworkEndpoint{
		mustEndpoint(t, NetworkEndpointInput{AddressType: AddressTypeIPv4, Address: "10.0.0.2", PortName: "a2a", Port: 8080, Protocol: TransportProtocolTCP}),
		mustEndpoint(t, NetworkEndpointInput{AddressType: AddressTypeIPv4, Address: "10.0.0.1", PortName: "a2a", Port: 8080, Protocol: TransportProtocolTCP}),
	}
	instance, err := NewInstance(InstanceInput{
		ID:        "uid-a",
		Endpoints: endpoints,
		Ready:     true,
		Serving:   true,
		Zone:      &zone,
		Weight:    &weight,
		Metadata:  metadata,
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	metadata["target_ref_uid"] = "changed"
	endpoints[0] = NetworkEndpoint{}
	zone = "changed"
	weight = 99

	if got := instance.Endpoints(); len(got) != 2 || got[0].Address() != "10.0.0.1" || got[1].Address() != "10.0.0.2" {
		t.Fatalf("endpoints = %#v, want sorted retained copies", got)
	}
	if got := instance.Metadata()["target_ref_uid"]; got != "uid-a" {
		t.Fatalf("metadata target_ref_uid = %q, want uid-a", got)
	}
	if got, ok := instance.Zone(); !ok || got != "zone-a" {
		t.Fatalf("zone = %q, %v, want zone-a, true", got, ok)
	}
	if got, ok := instance.Weight(); !ok || got != 0 {
		t.Fatalf("weight = %d, %v, want explicit zero, true", got, ok)
	}

	revisionTokens := []string{"service-rv", "slice-rv"}
	revision := mustRevision(t, revisionTokens, 0)
	revisionTokens[0] = "changed"
	instances := []Instance{instance}
	snapshot := mustSnapshot(t, target, revision, SnapshotStatePopulated, instances)
	instances[0] = Instance{}

	returnedInstances := snapshot.Instances()
	returnedInstances[0] = Instance{}
	returnedTokens := snapshot.Revision().SourceTokens()
	returnedTokens[0] = "changed"
	returnedMetadata := snapshot.Instances()[0].Metadata()
	returnedMetadata["target_ref_uid"] = "changed"
	if got := snapshot.Instances()[0].ID(); got != "uid-a" {
		t.Fatalf("snapshot instance ID = %q, want uid-a", got)
	}
	if got := snapshot.Revision().SourceTokens()[0]; got != "service-rv" {
		t.Fatalf("snapshot source token = %q, want service-rv", got)
	}
	if got := snapshot.Instances()[0].Metadata()["target_ref_uid"]; got != "uid-a" {
		t.Fatalf("snapshot metadata = %q, want uid-a", got)
	}

	changeRevision := mustRevision(t, []string{"service-rv-2", "slice-rv-2"}, 1)
	changedSnapshot := mustSnapshot(t, target, changeRevision, SnapshotStatePopulated, []Instance{instance})
	upserts := []Instance{instance}
	deleted := []string{"uid-z"}
	change, err := NewInstanceChange(InstanceChangeInput{
		Kind:               InstanceChangeInstancesChanged,
		Revision:           changeRevision,
		Upserts:            upserts,
		DeletedInstanceIDs: deleted,
		Snapshot:           changedSnapshot,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange: %v", err)
	}
	upserts[0] = Instance{}
	deleted[0] = "changed"
	returnedUpserts := change.Upserts()
	returnedUpserts[0] = Instance{}
	returnedDeleted := change.DeletedInstanceIDs()
	returnedDeleted[0] = "changed"
	if change.Upserts()[0].ID() != "uid-a" || change.DeletedInstanceIDs()[0] != "uid-z" {
		t.Fatal("InstanceChange exposed mutable slices")
	}
}

func TestSnapshotStateAndLifecycleRemainDistinct(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	revision := mustRevision(t, []string{"service-rv", "slice-rv"}, 0)
	missing := mustSnapshot(t, target, revision, SnapshotStateMissing, nil)
	empty := mustSnapshot(t, target, revision, SnapshotStateEmpty, nil)
	if missing.Equal(empty) {
		t.Fatal("missing and empty snapshots compare equal")
	}
	if DeriveLifecycleState(false, true, true) != LifecycleStateDraining {
		t.Fatal("terminating and serving must be draining")
	}
	if DeriveLifecycleState(true, false, true) != LifecycleStateUnavailable {
		t.Fatal("terminating and not serving must be unavailable")
	}
	if DeriveLifecycleState(true, true, false) != LifecycleStateReady {
		t.Fatal("ready and serving must be ready")
	}
}

func TestOutcomeErrorTaxonomySupportsErrorsIsAndHelpers(t *testing.T) {
	cases := []struct {
		outcome  Outcome
		sentinel error
		helper   func(error) bool
	}{
		{OutcomeMissing, ErrMissing, IsMissing},
		{OutcomeInvalid, ErrInvalid, IsInvalid},
		{OutcomeUnauthorized, ErrUnauthorized, IsUnauthorized},
		{OutcomeUnavailable, ErrUnavailable, IsUnavailable},
		{OutcomeStale, ErrStale, IsStale},
		{OutcomeWatchInterrupted, ErrWatchInterrupted, IsWatchInterrupted},
		{OutcomeCanceled, ErrCanceled, IsCanceled},
		{OutcomeClosed, ErrClosed, IsClosed},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.outcome), func(t *testing.T) {
			err := fmt.Errorf("operation: %w", NewOutcomeError(testCase.outcome, CauseNone))
			if !errors.Is(err, testCase.sentinel) || !testCase.helper(err) {
				t.Fatalf("error %v did not match %v", err, testCase.sentinel)
			}
			if got, ok := OutcomeOf(err); !ok || got != testCase.outcome {
				t.Fatalf("OutcomeOf = %q, %v, want %q, true", got, ok, testCase.outcome)
			}
		})
	}
}

func TestOutcomeErrorRejectsUnsafeCause(t *testing.T) {
	err := NewOutcomeError(OutcomeUnavailable, OutcomeCause("token=secret"))
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe cause error = %v, want invalid", err)
	}
	if err.Cause() != CauseUnknownCause {
		t.Fatalf("unsafe cause retained as %q, want %q", err.Cause(), CauseUnknownCause)
	}
}

func TestInstanceChangeRequiresExactUpsertResult(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	result := mustInstance(t, "uid-a", true, true, false)
	upsert := mustInstance(t, "uid-a", false, true, true)
	revision := mustRevision(t, []string{"service-rv-1", "slice-rv-1"}, 1)
	snapshot := mustSnapshot(t, target, revision, SnapshotStatePopulated, []Instance{result})
	if _, err := NewInstanceChange(InstanceChangeInput{
		Kind:     InstanceChangeInstancesChanged,
		Revision: revision,
		Upserts:  []Instance{upsert},
		Snapshot: snapshot,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched upsert error = %v, want invalid", err)
	}
}

func TestInstanceChangeStateChangedRequiresARealEmptyStateTransition(t *testing.T) {
	target := mustTarget(t, validTargetInput())
	revision := mustRevision(t, []string{"service-rv-1", "slice-rv-1"}, 1)
	empty := mustSnapshot(t, target, revision, SnapshotStateEmpty, nil)
	change, err := NewInstanceChange(InstanceChangeInput{
		Kind:          InstanceChangeStateChanged,
		Revision:      revision,
		PreviousState: SnapshotStateMissing,
		Snapshot:      empty,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange state transition: %v", err)
	}
	if change.PreviousState() != SnapshotStateMissing {
		t.Fatalf("PreviousState = %q, want missing", change.PreviousState())
	}

	populated := mustSnapshot(t, target, revision, SnapshotStatePopulated, []Instance{mustInstance(t, "uid-a", true, true, false)})
	for name, input := range map[string]InstanceChangeInput{
		"same state": {
			Kind:          InstanceChangeStateChanged,
			Revision:      revision,
			PreviousState: SnapshotStateEmpty,
			Snapshot:      empty,
		},
		"previous populated": {
			Kind:          InstanceChangeStateChanged,
			Revision:      revision,
			PreviousState: SnapshotStatePopulated,
			Snapshot:      empty,
		},
		"new populated": {
			Kind:          InstanceChangeStateChanged,
			Revision:      revision,
			PreviousState: SnapshotStateEmpty,
			Snapshot:      populated,
		},
		"delta": {
			Kind:          InstanceChangeStateChanged,
			Revision:      revision,
			PreviousState: SnapshotStateMissing,
			Upserts:       []Instance{mustInstance(t, "uid-a", true, true, false)},
			Snapshot:      empty,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewInstanceChange(input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("NewInstanceChange error = %v, want invalid", err)
			}
		})
	}
}

func validTargetInput() ReleaseTargetInput {
	return ReleaseTargetInput{
		AgentID:           "agent-a",
		AgentCardVersion:  "1.0.0",
		ReleaseID:         "release-a",
		CardDigest:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CanonicalEndpoint: "https://agent.example/a2a",
		Audience:          "https://agent.example",
	}
}

func mustTarget(t testing.TB, input ReleaseTargetInput) ReleaseTarget {
	t.Helper()
	target, err := NewReleaseTarget(input)
	if err != nil {
		t.Fatalf("NewReleaseTarget: %v", err)
	}
	return target
}

func mustEndpoint(t testing.TB, input NetworkEndpointInput) NetworkEndpoint {
	t.Helper()
	endpoint, err := NewNetworkEndpoint(input)
	if err != nil {
		t.Fatalf("NewNetworkEndpoint: %v", err)
	}
	return endpoint
}

func mustInstance(t testing.TB, id string, ready, serving, terminating bool) Instance {
	t.Helper()
	instance, err := NewInstance(InstanceInput{
		ID:          id,
		Endpoints:   []NetworkEndpoint{mustEndpoint(t, NetworkEndpointInput{AddressType: AddressTypeIPv4, Address: "10.0.0.1", PortName: "a2a", Port: 8080, Protocol: TransportProtocolTCP})},
		Ready:       ready,
		Serving:     serving,
		Terminating: terminating,
		Metadata:    map[string]string{"target_ref_uid": id},
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return instance
}

func mustRevision(t testing.TB, sourceTokens []string, localOrder uint64) Revision {
	t.Helper()
	revision, err := NewRevision(RevisionInput{SourceTokens: sourceTokens, LocalOrder: localOrder})
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	return revision
}

func mustSnapshot(t testing.TB, target ReleaseTarget, revision Revision, state SnapshotState, instances []Instance) InstanceSnapshot {
	t.Helper()
	snapshot, err := NewInstanceSnapshot(InstanceSnapshotInput{Target: target, Revision: revision, State: state, Instances: instances})
	if err != nil {
		t.Fatalf("NewInstanceSnapshot: %v", err)
	}
	return snapshot
}

func mustChange(t testing.TB, target ReleaseTarget, localOrder uint64, kind InstanceChangeKind, instances []Instance, upserts []Instance, deleted []string) InstanceChange {
	t.Helper()
	revision := mustRevision(t, []string{fmt.Sprintf("service-rv-%d", localOrder), fmt.Sprintf("slice-rv-%d", localOrder)}, localOrder)
	state := SnapshotStatePopulated
	if len(instances) == 0 {
		if kind == InstanceChangeTargetDeleted {
			state = SnapshotStateMissing
		} else {
			state = SnapshotStateEmpty
		}
	}
	snapshot := mustSnapshot(t, target, revision, state, instances)
	change, err := NewInstanceChange(InstanceChangeInput{Kind: kind, Revision: revision, Upserts: upserts, DeletedInstanceIDs: deleted, Snapshot: snapshot})
	if err != nil {
		t.Fatalf("NewInstanceChange: %v", err)
	}
	return change
}
