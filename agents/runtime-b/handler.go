package runtimeb

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

type runtimeTask struct {
	task   *a2a.Task
	cancel chan struct{}
}

// Handler implements the active A2A Profile for the deterministic Runtime B sample.
type Handler struct {
	mu    sync.RWMutex
	tasks map[a2a.TaskID]*runtimeTask
}

var _ a2asrv.RequestHandler = (*Handler)(nil)

func NewHandler() *Handler {
	return &Handler{tasks: make(map[a2a.TaskID]*runtimeTask)}
}

func (h *Handler) OnSendMessage(_ context.Context, params *a2a.MessageSendParams) (a2a.SendMessageResult, error) {
	request, err := parseFixture(params)
	if err != nil {
		return nil, err
	}
	switch request.kind {
	case fixtureSuccess:
		return successMessage(params.Message, request), nil
	case fixtureFailure:
		return nil, errFixtureFailure
	case fixtureStreamSuccess, fixtureHold:
		return nil, invalidParams("fixture requires message/stream")
	default:
		return nil, invalidParams("fixture is not supported")
	}
}

func (h *Handler) OnSendMessageStream(ctx context.Context, params *a2a.MessageSendParams) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		request, err := parseFixture(params)
		if err != nil {
			yield(nil, err)
			return
		}
		if request.kind == fixtureFailure {
			yield(nil, errFixtureFailure)
			return
		}
		if request.kind != fixtureStreamSuccess && request.kind != fixtureHold {
			yield(nil, invalidParams("fixture requires message/send"))
			return
		}

		task, err := h.createWorkingTask(params.Message)
		if err != nil {
			yield(nil, err)
			return
		}
		terminal := false
		defer func() {
			if !terminal {
				h.removeWorkingTask(task.task.ID)
			}
		}()

		if !yield(cloneTask(task.task), nil) {
			return
		}
		if request.kind == fixtureHold {
			select {
			case <-ctx.Done():
				return
			case <-task.cancel:
			}
			terminal = true
			yield(statusEvent(task.task, a2a.TaskStateCanceled, true), nil)
			return
		}

		if !yield(streamMessage(task.task, request), nil) {
			return
		}
		artifactID := a2a.ArtifactID(derivedID("artifact", params.Message.ID))
		if !yield(artifactEvent(task.task, artifactID, request, false, false, 0), nil) {
			return
		}
		if !yield(artifactEvent(task.task, artifactID, request, true, true, 1), nil) {
			return
		}
		if err := h.completeTask(task.task.ID); err != nil {
			yield(nil, err)
			return
		}
		terminal = true
		yield(statusEvent(task.task, a2a.TaskStateCompleted, true), nil)
	}
}

func (h *Handler) OnGetTask(_ context.Context, query *a2a.TaskQueryParams) (*a2a.Task, error) {
	if query == nil || query.ID == "" {
		return nil, invalidParams("task id is required")
	}
	if query.HistoryLength == nil || *query.HistoryLength < 0 {
		return nil, invalidParams("a non-negative historyLength is required")
	}

	h.mu.RLock()
	stored, exists := h.tasks[query.ID]
	if !exists {
		h.mu.RUnlock()
		return nil, a2a.ErrTaskNotFound
	}
	task := cloneTask(stored.task)
	h.mu.RUnlock()

	historyLength := *query.HistoryLength
	if historyLength > len(task.History) {
		return nil, invalidParams("historyLength exceeds available history")
	}
	task.History = task.History[len(task.History)-historyLength:]
	return task, nil
}

func (h *Handler) OnCancelTask(_ context.Context, params *a2a.TaskIDParams) (*a2a.Task, error) {
	if params == nil || params.ID == "" {
		return nil, invalidParams("task id is required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	stored, exists := h.tasks[params.ID]
	if !exists {
		return nil, a2a.ErrTaskNotFound
	}
	if stored.task.Status.State != a2a.TaskStateWorking {
		return nil, a2a.ErrTaskNotCancelable
	}
	stored.task.Status = a2a.TaskStatus{State: a2a.TaskStateCanceled}
	close(stored.cancel)
	return cloneTask(stored.task), nil
}

func (h *Handler) createWorkingTask(message *a2a.Message) (*runtimeTask, error) {
	taskID := a2a.TaskID(derivedID("task", message.ID))
	contextID := message.ContextID
	if contextID == "" {
		contextID = derivedID("context", message.ID)
	}
	stored := &runtimeTask{
		task: &a2a.Task{
			ID:        taskID,
			ContextID: contextID,
			Status:    a2a.TaskStatus{State: a2a.TaskStateWorking},
			History:   []*a2a.Message{cloneMessage(message)},
		},
		cancel: make(chan struct{}),
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.tasks[taskID]; exists {
		return nil, invalidParams("messageId already identifies a runtime task")
	}
	h.tasks[taskID] = stored
	return stored, nil
}

func (h *Handler) completeTask(taskID a2a.TaskID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	stored, exists := h.tasks[taskID]
	if !exists {
		return fmt.Errorf("runtime task disappeared")
	}
	if stored.task.Status.State != a2a.TaskStateWorking {
		return fmt.Errorf("runtime task is not working")
	}
	stored.task.Status = a2a.TaskStatus{State: a2a.TaskStateCompleted}
	return nil
}

func (h *Handler) removeWorkingTask(taskID a2a.TaskID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	stored, exists := h.tasks[taskID]
	if exists && stored.task.Status.State == a2a.TaskStateWorking {
		delete(h.tasks, taskID)
	}
}

func successMessage(input *a2a.Message, request fixtureRequest) *a2a.Message {
	contextID := input.ContextID
	if contextID == "" {
		contextID = derivedID("context", input.ID)
	}
	return &a2a.Message{
		ID:        derivedID("message", input.ID),
		ContextID: contextID,
		Role:      a2a.MessageRoleAgent,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{
			"agent":   "runtime-b",
			"fixture": string(request.kind),
			"value":   request.value,
		}}},
	}
}

func streamMessage(task *a2a.Task, request fixtureRequest) *a2a.Message {
	return &a2a.Message{
		ID:        derivedID("stream-message", string(task.ID)),
		TaskID:    task.ID,
		ContextID: task.ContextID,
		Role:      a2a.MessageRoleAgent,
		Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{
			"agent":   "runtime-b",
			"fixture": string(request.kind),
			"phase":   "working",
			"value":   request.value,
		}}},
	}
}

func artifactEvent(task *a2a.Task, artifactID a2a.ArtifactID, request fixtureRequest, appendPart, last bool, sequence int) *a2a.TaskArtifactUpdateEvent {
	return &a2a.TaskArtifactUpdateEvent{
		TaskID:    task.ID,
		ContextID: task.ContextID,
		Append:    appendPart,
		LastChunk: last,
		Artifact: &a2a.Artifact{
			ID:   artifactID,
			Name: "runtime-b-result",
			Parts: []a2a.Part{a2a.DataPart{Data: map[string]any{
				"agent":    "runtime-b",
				"fixture":  string(request.kind),
				"sequence": sequence,
				"value":    request.value,
			}}},
		},
	}
}

func statusEvent(task *a2a.Task, state a2a.TaskState, final bool) *a2a.TaskStatusUpdateEvent {
	return &a2a.TaskStatusUpdateEvent{
		TaskID:    task.ID,
		ContextID: task.ContextID,
		Status:    a2a.TaskStatus{State: state},
		Final:     final,
	}
}

func cloneTask(task *a2a.Task) *a2a.Task {
	cloned := *task
	cloned.Metadata = cloneJSONMap(task.Metadata)
	cloned.Status = cloneTaskStatus(task.Status)
	cloned.History = make([]*a2a.Message, len(task.History))
	for index, message := range task.History {
		cloned.History[index] = cloneMessage(message)
	}
	if task.Artifacts != nil {
		cloned.Artifacts = make([]*a2a.Artifact, len(task.Artifacts))
		for index, artifact := range task.Artifacts {
			cloned.Artifacts[index] = cloneArtifact(artifact)
		}
	}
	return &cloned
}

func cloneTaskStatus(status a2a.TaskStatus) a2a.TaskStatus {
	cloned := status
	if status.Message != nil {
		cloned.Message = cloneMessage(status.Message)
	}
	if status.Timestamp != nil {
		timestamp := *status.Timestamp
		cloned.Timestamp = &timestamp
	}
	return cloned
}

func cloneArtifact(artifact *a2a.Artifact) *a2a.Artifact {
	cloned := *artifact
	cloned.Extensions = append([]string(nil), artifact.Extensions...)
	cloned.Metadata = cloneJSONMap(artifact.Metadata)
	cloned.Parts = cloneParts(artifact.Parts)
	return &cloned
}

func cloneMessage(message *a2a.Message) *a2a.Message {
	cloned := *message
	cloned.Extensions = append([]string(nil), message.Extensions...)
	cloned.Metadata = cloneJSONMap(message.Metadata)
	cloned.Parts = cloneParts(message.Parts)
	cloned.ReferenceTasks = append([]a2a.TaskID(nil), message.ReferenceTasks...)
	return &cloned
}

func cloneParts(parts a2a.ContentParts) a2a.ContentParts {
	cloned := make(a2a.ContentParts, len(parts))
	for index, part := range parts {
		cloned[index] = clonePart(part)
	}
	return cloned
}

func clonePart(part a2a.Part) a2a.Part {
	switch typed := part.(type) {
	case a2a.DataPart:
		cloned := typed
		cloned.Data = cloneJSONMap(typed.Data)
		cloned.Metadata = cloneJSONMap(typed.Metadata)
		return cloned
	case a2a.TextPart:
		cloned := typed
		cloned.Metadata = cloneJSONMap(typed.Metadata)
		return cloned
	case a2a.FilePart:
		cloned := typed
		cloned.Metadata = cloneJSONMap(typed.Metadata)
		cloned.File = cloneFilePartContent(typed.File)
		return cloned
	default:
		panic(fmt.Sprintf("unsupported a2a part type %T", part))
	}
}

func cloneFilePartContent(content a2a.FilePartContent) a2a.FilePartContent {
	switch typed := content.(type) {
	case a2a.FileBytes:
		return typed
	case a2a.FileURI:
		return typed
	default:
		panic(fmt.Sprintf("unsupported a2a file part content type %T", content))
	}
}

func cloneJSONMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned, err := cloneJSONValue(source)
	if err != nil {
		panic(fmt.Sprintf("non-json a2a map value: %v", err))
	}
	clonedMap, ok := cloned.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("cloned a2a map has type %T", cloned))
	}
	return clonedMap
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
