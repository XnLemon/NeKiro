package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	InvocationResultSchemaVersion            = "1"
	InvocationResultStreamEventSchemaVersion = "1"
	InvocationEventV02SchemaVersion          = "0.2"

	ErrorCodeNotAcceptable PlatformErrorCode = "NOT_ACCEPTABLE"
)

type ResultStreamEventType string

const (
	ResultStreamEventAccepted  ResultStreamEventType = "accepted"
	ResultStreamEventChunk     ResultStreamEventType = "chunk"
	ResultStreamEventCompleted ResultStreamEventType = "completed"
	ResultStreamEventFailed    ResultStreamEventType = "failed"
	ResultStreamEventCanceled  ResultStreamEventType = "canceled"
	ResultStreamEventTimedOut  ResultStreamEventType = "timed_out"
)

var platformErrorV2Messages = map[PlatformErrorCode]string{
	ErrorCodeValidationError:      "The request is invalid.",
	ErrorCodeUnauthenticated:      "Authentication is required.",
	ErrorCodeForbidden:            "The requested operation is not allowed.",
	ErrorCodeNotFound:             "The requested resource was not found.",
	ErrorCodeConflict:             "The requested operation conflicts with current state.",
	ErrorCodeNotAcceptable:        "The requested result mode is not acceptable.",
	ErrorCodeAgentNotInstalled:    "The Agent is not installed in this Workspace.",
	ErrorCodeAgentDisabled:        "The Agent version is disabled.",
	ErrorCodeCapabilityNotAllowed: "The requested capability is not allowed.",
	ErrorCodeRouteNotFound:        "No route is available for the Agent.",
	ErrorCodeA2AProtocol:          "The Agent returned an invalid A2A response.",
	ErrorCodeAgentUnavailable:     "The Agent is unavailable.",
	ErrorCodeAgentExecutionFailed: "The Agent failed to complete the invocation.",
	ErrorCodeDependency:           "A required platform dependency failed.",
	ErrorCodeTimeout:              "The invocation timed out.",
	ErrorCodeCanceled:             "The invocation was canceled.",
	ErrorCodeInternal:             "The platform could not complete the request.",
}

type PlatformErrorV2 struct {
	Code         PlatformErrorCode `json:"code"`
	Message      string            `json:"message"`
	TraceID      TraceID           `json:"traceId"`
	InvocationID string            `json:"invocationId,omitempty"`
	RootTaskID   string            `json:"rootTaskId,omitempty"`
}

func (platformError *PlatformErrorV2) UnmarshalJSON(data []byte) error {
	type wirePlatformErrorV2 PlatformErrorV2
	var decoded wirePlatformErrorV2
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{"code", "message", "traceId"},
		[]string{"invocationId", "rootTaskId"},
	); err != nil {
		return fmt.Errorf("decode Platform Error v2: %w", err)
	}
	value := PlatformErrorV2(decoded)
	validator, err := resultContractDecodeValidator()
	if err != nil {
		return err
	}
	if err := validator.ValidatePlatformError(value); err != nil {
		return fmt.Errorf("decode Platform Error v2: %w", err)
	}
	*platformError = value
	return nil
}

func NewPlatformErrorV2(code PlatformErrorCode, traceID TraceID) (PlatformErrorV2, error) {
	if _, err := ParseTraceID(string(traceID)); err != nil {
		return PlatformErrorV2{}, fmt.Errorf("invalid trace id")
	}
	message, exists := platformErrorV2Messages[code]
	if !exists {
		return PlatformErrorV2{}, fmt.Errorf("unknown platform error code %q", code)
	}
	return PlatformErrorV2{Code: code, Message: message, TraceID: traceID}, nil
}

func NewCorrelatedPlatformErrorV2(
	code PlatformErrorCode,
	traceID TraceID,
	invocationID string,
	rootTaskID string,
) (PlatformErrorV2, error) {
	platformError, err := NewPlatformErrorV2(code, traceID)
	if err != nil {
		return PlatformErrorV2{}, err
	}
	if err := validateSafeContractIdentifier("invocation id", invocationID); err != nil {
		return PlatformErrorV2{}, err
	}
	if err := validateSafeContractIdentifier("root task id", rootTaskID); err != nil {
		return PlatformErrorV2{}, err
	}
	platformError.InvocationID = invocationID
	platformError.RootTaskID = rootTaskID
	return platformError, nil
}

type InvocationResult struct {
	SchemaVersion string          `json:"schemaVersion"`
	InvocationID  string          `json:"invocationId"`
	RootTaskID    string          `json:"rootTaskId"`
	TraceID       TraceID         `json:"traceId"`
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result"`
}

type InvocationResultStreamEvent struct {
	SchemaVersion string                `json:"schemaVersion"`
	Sequence      int64                 `json:"sequence"`
	Type          ResultStreamEventType `json:"type"`
	Status        string                `json:"status"`
	InvocationID  string                `json:"invocationId"`
	RootTaskID    string                `json:"rootTaskId"`
	TraceID       TraceID               `json:"traceId"`
	ChunkIndex    *int64                `json:"chunkIndex,omitempty"`
	Chunk         json.RawMessage       `json:"chunk,omitempty"`
	Error         *PlatformErrorV2      `json:"error,omitempty"`
}

func (event *InvocationResultStreamEvent) UnmarshalJSON(data []byte) error {
	type wireInvocationResultStreamEvent InvocationResultStreamEvent
	var decoded wireInvocationResultStreamEvent
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{"schemaVersion", "sequence", "type", "status", "invocationId", "rootTaskId", "traceId"},
		[]string{"chunkIndex", "error"},
	); err != nil {
		return fmt.Errorf("decode Invocation Result Stream Event: %w", err)
	}
	value := InvocationResultStreamEvent(decoded)
	validator, err := resultContractDecodeValidator()
	if err != nil {
		return err
	}
	if err := validator.ValidateInvocationResultStreamEvent(value); err != nil {
		return fmt.Errorf("decode Invocation Result Stream Event: %w", err)
	}
	*event = value
	return nil
}

type InvocationEventV02 struct {
	SchemaVersion      string           `json:"schemaVersion"`
	EventID            string           `json:"eventId"`
	Sequence           int64            `json:"sequence"`
	OccurredAt         time.Time        `json:"occurredAt"`
	Type               string           `json:"type"`
	Status             string           `json:"status"`
	InvocationID       string           `json:"invocationId"`
	RootTaskID         string           `json:"rootTaskId"`
	ParentInvocationID string           `json:"parentInvocationId,omitempty"`
	TraceID            TraceID          `json:"traceId"`
	Caller             Caller           `json:"caller"`
	WorkspaceID        string           `json:"workspaceId"`
	TargetAgentID      string           `json:"targetAgentId"`
	AgentCardVersion   string           `json:"agentCardVersion"`
	Capability         string           `json:"capability"`
	ChunkIndex         *int64           `json:"chunkIndex,omitempty"`
	ChunkBytes         *int64           `json:"chunkBytes,omitempty"`
	LatencyMS          *int64           `json:"latencyMs,omitempty"`
	Error              *PlatformErrorV2 `json:"error,omitempty"`
}

func (event *InvocationEventV02) UnmarshalJSON(data []byte) error {
	type wireInvocationEventV02 InvocationEventV02
	var decoded wireInvocationEventV02
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{
			"schemaVersion",
			"eventId",
			"sequence",
			"occurredAt",
			"type",
			"status",
			"invocationId",
			"rootTaskId",
			"traceId",
			"caller",
			"workspaceId",
			"targetAgentId",
			"agentCardVersion",
			"capability",
		},
		[]string{"parentInvocationId", "chunkIndex", "chunkBytes", "latencyMs", "error"},
	); err != nil {
		return fmt.Errorf("decode Invocation Event v0.2: %w", err)
	}
	value := InvocationEventV02(decoded)
	validator, err := resultContractDecodeValidator()
	if err != nil {
		return err
	}
	if err := validator.ValidateInvocationEvent(value); err != nil {
		return fmt.Errorf("decode Invocation Event v0.2: %w", err)
	}
	*event = value
	return nil
}

type RouterEventEnvelopeV02 struct {
	Event InvocationEventV02 `json:"event"`
}

const (
	invocationResultSchemaID            = "https://schemas.nekiro.dev/invocation-result/v1"
	invocationResultStreamEventSchemaID = "https://schemas.nekiro.dev/invocation-result-stream-event/v1"
	invocationEventV02SchemaID          = "https://schemas.nekiro.dev/invocation-event/v0.2"
	platformErrorV2SchemaID             = "https://schemas.nekiro.dev/platform-error/v2"
)

type ResultContractValidator struct {
	invocationResult            *jsonschema.Schema
	invocationResultStreamEvent *jsonschema.Schema
	invocationEvent             *jsonschema.Schema
	platformError               *jsonschema.Schema
}

var resultContractDecodeState struct {
	once      sync.Once
	validator *ResultContractValidator
	err       error
}

func resultContractDecodeValidator() (*ResultContractValidator, error) {
	resultContractDecodeState.once.Do(func() {
		resultContractDecodeState.validator, resultContractDecodeState.err = NewResultContractValidator()
	})
	if resultContractDecodeState.err != nil {
		return nil, fmt.Errorf("initialize result contract decoder: %w", resultContractDecodeState.err)
	}
	return resultContractDecodeState.validator, nil
}

func NewResultContractValidator() (*ResultContractValidator, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.RegisterFormat(&jsonschema.Format{
		Name: "semver-range",
		Validate: func(value any) error {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			if _, err := semver.NewConstraint(text); err != nil {
				return errors.New("invalid semantic version range")
			}
			return nil
		},
	})

	resources := []struct {
		id   string
		path string
	}{
		{commonSchemaID, "schemas/common.v1.schema.json"},
		{platformErrorV2SchemaID, "schemas/platform-error.v2.schema.json"},
		{invocationResultSchemaID, "schemas/invocation-result.v1.schema.json"},
		{invocationResultStreamEventSchemaID, "schemas/invocation-result-stream-event.v1.schema.json"},
		{invocationEventV02SchemaID, "schemas/invocation-event.v0.2.schema.json"},
	}

	for _, resource := range resources {
		document, err := readJSONDocument(resource.path)
		if err != nil {
			return nil, err
		}
		if err := compiler.AddResource(resource.id, document); err != nil {
			return nil, fmt.Errorf("add schema resource %s: %w", resource.id, err)
		}
	}

	invocationResult, err := compiler.Compile(invocationResultSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Invocation Result schema: %w", err)
	}
	invocationResultStreamEvent, err := compiler.Compile(invocationResultStreamEventSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Invocation Result Stream Event schema: %w", err)
	}
	invocationEvent, err := compiler.Compile(invocationEventV02SchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Invocation Event v0.2 schema: %w", err)
	}
	platformError, err := compiler.Compile(platformErrorV2SchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Platform Error v2 schema: %w", err)
	}

	return &ResultContractValidator{
		invocationResult:            invocationResult,
		invocationResultStreamEvent: invocationResultStreamEvent,
		invocationEvent:             invocationEvent,
		platformError:               platformError,
	}, nil
}

func (v *ResultContractValidator) ValidateInvocationResult(result InvocationResult) error {
	if result.Result == nil {
		return errors.New("invocation result JSON value is required")
	}
	return validateMappedValue(v.invocationResult, result)
}

func (v *ResultContractValidator) ValidateInvocationResultStreamEvent(event InvocationResultStreamEvent) error {
	return validateMappedValue(v.invocationResultStreamEvent, event)
}

func (v *ResultContractValidator) ValidateInvocationEvent(event InvocationEventV02) error {
	if err := validateMappedValue(v.invocationEvent, event); err != nil {
		return err
	}
	if event.Error != nil && (event.Error.InvocationID != event.InvocationID || event.Error.RootTaskID != event.RootTaskID || event.Error.TraceID != event.TraceID) {
		return errors.New("invocation event error correlation changed")
	}
	return nil
}

func (v *ResultContractValidator) ValidatePlatformError(platformError PlatformErrorV2) error {
	return validateMappedValue(v.platformError, platformError)
}

var (
	ErrResultStreamInterrupted = errors.New("result stream ended before a terminal event")
	ErrResultStreamTerminated  = errors.New("result stream already has a terminal event")
	ErrResultStreamClosed      = errors.New("result stream validation is closed")
)

type ResultStreamSequenceValidator struct {
	contracts      *ResultContractValidator
	invocationID   string
	rootTaskID     string
	traceID        TraceID
	nextSequence   int64
	nextChunkIndex int64
	terminal       ResultStreamEventType
	closed         bool
}

func NewResultStreamSequenceValidator(
	contracts *ResultContractValidator,
	invocationID string,
	rootTaskID string,
	traceID TraceID,
) (*ResultStreamSequenceValidator, error) {
	if contracts == nil {
		return nil, errors.New("result contract validator is required")
	}
	if err := validateSafeContractIdentifier("invocation id", invocationID); err != nil {
		return nil, err
	}
	if err := validateSafeContractIdentifier("root task id", rootTaskID); err != nil {
		return nil, err
	}
	if _, err := ParseTraceID(string(traceID)); err != nil {
		return nil, fmt.Errorf("invalid trace id")
	}
	return &ResultStreamSequenceValidator{
		contracts:    contracts,
		invocationID: invocationID,
		rootTaskID:   rootTaskID,
		traceID:      traceID,
	}, nil
}

func (v *ResultStreamSequenceValidator) Accept(event InvocationResultStreamEvent) error {
	if v.closed {
		return ErrResultStreamClosed
	}
	if v.terminal != "" {
		return ErrResultStreamTerminated
	}
	if err := v.contracts.ValidateInvocationResultStreamEvent(event); err != nil {
		return fmt.Errorf("validate result stream event: %w", err)
	}
	if event.InvocationID != v.invocationID || event.RootTaskID != v.rootTaskID || event.TraceID != v.traceID {
		return errors.New("result stream correlation changed")
	}
	if event.Sequence != v.nextSequence {
		return fmt.Errorf("result stream sequence must be %d", v.nextSequence)
	}
	if v.nextSequence == 0 && event.Type != ResultStreamEventAccepted {
		return errors.New("result stream must begin with an accepted event")
	}
	if v.nextSequence > 0 && event.Type == ResultStreamEventAccepted {
		return errors.New("result stream accepted event must be first")
	}
	if event.Type == ResultStreamEventChunk {
		if event.ChunkIndex == nil || *event.ChunkIndex != v.nextChunkIndex {
			return fmt.Errorf("result stream chunk index must be %d", v.nextChunkIndex)
		}
		v.nextChunkIndex++
	}
	if event.Error != nil && (event.Error.TraceID != v.traceID || event.Error.InvocationID != v.invocationID || event.Error.RootTaskID != v.rootTaskID) {
		return errors.New("result stream error correlation changed")
	}

	v.nextSequence++
	if isResultStreamTerminal(event.Type) {
		v.terminal = event.Type
	}
	return nil
}

func (v *ResultStreamSequenceValidator) Finish() error {
	v.closed = true
	if v.terminal == "" {
		return ErrResultStreamInterrupted
	}
	return nil
}

func (v *ResultStreamSequenceValidator) TerminalType() ResultStreamEventType {
	return v.terminal
}

func isResultStreamTerminal(eventType ResultStreamEventType) bool {
	switch eventType {
	case ResultStreamEventCompleted, ResultStreamEventFailed, ResultStreamEventCanceled, ResultStreamEventTimedOut:
		return true
	default:
		return false
	}
}

func validateSafeContractIdentifier(name string, value string) error {
	if !safeIdentifierPattern.MatchString(value) {
		return fmt.Errorf("invalid %s", name)
	}
	return nil
}

func unmarshalStrictResultContractObject(
	data []byte,
	destination any,
	requiredFields []string,
	optionalNonNullableFields []string,
) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("contract value must be a JSON object")
	}
	for _, field := range requiredFields {
		value, exists := fields[field]
		if !exists {
			return fmt.Errorf("required field %q is missing", field)
		}
		if isJSONNull(value) {
			return fmt.Errorf("required field %q must not be null", field)
		}
	}
	for _, field := range optionalNonNullableFields {
		if value, exists := fields[field]; exists && isJSONNull(value) {
			return fmt.Errorf("optional field %q must not be null", field)
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}
