package runtimeb

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2asrv"
)

func TestListenAddressFromEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		valid bool
	}{
		{name: "valid", value: "127.0.0.1:4102", set: true, valid: true},
		{name: "missing"},
		{name: "empty", set: true},
		{name: "whitespace", value: " 127.0.0.1:4102", set: true},
		{name: "missing host", value: ":4102", set: true},
		{name: "missing port", value: "127.0.0.1", set: true},
		{name: "zero port", value: "127.0.0.1:0", set: true},
		{name: "large port", value: "127.0.0.1:65536", set: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			address, err := ListenAddressFromEnvironment(func(name string) (string, bool) {
				if name != ListenAddressEnvironment {
					t.Fatalf("lookup name = %q", name)
				}
				return test.value, test.set
			})
			if test.valid {
				if err != nil || address != test.value {
					t.Fatalf("address = (%q, %v)", address, err)
				}
				return
			}
			if err == nil || address != "" {
				t.Fatalf("invalid address = (%q, %v)", address, err)
			}
		})
	}
}

func TestOfficialA2AClientAllActiveOperations(t *testing.T) {
	handler := NewHandler()
	server := httptest.NewServer(NewHTTPHandler(handler))
	t.Cleanup(server.Close)
	client := newA2AClient(t, server, nil)

	result, err := client.SendMessage(t.Context(), fixtureParams("official-send", fixtureSuccess, map[string]any{"exact": true}))
	if err != nil {
		t.Fatalf("message/send: %v", err)
	}
	message := requireMessage(t, result)
	if err := contracts.ValidateA2AMessageResult(message); err != nil {
		t.Fatalf("message/send profile validation: %v", err)
	}

	events, err := collectEvents(client.SendStreamingMessage(t.Context(), fixtureParams("official-stream", fixtureStreamSuccess, "stream")))
	if err != nil {
		t.Fatalf("message/stream: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("stream event count = %d", len(events))
	}
	task := requireTaskEvent(t, events[0])
	historyLength := 1
	queried, err := client.GetTask(t.Context(), &a2a.TaskQueryParams{ID: task.ID, HistoryLength: &historyLength})
	if err != nil {
		t.Fatalf("tasks/get: %v", err)
	}
	if _, err := contracts.ValidateA2ATask(queried); err != nil {
		t.Fatalf("tasks/get profile validation: %v", err)
	}
	if _, err := client.CancelTask(t.Context(), &a2a.TaskIDParams{ID: task.ID}); !errors.Is(err, a2a.ErrTaskNotCancelable) {
		t.Fatalf("tasks/cancel completed = %v", err)
	}

	holdEvents := make(chan a2a.Event, 2)
	holdErrors := make(chan error, 1)
	holdDone := make(chan struct{})
	go func() {
		defer close(holdDone)
		for event, err := range client.SendStreamingMessage(t.Context(), fixtureParams("official-hold", fixtureHold, "hold")) {
			if err != nil {
				holdErrors <- err
				return
			}
			holdEvents <- event
		}
	}()
	holdTask := requireTaskEvent(t, receiveEvent(t, holdEvents))
	canceled, err := client.CancelTask(t.Context(), &a2a.TaskIDParams{ID: holdTask.ID})
	if err != nil || canceled.Status.State != a2a.TaskStateCanceled || canceled.ID != holdTask.ID {
		t.Fatalf("tasks/cancel working = (%#v, %v)", canceled, err)
	}
	terminal, ok := receiveEvent(t, holdEvents).(*a2a.TaskStatusUpdateEvent)
	if !ok || terminal.Status.State != a2a.TaskStateCanceled || !terminal.Final {
		t.Fatalf("canceled stream terminal = %#v", terminal)
	}
	select {
	case err := <-holdErrors:
		t.Fatalf("held official stream: %v", err)
	case <-holdDone:
	case <-time.After(2 * time.Second):
		t.Fatal("held official stream did not stop")
	}

	if _, err := client.GetTask(t.Context(), &a2a.TaskQueryParams{ID: "missing", HistoryLength: &historyLength}); !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("tasks/get missing = %v", err)
	}
	if _, err := client.CancelTask(t.Context(), &a2a.TaskIDParams{ID: "missing"}); !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("tasks/cancel missing = %v", err)
	}
	if result, err := client.SendMessage(t.Context(), fixtureParams("official-failure", fixtureFailure, "fail")); err == nil || result != nil {
		t.Fatalf("failure result = (%#v, %v)", result, err)
	}
}

func TestOfficialServerReceivesAllProfileContextHeaders(t *testing.T) {
	profile, err := contracts.LoadA2AProfile()
	if err != nil {
		t.Fatalf("load A2A Profile: %v", err)
	}
	headers := map[string]string{
		profile.ContextHeaders.TraceID:            "trace-1",
		profile.ContextHeaders.InvocationID:       "invocation-1",
		profile.ContextHeaders.RootTaskID:         "root-task-1",
		profile.ContextHeaders.ParentInvocationID: "parent-invocation-1",
		profile.ContextHeaders.WorkspaceID:        "workspace-1",
	}
	recorder := &contextRecordingHandler{RequestHandler: NewHandler(), calls: make(chan recordedCall, 1)}
	server := httptest.NewServer(a2asrv.NewJSONRPCHandler(recorder))
	t.Cleanup(server.Close)
	meta := make(a2aclient.CallMeta, len(headers))
	for name, value := range headers {
		meta[name] = []string{value}
	}
	client := newA2AClient(t, server, []a2aclient.CallInterceptor{a2aclient.NewStaticCallMetaInjector(meta)})
	if _, err := client.SendMessage(t.Context(), fixtureParams("headers", fixtureSuccess, "value")); err != nil {
		t.Fatalf("send with headers: %v", err)
	}
	call := <-recorder.calls
	if call.method != "message/send" {
		t.Fatalf("recorded method = %q", call.method)
	}
	for name, value := range headers {
		values, exists := call.meta.Get(name)
		if !exists || len(values) != 1 || values[0] != value {
			t.Errorf("header %s = (%v, %v), want %q", name, values, exists, value)
		}
	}
}

func TestOfficialServerUsesStrictOneLineSSEFrames(t *testing.T) {
	server := httptest.NewServer(NewHTTPHandler(NewHandler()))
	t.Cleanup(server.Close)
	payload := `{"jsonrpc":"2.0","id":"raw-stream","method":"message/stream","params":{"message":{"kind":"message","messageId":"raw-stream-message","role":"user","parts":[{"kind":"data","data":{"fixture":"stream-success","value":"raw"}}]}}}`
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if mediaType := strings.Split(response.Header.Get("Content-Type"), ";")[0]; mediaType != "text/event-stream" {
		t.Fatalf("content type = %q", mediaType)
	}
	frames := bytes.Split(bytes.TrimSpace(body), []byte("\n\n"))
	if len(frames) != 5 {
		t.Fatalf("SSE frame count = %d, body = %s", len(frames), body)
	}
	for index, frame := range frames {
		lines := bytes.Split(frame, []byte("\n"))
		if len(lines) != 2 || !bytes.HasPrefix(lines[0], []byte("id: ")) || !bytes.HasPrefix(lines[1], []byte("data: ")) {
			t.Errorf("frame %d is not one id line followed by one data line: %q", index, frame)
		}
	}
}

type recordedCall struct {
	method string
	meta   *a2asrv.RequestMeta
}

type contextRecordingHandler struct {
	a2asrv.RequestHandler
	calls chan recordedCall
}

func (h *contextRecordingHandler) OnSendMessage(ctx context.Context, params *a2a.MessageSendParams) (a2a.SendMessageResult, error) {
	callContext, ok := a2asrv.CallContextFrom(ctx)
	if !ok {
		return nil, errors.New("A2A call context is missing")
	}
	h.calls <- recordedCall{method: "message/send", meta: callContext.RequestMeta()}
	return h.RequestHandler.OnSendMessage(ctx, params)
}

func newA2AClient(t *testing.T, server *httptest.Server, interceptors []a2aclient.CallInterceptor) *a2aclient.Client {
	t.Helper()
	options := []a2aclient.FactoryOption{a2aclient.WithJSONRPCTransport(server.Client())}
	if len(interceptors) > 0 {
		options = append(options, a2aclient.WithInterceptors(interceptors...))
	}
	client, err := a2aclient.NewFromEndpoints(t.Context(), []a2a.AgentInterface{
		{URL: server.URL, Transport: a2a.TransportProtocolJSONRPC},
	}, options...)
	if err != nil {
		t.Fatalf("create A2A client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Destroy(); err != nil {
			t.Errorf("destroy A2A client: %v", err)
		}
	})
	return client
}
