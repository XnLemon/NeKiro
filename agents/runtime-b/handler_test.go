package runtimeb

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
)

func TestHandlerSendMessageDeterministicSuccessAndFailure(t *testing.T) {
	handler := NewHandler()
	request := fixtureParams("message-1", fixtureSuccess, map[string]any{"request": "sample"})

	first, err := handler.OnSendMessage(t.Context(), request)
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	second, err := handler.OnSendMessage(t.Context(), request)
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
	firstMessage := requireMessage(t, first)
	secondMessage := requireMessage(t, second)
	if firstMessage.ID != secondMessage.ID || firstMessage.ContextID != secondMessage.ContextID {
		t.Fatalf("deterministic identities differ: %#v / %#v", firstMessage, secondMessage)
	}
	part := requireDataPart(t, firstMessage.Parts[0])
	if part.Data["agent"] != "runtime-b" || part.Data["fixture"] != "success" {
		t.Fatalf("success result = %#v", part.Data)
	}
	value, ok := part.Data["value"].(map[string]any)
	if !ok || value["request"] != "sample" {
		t.Fatalf("success value = %#v", part.Data["value"])
	}

	result, err := handler.OnSendMessage(t.Context(), fixtureParams("message-failure", fixtureFailure, "fail"))
	if result != nil || !errors.Is(err, errFixtureFailure) {
		t.Fatalf("failure result = (%#v, %v)", result, err)
	}
}

func TestHandlerRejectsInvalidFixtureRequests(t *testing.T) {
	handler := NewHandler()
	valid := fixtureParams("message-valid", fixtureSuccess, "value")
	tests := map[string]*a2a.MessageSendParams{
		"nil params":       nil,
		"nil message":      {},
		"empty message id": fixtureParams("", fixtureSuccess, "value"),
		"agent role":       fixtureParamsWithRole("message-agent", a2a.MessageRoleAgent, fixtureSuccess, "value"),
		"no parts":         {Message: &a2a.Message{ID: "message-no-parts", Role: a2a.MessageRoleUser}},
		"text part": {Message: &a2a.Message{
			ID: "message-text", Role: a2a.MessageRoleUser, Parts: []a2a.Part{a2a.TextPart{Text: "success"}},
		}},
		"unknown fixture": fixtureParams("message-unknown", fixtureKind("unknown"), "value"),
		"missing value": {Message: &a2a.Message{
			ID: "message-missing", Role: a2a.MessageRoleUser,
			Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success"}}},
		}},
		"extra field": {Message: &a2a.Message{
			ID: "message-extra", Role: a2a.MessageRoleUser,
			Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "value", "extra": true}}},
		}},
	}
	valid.Message.Parts = append(valid.Message.Parts, a2a.DataPart{Data: map[string]any{"fixture": "success", "value": "other"}})
	tests["multiple parts"] = valid

	for name, request := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := handler.OnSendMessage(t.Context(), request)
			if result != nil || !errors.Is(err, a2a.ErrInvalidParams) {
				t.Fatalf("result = (%#v, %v), want invalid params", result, err)
			}
		})
	}
}

func TestHandlerSuccessfulStreamHasExactOrder(t *testing.T) {
	handler := NewHandler()
	events, err := collectEvents(handler.OnSendMessageStream(t.Context(), fixtureParams("stream-1", fixtureStreamSuccess, "payload")))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("event count = %d, want 5", len(events))
	}
	task, ok := events[0].(*a2a.Task)
	if !ok || task.Status.State != a2a.TaskStateWorking || task.ID == "" || task.ContextID == "" {
		t.Fatalf("event 0 = %#v", events[0])
	}
	message, ok := events[1].(*a2a.Message)
	if !ok || message.Role != a2a.MessageRoleAgent || message.TaskID != task.ID || message.ContextID != task.ContextID {
		t.Fatalf("event 1 = %#v", events[1])
	}
	base, ok := events[2].(*a2a.TaskArtifactUpdateEvent)
	if !ok || base.Append || base.LastChunk || base.TaskID != task.ID || base.Artifact == nil || base.Artifact.ID == "" {
		t.Fatalf("event 2 = %#v", events[2])
	}
	last, ok := events[3].(*a2a.TaskArtifactUpdateEvent)
	if !ok || !last.Append || !last.LastChunk || last.TaskID != task.ID || last.Artifact == nil || last.Artifact.ID != base.Artifact.ID {
		t.Fatalf("event 3 = %#v", events[3])
	}
	terminal, ok := events[4].(*a2a.TaskStatusUpdateEvent)
	if !ok || !terminal.Final || terminal.Status.State != a2a.TaskStateCompleted || terminal.TaskID != task.ID {
		t.Fatalf("event 4 = %#v", events[4])
	}

	historyLength := 1
	got, err := handler.OnGetTask(t.Context(), &a2a.TaskQueryParams{ID: task.ID, HistoryLength: &historyLength})
	if err != nil || got.Status.State != a2a.TaskStateCompleted || len(got.History) != 1 {
		t.Fatalf("get completed task = (%#v, %v)", got, err)
	}
	if _, err := handler.OnCancelTask(t.Context(), &a2a.TaskIDParams{ID: task.ID}); !errors.Is(err, a2a.ErrTaskNotCancelable) {
		t.Fatalf("cancel completed task = %v", err)
	}
}

func TestHandlerHoldStreamCancelsSameTask(t *testing.T) {
	handler := NewHandler()
	eventChannel := make(chan a2a.Event, 2)
	errorChannel := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event, err := range handler.OnSendMessageStream(t.Context(), fixtureParams("hold-1", fixtureHold, "payload")) {
			if err != nil {
				errorChannel <- err
				return
			}
			eventChannel <- event
		}
	}()

	working := requireTaskEvent(t, receiveEvent(t, eventChannel))
	canceled, err := handler.OnCancelTask(t.Context(), &a2a.TaskIDParams{ID: working.ID})
	if err != nil || canceled.ID != working.ID || canceled.Status.State != a2a.TaskStateCanceled {
		t.Fatalf("cancel = (%#v, %v)", canceled, err)
	}
	terminal, ok := receiveEvent(t, eventChannel).(*a2a.TaskStatusUpdateEvent)
	if !ok || !terminal.Final || terminal.TaskID != working.ID || terminal.Status.State != a2a.TaskStateCanceled {
		t.Fatalf("terminal = %#v", terminal)
	}
	select {
	case err := <-errorChannel:
		t.Fatalf("hold stream error: %v", err)
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("hold stream did not stop")
	}

	historyLength := 1
	queried, err := handler.OnGetTask(t.Context(), &a2a.TaskQueryParams{ID: working.ID, HistoryLength: &historyLength})
	if err != nil || queried.Status.State != a2a.TaskStateCanceled {
		t.Fatalf("get canceled task = (%#v, %v)", queried, err)
	}
}

func TestHandlerHoldStreamContextTerminationDoesNotCreateTerminal(t *testing.T) {
	handler := NewHandler()
	ctx, cancel := context.WithCancel(t.Context())
	eventChannel := make(chan a2a.Event, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event, err := range handler.OnSendMessageStream(ctx, fixtureParams("hold-context", fixtureHold, "payload")) {
			if err != nil {
				return
			}
			eventChannel <- event
		}
	}()
	working := requireTaskEvent(t, receiveEvent(t, eventChannel))
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("context-terminated stream did not stop")
	}
	historyLength := 1
	if _, err := handler.OnGetTask(t.Context(), &a2a.TaskQueryParams{ID: working.ID, HistoryLength: &historyLength}); !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("context-terminated task = %v, want task not found", err)
	}
}

func TestHandlerTaskErrorsAndHistoryBounds(t *testing.T) {
	handler := NewHandler()
	historyLength := 1
	if _, err := handler.OnGetTask(t.Context(), &a2a.TaskQueryParams{ID: "missing", HistoryLength: &historyLength}); !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("get missing = %v", err)
	}
	if _, err := handler.OnCancelTask(t.Context(), &a2a.TaskIDParams{ID: "missing"}); !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("cancel missing = %v", err)
	}
	if _, err := handler.OnGetTask(t.Context(), &a2a.TaskQueryParams{ID: "missing"}); !errors.Is(err, a2a.ErrInvalidParams) {
		t.Fatalf("get without history length = %v", err)
	}
	negative := -1
	if _, err := handler.OnGetTask(t.Context(), &a2a.TaskQueryParams{ID: "missing", HistoryLength: &negative}); !errors.Is(err, a2a.ErrInvalidParams) {
		t.Fatalf("get negative history length = %v", err)
	}
}

func TestHandlerConcurrentSendIdentityIsolation(t *testing.T) {
	handler := NewHandler()
	const requests = 100
	identities := make(chan string, requests)
	errorsChannel := make(chan error, requests)
	var group sync.WaitGroup
	for index := range requests {
		group.Add(1)
		go func() {
			defer group.Done()
			messageID := fmt.Sprintf("concurrent-%03d", index)
			result, err := handler.OnSendMessage(t.Context(), fixtureParams(messageID, fixtureSuccess, index))
			if err != nil {
				errorsChannel <- err
				return
			}
			message := result.(*a2a.Message)
			identities <- message.ID + "/" + message.ContextID
		}()
	}
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Errorf("concurrent send: %v", err)
	}
	close(identities)
	seen := make(map[string]struct{}, requests)
	for identity := range identities {
		if _, exists := seen[identity]; exists {
			t.Errorf("duplicate identity %q", identity)
		}
		seen[identity] = struct{}{}
	}
	if len(seen) != requests {
		t.Fatalf("identity count = %d, want %d", len(seen), requests)
	}
}

func fixtureParams(messageID string, kind fixtureKind, value any) *a2a.MessageSendParams {
	return fixtureParamsWithRole(messageID, a2a.MessageRoleUser, kind, value)
}

func fixtureParamsWithRole(messageID string, role a2a.MessageRole, kind fixtureKind, value any) *a2a.MessageSendParams {
	return &a2a.MessageSendParams{Message: &a2a.Message{
		ID: messageID, Role: role,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{"fixture": string(kind), "value": value}}},
	}}
}

func collectEvents(sequence iter.Seq2[a2a.Event, error]) ([]a2a.Event, error) {
	var events []a2a.Event
	for event, err := range sequence {
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func requireMessage(t *testing.T, result a2a.SendMessageResult) *a2a.Message {
	t.Helper()
	message, ok := result.(*a2a.Message)
	if !ok {
		t.Fatalf("result type = %T, want *a2a.Message", result)
	}
	return message
}

func requireDataPart(t *testing.T, part a2a.Part) a2a.DataPart {
	t.Helper()
	data, ok := part.(a2a.DataPart)
	if !ok {
		t.Fatalf("part type = %T, want a2a.DataPart", part)
	}
	return data
}

func requireTaskEvent(t *testing.T, event a2a.Event) *a2a.Task {
	t.Helper()
	task, ok := event.(*a2a.Task)
	if !ok {
		t.Fatalf("event type = %T, want *a2a.Task", event)
	}
	return task
}

func receiveEvent(t *testing.T, events <-chan a2a.Event) a2a.Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream event")
		return nil
	}
}
