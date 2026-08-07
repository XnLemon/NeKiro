package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NeKiro-project/NeKiro/registry"
	"github.com/NeKiro-project/NeKiro/registry/testkit"
)

func TestTargetKeyV1Vector(t *testing.T) {
	target := mustTestTarget(t)
	key, err := TargetKey(target)
	if err != nil {
		t.Fatalf("TargetKey: %v", err)
	}
	const want = "f7nt6cxpnngsq3jmjii4h5fwo4gixmr6bk5jk2ptoh7knjxc7nmq"
	if key != want {
		t.Fatalf("TargetKey = %q, want %q", key, want)
	}
}

func TestBindingRejectsDefaultsAliasesAndReservedLabels(t *testing.T) {
	input := validBindingInput(t)
	for name, mutate := range map[string]func(*BindingInput){
		"missing version":              func(value *BindingInput) { value.Version = "" },
		"missing namespace":            func(value *BindingInput) { value.Namespace = "" },
		"missing service owner labels": func(value *BindingInput) { value.ServiceOwnerLabels = nil },
		"reserved selector": func(value *BindingInput) {
			value.EndpointSliceLabels = map[string]string{LabelReleaseTarget: "override"}
		},
		"raw release marker": func(value *BindingInput) {
			value.EndpointSliceLabels = map[string]string{"owner.example/name": value.Target.ReleaseID()}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := input
			mutate(&candidate)
			if _, err := NewBinding(candidate); !errors.Is(err, registry.ErrInvalid) {
				t.Fatalf("NewBinding error = %v, want invalid", err)
			}
		})
	}

	binding, err := NewBinding(input)
	if err != nil {
		t.Fatalf("NewBinding: %v", err)
	}
	labels := binding.ServiceLabels()
	labels[LabelReleaseTarget] = "changed"
	if binding.ServiceLabels()[LabelReleaseTarget] != binding.TargetKey() {
		t.Fatal("ServiceLabels exposed mutable retained labels")
	}
}

func TestSnapshotUsesExactlyTwoListsAndPreservesTopology(t *testing.T) {
	bindingInput := validBindingInput(t)
	binding, err := NewBinding(bindingInput)
	if err != nil {
		t.Fatalf("NewBinding: %v", err)
	}
	service := serviceObject(binding, "service-rv-item")
	slice := endpointSliceObject(binding, "slice-uid-a", "slice-rv-item", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.2", "10.0.0.1"}, nil, nil, nil, "zone-a"),
	})
	executor := newFixtureExecutor(
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{slice}})},
	)
	directory := mustTestDirectory(t, executor, binding)
	snapshot, err := directory.Snapshot(context.Background(), binding.Target())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.State() != registry.SnapshotStatePopulated || len(snapshot.Instances()) != 1 {
		t.Fatalf("snapshot state/instances = %s/%d, want populated/1", snapshot.State(), len(snapshot.Instances()))
	}
	instance := snapshot.Instances()[0]
	if instance.ID() != "instance-a" || instance.State() != registry.LifecycleStateReady {
		t.Fatalf("instance = %q/%q, want instance-a/ready", instance.ID(), instance.State())
	}
	endpoints := instance.Endpoints()
	if len(endpoints) != 2 || endpoints[0].Address() != "10.0.0.1" || endpoints[1].Address() != "10.0.0.2" {
		t.Fatalf("endpoints = %#v, want sorted two-address set", endpoints)
	}
	if got, ok := instance.Zone(); !ok || got != "zone-a" {
		t.Fatalf("zone = %q/%v, want zone-a/true", got, ok)
	}
	if got := instance.Metadata()[MetadataTargetRefUID]; got != "instance-a" {
		t.Fatalf("target uid metadata = %q", got)
	}
	if got := snapshot.Revision().SourceTokens(); len(got) != 2 || got[0] != "service-rv" || got[1] != "slice-rv" {
		t.Fatalf("revision tokens = %#v", got)
	}
	if !snapshot.Target().Equal(binding.Target()) {
		t.Fatal("snapshot changed exact target")
	}
	requests := executor.Requests()
	if len(requests) != 2 {
		t.Fatalf("executor calls = %d, want 2", len(requests))
	}
	if requests[0].Method() != "GET" || !strings.Contains(requests[0].URL(), "/api/v1/namespaces/ns-a/services?") ||
		!strings.Contains(requests[0].URL(), "fieldSelector=metadata.name%3Dservice-a") {
		t.Fatalf("service list request = %q", requests[0].URL())
	}
	if !strings.Contains(requests[1].URL(), "/apis/discovery.k8s.io/v1/namespaces/ns-a/endpointslices?") ||
		strings.Contains(requests[1].URL(), "release-target") || strings.Contains(requests[1].URL(), "binding-version") {
		t.Fatalf("slice selector request = %q", requests[1].URL())
	}
}

func TestSnapshotDistinguishesMissingServiceAndEmptySlices(t *testing.T) {
	binding := mustTestBinding(t)
	tests := []struct {
		name         string
		serviceItems []any
		wantState    registry.SnapshotState
	}{
		{name: "missing", serviceItems: nil, wantState: registry.SnapshotStateMissing},
		{name: "empty", serviceItems: []any{serviceObject(binding, "service-rv-item")}, wantState: registry.SnapshotStateEmpty},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			executor := newFixtureExecutor(
				fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": testCase.serviceItems})},
				fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{}})},
			)
			directory := mustTestDirectory(t, executor, binding)
			snapshot, err := directory.Snapshot(context.Background(), binding.Target())
			if err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
			if snapshot.State() != testCase.wantState || len(snapshot.Instances()) != 0 {
				t.Fatalf("snapshot = %s/%d, want %s/0", snapshot.State(), len(snapshot.Instances()), testCase.wantState)
			}
		})
	}
}

func TestObserveDualWatchLifecycleAndStaleTerminal(t *testing.T) {
	binding := mustTestBinding(t)
	service := serviceObject(binding, "service-rv-item")
	slice := endpointSliceObject(binding, "slice-uid-a", "slice-rv-item", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.1"}, boolPtr(true), boolPtr(true), boolPtr(false), "zone-a"),
	})
	serviceReader, serviceWriter := io.Pipe()
	sliceReader, sliceWriter := io.Pipe()
	executor := newFixtureExecutor(
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{slice}})},
		fixtureResponse{status: 200, body: serviceReader},
		fixtureResponse{status: 200, body: sliceReader},
	)
	directory := mustTestDirectory(t, executor, binding)
	observationResult := make(chan struct {
		observation registry.InstanceObservation
		err         error
	}, 1)
	go func() {
		observation, err := directory.Observe(context.Background(), binding.Target())
		observationResult <- struct {
			observation registry.InstanceObservation
			err         error
		}{observation: observation, err: err}
	}()
	result := awaitObservation(t, observationResult)
	if result.err != nil {
		t.Fatalf("Observe: %v", result.err)
	}
	if result.observation.Initial().State() != registry.SnapshotStatePopulated {
		t.Fatalf("initial state = %s", result.observation.Initial().State())
	}
	requests := executor.Requests()
	if len(requests) != 4 || strings.Contains(requests[0].URL(), "resourceVersion=") ||
		strings.Contains(requests[1].URL(), "resourceVersion=") || !strings.Contains(requests[2].URL(), "resourceVersion=service-rv") ||
		!strings.Contains(requests[3].URL(), "resourceVersion=slice-rv") {
		t.Fatalf("List/Watch handoff requests = %#v", requestURLs(requests))
	}

	modified := endpointSliceObject(binding, "slice-uid-a", "slice-rv-2", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.1"}, boolPtr(false), boolPtr(true), boolPtr(true), "zone-a"),
	})
	writeWatchEvent(t, serviceWriter, "MODIFIED", serviceObject(binding, "service-rv-2"))
	// The Service event does not alter aggregate topology and must not create a
	// fake change. The EndpointSlice event does create the draining transition.
	writeWatchEvent(t, sliceWriter, "MODIFIED", modified)
	change, err := result.observation.Watch().Next(context.Background())
	if err != nil {
		t.Fatalf("Next lifecycle change: %v", err)
	}
	if change.Kind() != registry.InstanceChangeInstancesChanged || change.Snapshot().Instances()[0].State() != registry.LifecycleStateDraining {
		t.Fatalf("change = %s/%s, want instances_changed/draining", change.Kind(), change.Snapshot().Instances()[0].State())
	}

	writeWatchEvent(t, sliceWriter, "ERROR", map[string]any{"code": 410, "reason": "Expired"})
	_, err = result.observation.Watch().Next(context.Background())
	if !errors.Is(err, registry.ErrStale) {
		t.Fatalf("stale Next error = %v, want stale", err)
	}
	_ = serviceWriter.Close()
	_ = sliceWriter.Close()
}

func TestObserveOwnerDeletionDeliversTargetDeletedThenClosed(t *testing.T) {
	binding := mustTestBinding(t)
	service := serviceObject(binding, "service-rv-item")
	executor := newFixtureExecutor(
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{}})},
	)
	serviceReader, serviceWriter := io.Pipe()
	sliceReader, sliceWriter := io.Pipe()
	executor.Append(fixtureResponse{status: 200, body: serviceReader}, fixtureResponse{status: 200, body: sliceReader})
	directory := mustTestDirectory(t, executor, binding)
	observation, err := directory.Observe(context.Background(), binding.Target())
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	writeWatchEvent(t, serviceWriter, "DELETED", serviceObject(binding, "service-rv-2"))
	change, err := observation.Watch().Next(context.Background())
	if err != nil || change.Kind() != registry.InstanceChangeTargetDeleted || change.Snapshot().State() != registry.SnapshotStateMissing {
		t.Fatalf("target deletion = %#v/%v", change, err)
	}
	if _, err := observation.Watch().Next(context.Background()); !errors.Is(err, registry.ErrClosed) {
		t.Fatalf("after target deletion = %v, want closed", err)
	}
	_ = serviceWriter.Close()
	_ = sliceWriter.Close()
}

func TestObserveMissingServiceAppearanceUsesExplicitStateChange(t *testing.T) {
	binding := mustTestBinding(t)
	serviceReader, serviceWriter := io.Pipe()
	sliceReader, sliceWriter := io.Pipe()
	executor := newFixtureExecutor(
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{}})},
		fixtureResponse{status: 200, body: serviceReader},
		fixtureResponse{status: 200, body: sliceReader},
	)
	directory := mustTestDirectory(t, executor, binding)
	observation, err := directory.Observe(context.Background(), binding.Target())
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if observation.Initial().State() != registry.SnapshotStateMissing {
		t.Fatalf("initial state = %s, want missing", observation.Initial().State())
	}
	writeWatchEvent(t, serviceWriter, "ADDED", serviceObject(binding, "service-rv-2"))
	change, err := observation.Watch().Next(context.Background())
	if err != nil {
		t.Fatalf("Next state change: %v", err)
	}
	if change.Kind() != registry.InstanceChangeStateChanged || change.PreviousState() != registry.SnapshotStateMissing ||
		change.Snapshot().State() != registry.SnapshotStateEmpty || len(change.Upserts()) != 0 || len(change.DeletedInstanceIDs()) != 0 {
		t.Fatalf("state change = kind=%s previous=%s state=%s upserts=%d deleted=%d", change.Kind(), change.PreviousState(), change.Snapshot().State(), len(change.Upserts()), len(change.DeletedInstanceIDs()))
	}
	_ = serviceWriter.Close()
	_ = sliceWriter.Close()
}

func TestSnapshotFoldsReshardDuplicatesAndRejectsConflicts(t *testing.T) {
	binding := mustTestBinding(t)
	service := serviceObject(binding, "service-rv-item")
	identical := endpointSliceObject(binding, "slice-uid-a", "slice-rv-a", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.1"}, nil, nil, nil, "zone-a"),
	})
	duplicate := endpointSliceObject(binding, "slice-uid-b", "slice-rv-b", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.1", "10.0.0.2"}, nil, nil, nil, "zone-a"),
	})
	executor := newFixtureExecutor(
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{identical, duplicate}})},
	)
	directory := mustTestDirectory(t, executor, binding)
	snapshot, err := directory.Snapshot(context.Background(), binding.Target())
	if err != nil {
		t.Fatalf("Snapshot duplicate aggregation: %v", err)
	}
	if got := len(snapshot.Instances()); got != 1 || len(snapshot.Instances()[0].Endpoints()) != 2 {
		t.Fatalf("aggregated topology = %d instances / %d endpoints, want 1 / 2", got, len(snapshot.Instances()[0].Endpoints()))
	}

	conflict := endpointSliceObject(binding, "slice-uid-c", "slice-rv-c", []map[string]any{
		endpointObject("instance-b", []string{"10.0.0.1"}, nil, nil, nil, "zone-a"),
	})
	executor = newFixtureExecutor(
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{identical, conflict}})},
	)
	directory = mustTestDirectory(t, executor, binding)
	if _, err := directory.Snapshot(context.Background(), binding.Target()); !errors.Is(err, registry.ErrInvalid) {
		t.Fatalf("tuple owner conflict = %v, want invalid", err)
	}
}

func TestTypedHTTPOutcomesAndNoRetry(t *testing.T) {
	binding := mustTestBinding(t)
	for name, response := range map[string]fixtureResponse{
		"unauthorized": {status: 401, body: io.NopCloser(strings.NewReader("{}"))},
		"forbidden":    {status: 403, body: io.NopCloser(strings.NewReader("{}"))},
		"unavailable":  {status: 503, body: io.NopCloser(strings.NewReader("{}"))},
		"stale":        {status: 410, body: io.NopCloser(strings.NewReader("{}"))},
		"transport":    {err: errors.New("transport failure")},
	} {
		t.Run(name, func(t *testing.T) {
			executor := newFixtureExecutor(response)
			directory := mustTestDirectory(t, executor, binding)
			_, err := directory.Snapshot(context.Background(), binding.Target())
			if err == nil {
				t.Fatal("Snapshot unexpectedly succeeded")
			}
			if len(executor.Requests()) != 1 {
				t.Fatalf("executor calls = %d, want exactly one failed list attempt", len(executor.Requests()))
			}
			switch name {
			case "unauthorized", "forbidden":
				if !errors.Is(err, registry.ErrUnauthorized) {
					t.Fatalf("error = %v, want unauthorized", err)
				}
			case "unavailable", "transport":
				if !errors.Is(err, registry.ErrUnavailable) {
					t.Fatalf("error = %v, want unavailable", err)
				}
			case "stale":
				if !errors.Is(err, registry.ErrStale) {
					t.Fatalf("error = %v, want stale", err)
				}
			}
		})
	}
}

func TestObserveOverflowAndOversizedEnvelopeLatchWatchInterrupted(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		bounds      ResourceBounds
		writeEvents func(*testing.T, Binding, *io.PipeWriter)
		wantCause   registry.OutcomeCause
	}{
		{
			name: "delivery overflow",
			bounds: ResourceBounds{
				ListResponseBytes:  1 << 20,
				WatchEnvelopeBytes: 1 << 16,
				EndpointSliceCount: 8,
				EndpointCount:      64,
				PendingChanges:     1,
			},
			writeEvents: func(t *testing.T, binding Binding, writer *io.PipeWriter) {
				writeWatchEvent(t, writer, "MODIFIED", endpointSliceObject(binding, "slice-uid-a", "slice-rv-2", []map[string]any{
					endpointObject("instance-a", []string{"10.0.0.1"}, boolPtr(false), boolPtr(true), boolPtr(true), "zone-a"),
				}))
				writeWatchEvent(t, writer, "MODIFIED", endpointSliceObject(binding, "slice-uid-a", "slice-rv-3", []map[string]any{
					endpointObject("instance-a", []string{"10.0.0.1"}, boolPtr(false), boolPtr(false), boolPtr(true), "zone-a"),
				}))
			},
			wantCause: registry.CauseDeliveryOverflow,
		},
		{
			name: "oversized envelope",
			bounds: ResourceBounds{
				ListResponseBytes:  1 << 20,
				WatchEnvelopeBytes: 16,
				EndpointSliceCount: 8,
				EndpointCount:      64,
				PendingChanges:     8,
			},
			writeEvents: func(t *testing.T, _ Binding, writer *io.PipeWriter) {
				t.Helper()
				if _, err := writer.Write([]byte(`{"type":"MODIFIED","object":{"padding":"this-is-deliberately-too-large"}}` + "\n")); err != nil {
					t.Fatalf("write oversized envelope: %v", err)
				}
			},
			wantCause: registry.CauseWatchEventTooLarge,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := validBindingInput(t)
			input.Bounds = testCase.bounds
			binding, err := NewBinding(input)
			if err != nil {
				t.Fatalf("NewBinding: %v", err)
			}
			service := serviceObject(binding, "service-rv-item")
			slice := endpointSliceObject(binding, "slice-uid-a", "slice-rv-item", []map[string]any{
				endpointObject("instance-a", []string{"10.0.0.1"}, boolPtr(true), boolPtr(true), boolPtr(false), "zone-a"),
			})
			serviceReader, serviceWriter := io.Pipe()
			sliceReader, sliceWriter := io.Pipe()
			executor := newFixtureExecutor(
				fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
				fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{slice}})},
				fixtureResponse{status: 200, body: serviceReader},
				fixtureResponse{status: 200, body: sliceReader},
			)
			directory := mustTestDirectory(t, executor, binding)
			observation, err := directory.Observe(context.Background(), binding.Target())
			if err != nil {
				t.Fatalf("Observe: %v", err)
			}
			testCase.writeEvents(t, binding, sliceWriter)
			watch := observation.Watch().(*kubernetesWatch)
			awaitCondition(t, func() bool { return watch.session.finished() })
			_, err = observation.Watch().Next(context.Background())
			if !errors.Is(err, registry.ErrWatchInterrupted) {
				t.Fatalf("Next error = %v, want watch_interrupted", err)
			}
			var outcome *registry.OutcomeError
			if !errors.As(err, &outcome) || outcome.Cause() != testCase.wantCause {
				t.Fatalf("watch terminal = %v, want cause %q", err, testCase.wantCause)
			}
			_ = serviceWriter.Close()
			_ = sliceWriter.Close()
		})
	}
}

func TestEndpointNormalizationRejectsInvalidIdentityAndPort(t *testing.T) {
	binding := mustTestBinding(t)
	service := serviceObject(binding, "service-rv-item")
	base := endpointSliceObject(binding, "slice-uid-a", "slice-rv-item", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.1"}, nil, nil, nil, "zone-a"),
	})
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing target uid", mutate: func(slice map[string]any) {
			slice["endpoints"].([]any)[0].(map[string]any)["targetRef"] = map[string]any{}
		}},
		{name: "fqdn address", mutate: func(slice map[string]any) {
			slice["endpoints"].([]any)[0].(map[string]any)["addresses"] = []string{"agent.example"}
		}},
		{name: "duplicate configured port", mutate: func(slice map[string]any) {
			slice["ports"] = []any{
				map[string]any{"name": binding.PortName(), "protocol": "TCP", "port": 8080},
				map[string]any{"name": binding.PortName(), "protocol": "TCP", "port": 8081},
			}
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			copySlice := cloneJSONMap(t, base)
			testCase.mutate(copySlice)
			executor := newFixtureExecutor(
				fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
				fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{copySlice}})},
			)
			directory := mustTestDirectory(t, executor, binding)
			if _, err := directory.Snapshot(context.Background(), binding.Target()); !errors.Is(err, registry.ErrInvalid) {
				t.Fatalf("Snapshot error = %v, want invalid", err)
			}
		})
	}
}

func TestEndpointConditionNilSemantics(t *testing.T) {
	binding := mustTestBinding(t)
	values := []struct {
		name  string
		value *bool
	}{
		{name: "nil", value: nil},
		{name: "true", value: boolPtr(true)},
		{name: "false", value: boolPtr(false)},
	}
	for _, ready := range values {
		for _, serving := range values {
			for _, terminating := range values {
				name := ready.name + "/" + serving.name + "/" + terminating.name
				t.Run(name, func(t *testing.T) {
					normalized, err := normalizeEndpoint(wireEndpoint{
						Addresses:  []string{"10.0.0.1"},
						TargetRef:  &wireObjectReference{UID: "instance-a"},
						Conditions: wireEndpointConditions{Ready: ready.value, Serving: serving.value, Terminating: terminating.value},
					}, binding, 8080)
					if err != nil {
						t.Fatalf("normalizeEndpoint: %v", err)
					}
					wantReady := ready.value == nil || *ready.value
					wantServing := serving.value == nil || *serving.value
					wantTerminating := terminating.value != nil && *terminating.value
					if normalized.ready != wantReady || normalized.serving != wantServing || normalized.terminating != wantTerminating {
						t.Fatalf("resolved = ready=%v serving=%v terminating=%v, want %v/%v/%v", normalized.ready, normalized.serving, normalized.terminating, wantReady, wantServing, wantTerminating)
					}
				})
			}
		}
	}
	for _, testCase := range []struct {
		ready, serving, terminating bool
		want                        registry.LifecycleState
	}{
		{true, true, true, registry.LifecycleStateDraining},
		{false, false, true, registry.LifecycleStateUnavailable},
		{true, true, false, registry.LifecycleStateReady},
		{false, true, false, registry.LifecycleStateUnavailable},
	} {
		if got := registry.DeriveLifecycleState(testCase.ready, testCase.serving, testCase.terminating); got != testCase.want {
			t.Fatalf("derived state = %s, want %s", got, testCase.want)
		}
	}
}

func TestKubernetesDirectoryRunsProviderNeutralConformance(t *testing.T) {
	binding := mustTestBinding(t)
	initialInstance := mustProviderInstance(t, "instance-a", true, true, false)
	changedInstance := mustProviderInstance(t, "instance-a", false, true, true)
	initialRevision := mustProviderRevision(t, []string{"service-rv", "slice-rv"}, 0)
	changeRevision := mustProviderRevision(t, []string{"service-rv", "slice-rv-2"}, 1)
	initial := mustProviderSnapshot(t, binding.Target(), initialRevision, registry.SnapshotStatePopulated, []registry.Instance{initialInstance})
	changedSnapshot := mustProviderSnapshot(t, binding.Target(), changeRevision, registry.SnapshotStatePopulated, []registry.Instance{changedInstance})
	change, err := registry.NewInstanceChange(registry.InstanceChangeInput{
		Kind:     registry.InstanceChangeInstancesChanged,
		Revision: changeRevision,
		Upserts:  []registry.Instance{changedInstance},
		Snapshot: changedSnapshot,
	})
	if err != nil {
		t.Fatalf("NewInstanceChange: %v", err)
	}

	service := serviceObject(binding, "service-rv-item")
	initialSlice := endpointSliceObject(binding, "slice-uid-a", "slice-rv-item", []map[string]any{
		endpointObject("instance-a", []string{"10.0.0.1"}, boolPtr(true), boolPtr(true), boolPtr(false), "zone-a"),
	})
	secondServiceReader, _ := io.Pipe()
	secondSliceReader, _ := io.Pipe()
	serviceReader, serviceWriter := io.Pipe()
	sliceReader, sliceWriter := io.Pipe()
	executor := newFixtureExecutor(
		// Snapshot.
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{initialSlice}})},
		// The backend-neutral suite reads a fresh snapshot to prove immutability.
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{initialSlice}})},
		// First Observe.
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{initialSlice}})},
		fixtureResponse{status: 200, body: serviceReader},
		fixtureResponse{status: 200, body: sliceReader},
		// Second Observe after the first terminal outcome.
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "service-rv"}, "items": []any{service}})},
		fixtureResponse{status: 200, body: jsonReadCloser(map[string]any{"metadata": map[string]any{"resourceVersion": "slice-rv"}, "items": []any{initialSlice}})},
		fixtureResponse{status: 200, body: secondServiceReader},
		fixtureResponse{status: 200, body: secondSliceReader},
	)
	directory := mustTestDirectory(t, executor, binding)
	driver := &kubernetesConformanceDriver{
		directory:     directory,
		binding:       binding,
		serviceWriter: serviceWriter,
		sliceWriter:   sliceWriter,
	}
	unbound := mustDifferentTarget(t)
	testkit.RunDirectoryConformance(t, driver, testkit.DirectoryConformanceFixture{
		Target:        binding.Target(),
		UnboundTarget: unbound,
		Initial:       initial,
		Change:        change,
		Terminal:      registry.NewOutcomeError(registry.OutcomeStale, registry.CauseResourceVersionExpired),
	})
}

type fixtureResponse struct {
	status int
	body   io.ReadCloser
	err    error
}

type kubernetesConformanceDriver struct {
	directory     *Directory
	binding       Binding
	serviceWriter *io.PipeWriter
	sliceWriter   *io.PipeWriter
}

func (d *kubernetesConformanceDriver) Directory() registry.InstanceDirectory { return d.directory }

func (d *kubernetesConformanceDriver) Emit(target registry.ReleaseTarget, change registry.InstanceChange) error {
	if !target.Equal(d.binding.Target()) || change.Kind() != registry.InstanceChangeInstancesChanged {
		return registry.ErrInvalid
	}
	instance := change.Snapshot().Instances()[0]
	endpoints := instance.Endpoints()
	addresses := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		addresses = append(addresses, endpoint.Address())
	}
	ready, serving, terminating := instance.Ready(), instance.Serving(), instance.Terminating()
	object := endpointSliceObject(d.binding, "slice-uid-a", change.Revision().SourceTokens()[1], []map[string]any{
		endpointObject(instance.ID(), addresses, &ready, &serving, &terminating, "zone-a"),
	})
	data, err := json.Marshal(map[string]any{"type": "MODIFIED", "object": object})
	if err != nil {
		return err
	}
	_, err = d.sliceWriter.Write(append(data, '\n'))
	return err
}

func (d *kubernetesConformanceDriver) Terminate(target registry.ReleaseTarget, terminal error) error {
	if !target.Equal(d.binding.Target()) || !errors.Is(terminal, registry.ErrStale) {
		return registry.ErrInvalid
	}
	data, err := json.Marshal(map[string]any{"type": "ERROR", "object": map[string]any{"code": 410, "reason": "Expired"}})
	if err != nil {
		return err
	}
	_, err = d.serviceWriter.Write(append(data, '\n'))
	return err
}

type fixtureExecutor struct {
	mu        sync.Mutex
	responses []fixtureResponse
	requests  []KubernetesRequest
}

func newFixtureExecutor(responses ...fixtureResponse) *fixtureExecutor {
	return &fixtureExecutor{responses: append([]fixtureResponse(nil), responses...)}
}

func (e *fixtureExecutor) Guarantees() KubernetesRequestExecutorGuarantees {
	return KubernetesRequestExecutorGuarantees{
		Version:                  KubernetesRequestExecutorVersionV1,
		ExactlyOneAttempt:        true,
		NoRedirect:               true,
		NoEnvironmentProxy:       true,
		NoResponseCache:          true,
		NoImplicitLimiter:        true,
		NoRetry:                  true,
		NoAuthoritySwitch:        true,
		NoHiddenReauthentication: true,
	}
}

func (e *fixtureExecutor) Execute(_ context.Context, request KubernetesRequest) (KubernetesResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, request)
	if len(e.responses) == 0 {
		return KubernetesResponse{}, errors.New("fixture response exhausted")
	}
	response := e.responses[0]
	e.responses = e.responses[1:]
	if response.err != nil {
		return KubernetesResponse{}, response.err
	}
	return KubernetesResponse{StatusCode: response.status, Body: response.body}, nil
}

func (e *fixtureExecutor) Append(responses ...fixtureResponse) {
	e.mu.Lock()
	e.responses = append(e.responses, responses...)
	e.mu.Unlock()
}

func (e *fixtureExecutor) Requests() []KubernetesRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]KubernetesRequest(nil), e.requests...)
}

func validBindingInput(t *testing.T) BindingInput {
	t.Helper()
	return BindingInput{
		Version:                BindingVersionV1,
		Target:                 mustTestTarget(t),
		APIOrigin:              "https://kube.example",
		Namespace:              "ns-a",
		ServiceName:            "service-a",
		ServiceUID:             "service-uid-a",
		EndpointSliceManagedBy: "controller.example",
		ServiceOwnerLabels:     map[string]string{"owner.example/name": "agent-a"},
		EndpointSliceLabels:    map[string]string{"owner.example/name": "agent-a"},
		AddressType:            registry.AddressTypeIPv4,
		PortName:               "a2a",
		Protocol:               registry.TransportProtocolTCP,
		Bounds: ResourceBounds{
			ListResponseBytes:  1 << 20,
			WatchEnvelopeBytes: 1 << 16,
			EndpointSliceCount: 8,
			EndpointCount:      64,
			PendingChanges:     8,
		},
	}
}

func mustTestBinding(t *testing.T) Binding {
	t.Helper()
	binding, err := NewBinding(validBindingInput(t))
	if err != nil {
		t.Fatalf("NewBinding: %v", err)
	}
	return binding
}

func mustTestDirectory(t *testing.T, executor *fixtureExecutor, binding Binding) *Directory {
	t.Helper()
	directory, err := NewDirectory(DirectoryConfig{Bindings: []Binding{binding}, Executor: executor})
	if err != nil {
		t.Fatalf("NewDirectory: %v", err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	return directory
}

func mustTestTarget(t *testing.T) registry.ReleaseTarget {
	t.Helper()
	target, err := registry.NewReleaseTarget(registry.ReleaseTargetInput{
		AgentID:           "agent-a",
		AgentCardVersion:  "1.0.0",
		ReleaseID:         "release-a",
		CardDigest:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CanonicalEndpoint: "https://agent.example/a2a",
		Audience:          "https://agent.example",
	})
	if err != nil {
		t.Fatalf("NewReleaseTarget: %v", err)
	}
	return target
}

func mustDifferentTarget(t *testing.T) registry.ReleaseTarget {
	t.Helper()
	target, err := registry.NewReleaseTarget(registry.ReleaseTargetInput{
		AgentID:           "agent-b",
		AgentCardVersion:  "1.0.0",
		ReleaseID:         "release-b",
		CardDigest:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CanonicalEndpoint: "https://agent-b.example/a2a",
		Audience:          "https://agent-b.example",
	})
	if err != nil {
		t.Fatalf("NewReleaseTarget: %v", err)
	}
	return target
}

func mustProviderInstance(t *testing.T, id string, ready, serving, terminating bool) registry.Instance {
	t.Helper()
	endpoint, err := registry.NewNetworkEndpoint(registry.NetworkEndpointInput{
		AddressType: registry.AddressTypeIPv4,
		Address:     "10.0.0.1",
		PortName:    "a2a",
		Port:        8080,
		Protocol:    registry.TransportProtocolTCP,
	})
	if err != nil {
		t.Fatalf("NewNetworkEndpoint: %v", err)
	}
	zone := "zone-a"
	instance, err := registry.NewInstance(registry.InstanceInput{
		ID:          id,
		Endpoints:   []registry.NetworkEndpoint{endpoint},
		Ready:       ready,
		Serving:     serving,
		Terminating: terminating,
		Zone:        &zone,
		Metadata: map[string]string{
			MetadataTargetRefUID: id,
			MetadataAddressType:  string(registry.AddressTypeIPv4),
		},
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return instance
}

func mustProviderRevision(t *testing.T, tokens []string, order uint64) registry.Revision {
	t.Helper()
	revision, err := registry.NewRevision(registry.RevisionInput{SourceTokens: tokens, LocalOrder: order})
	if err != nil {
		t.Fatalf("NewRevision: %v", err)
	}
	return revision
}

func mustProviderSnapshot(t *testing.T, target registry.ReleaseTarget, revision registry.Revision, state registry.SnapshotState, instances []registry.Instance) registry.InstanceSnapshot {
	t.Helper()
	snapshot, err := registry.NewInstanceSnapshot(registry.InstanceSnapshotInput{Target: target, Revision: revision, State: state, Instances: instances})
	if err != nil {
		t.Fatalf("NewInstanceSnapshot: %v", err)
	}
	return snapshot
}

func serviceObject(binding Binding, resourceVersion string) map[string]any {
	return map[string]any{"metadata": map[string]any{
		"name": binding.ServiceName(), "namespace": binding.Namespace(), "uid": binding.ServiceUID(),
		"resourceVersion": resourceVersion, "labels": binding.ServiceLabels(),
	}}
}

func endpointSliceObject(binding Binding, uid, resourceVersion string, endpoints []map[string]any) map[string]any {
	owner := map[string]any{"apiVersion": "v1", "kind": "Service", "name": binding.ServiceName(), "uid": binding.ServiceUID()}
	return map[string]any{
		"metadata": map[string]any{
			"name": "slice-" + uid, "namespace": binding.Namespace(), "uid": uid, "resourceVersion": resourceVersion,
			"labels": binding.EndpointSliceLabelsForObject(), "ownerReferences": []any{owner},
		},
		"addressType": string(binding.AddressType()),
		"ports":       []any{map[string]any{"name": binding.PortName(), "protocol": string(binding.Protocol()), "port": 8080}},
		"endpoints":   endpoints,
	}
}

func endpointObject(uid string, addresses []string, ready, serving, terminating *bool, zone string) map[string]any {
	conditions := map[string]any{}
	if ready != nil {
		conditions["ready"] = *ready
	}
	if serving != nil {
		conditions["serving"] = *serving
	}
	if terminating != nil {
		conditions["terminating"] = *terminating
	}
	endpoint := map[string]any{
		"addresses":  addresses,
		"targetRef":  map[string]any{"uid": uid},
		"conditions": conditions,
	}
	if zone != "" {
		endpoint["zone"] = zone
	}
	return endpoint
}

func jsonReadCloser(value any) io.ReadCloser {
	data, _ := json.Marshal(value)
	return io.NopCloser(strings.NewReader(string(data)))
}

func cloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal clone: %v", err)
	}
	copyValue := map[string]any{}
	if err := json.Unmarshal(data, &copyValue); err != nil {
		t.Fatalf("unmarshal clone: %v", err)
	}
	return copyValue
}

func requestURLs(requests []KubernetesRequest) []string {
	urls := make([]string, len(requests))
	for index, request := range requests {
		urls[index] = request.URL()
	}
	return urls
}

func writeWatchEvent(t *testing.T, writer *io.PipeWriter, kind string, object any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"type": kind, "object": object})
	if err != nil {
		t.Fatalf("marshal watch event: %v", err)
	}
	data = append(data, '\n')
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("write watch event: %v", err)
	}
}

func boolPtr(value bool) *bool { return &value }

func awaitObservation(t *testing.T, result <-chan struct {
	observation registry.InstanceObservation
	err         error
}) struct {
	observation registry.InstanceObservation
	err         error
} {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Observe")
		return struct {
			observation registry.InstanceObservation
			err         error
		}{}
	}
}

func awaitCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
