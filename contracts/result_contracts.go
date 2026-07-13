package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
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

type InvocationSemanticRuleID string

const InvocationRuleCorrelationMatches InvocationSemanticRuleID = "INV-CORR-001"

type InvocationContractKind string

const (
	InvocationContractEventV02         InvocationContractKind = "invocation-event-v0.2"
	InvocationContractStreamEventV1    InvocationContractKind = "invocation-result-stream-event-v1"
	invocationConformanceSchemaVersion                        = "1"
)

//go:embed invocation/v1/semantic-rules.md invocation/v1/conformance/*.json
var invocationContractFiles embed.FS

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

type InvocationSemanticValidationError struct {
	RuleID InvocationSemanticRuleID
}

func (validationError *InvocationSemanticValidationError) Error() string {
	return fmt.Sprintf("invocation correlation changed (%s)", validationError.RuleID)
}

func (platformError *PlatformErrorV2) UnmarshalJSON(data []byte) error {
	type wirePlatformErrorV2 PlatformErrorV2
	var decoded wirePlatformErrorV2
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{"code", "message", "traceId"},
		nil,
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

type ResolveAgentRequestV1 struct {
	InvocationID string  `json:"invocationId"`
	RootTaskID   string  `json:"rootTaskId"`
	TraceID      TraceID `json:"traceId"`
	WorkspaceID  string  `json:"workspaceId"`
	AgentID      string  `json:"agentId"`
	Version      string  `json:"version"`
	Capability   string  `json:"capability"`
}

func (request *ResolveAgentRequestV1) UnmarshalJSON(data []byte) error {
	type wireResolveAgentRequestV1 ResolveAgentRequestV1
	var decoded wireResolveAgentRequestV1
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{"invocationId", "rootTaskId", "traceId", "workspaceId", "agentId", "version", "capability"},
		nil,
		nil,
	); err != nil {
		return fmt.Errorf("decode Resolve Agent Request v1: %w", err)
	}
	value := ResolveAgentRequestV1(decoded)
	if err := ValidateResolveAgentRequestV1(value); err != nil {
		return fmt.Errorf("decode Resolve Agent Request v1: %w", err)
	}
	*request = value
	return nil
}

func ValidateResolveAgentRequestV1(request ResolveAgentRequestV1) error {
	identifiers := []struct {
		name  string
		value string
	}{
		{name: "invocation id", value: request.InvocationID},
		{name: "root task id", value: request.RootTaskID},
		{name: "workspace id", value: request.WorkspaceID},
		{name: "agent id", value: request.AgentID},
		{name: "capability", value: request.Capability},
	}
	for _, identifier := range identifiers {
		if err := validateSafeContractIdentifier(identifier.name, identifier.value); err != nil {
			return err
		}
	}
	if _, err := ParseTraceID(string(request.TraceID)); err != nil {
		return fmt.Errorf("invalid trace id")
	}
	if _, err := semver.StrictNewVersion(request.Version); err != nil {
		return fmt.Errorf("invalid Agent version")
	}
	return nil
}

func (result *InvocationResult) UnmarshalJSON(data []byte) error {
	type wireInvocationResult InvocationResult
	var decoded wireInvocationResult
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{"schemaVersion", "invocationId", "rootTaskId", "traceId", "status"},
		[]string{"result"},
		nil,
	); err != nil {
		return fmt.Errorf("decode Invocation Result: %w", err)
	}
	value := InvocationResult(decoded)
	validator, err := resultContractDecodeValidator()
	if err != nil {
		return err
	}
	if err := validator.ValidateInvocationResult(value); err != nil {
		return fmt.Errorf("decode Invocation Result: %w", err)
	}
	*result = value
	return nil
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
		nil,
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
		nil,
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

func (envelope *RouterEventEnvelopeV02) UnmarshalJSON(data []byte) error {
	type wireRouterEventEnvelopeV02 RouterEventEnvelopeV02
	var decoded wireRouterEventEnvelopeV02
	if err := unmarshalStrictResultContractObject(
		data,
		&decoded,
		[]string{"event"},
		nil,
		nil,
	); err != nil {
		return fmt.Errorf("decode Router Event Envelope v0.2: %w", err)
	}
	*envelope = RouterEventEnvelopeV02(decoded)
	return nil
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
	if err := validateMappedValue(v.invocationResultStreamEvent, event); err != nil {
		return err
	}
	return validateNestedPlatformErrorCorrelation(event.InvocationID, event.RootTaskID, event.TraceID, event.Error)
}

func (v *ResultContractValidator) ValidateInvocationEvent(event InvocationEventV02) error {
	if err := validateMappedValue(v.invocationEvent, event); err != nil {
		return err
	}
	return validateNestedPlatformErrorCorrelation(event.InvocationID, event.RootTaskID, event.TraceID, event.Error)
}

func (v *ResultContractValidator) ValidatePlatformError(platformError PlatformErrorV2) error {
	return validateMappedValue(v.platformError, platformError)
}

func (v *ResultContractValidator) ValidateResolveAgentErrorCorrelation(
	request ResolveAgentRequestV1,
	platformError PlatformErrorV2,
) error {
	if err := ValidateResolveAgentRequestV1(request); err != nil {
		return err
	}
	if err := v.ValidatePlatformError(platformError); err != nil {
		return err
	}
	if platformError.InvocationID != request.InvocationID || platformError.RootTaskID != request.RootTaskID || platformError.TraceID != request.TraceID {
		return errors.New("Resolve Agent error correlation changed")
	}
	return nil
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
	requiredNonNullableFields []string,
	requiredNullableFields []string,
	optionalNonNullableFields []string,
) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("contract value must be a JSON object")
	}
	for _, field := range requiredNonNullableFields {
		value, exists := fields[field]
		if !exists {
			return fmt.Errorf("required field %q is missing", field)
		}
		if isJSONNull(value) {
			return fmt.Errorf("required field %q must not be null", field)
		}
	}
	for _, field := range requiredNullableFields {
		if _, exists := fields[field]; !exists {
			return fmt.Errorf("required field %q is missing", field)
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

func validateNestedPlatformErrorCorrelation(
	invocationID string,
	rootTaskID string,
	traceID TraceID,
	platformError *PlatformErrorV2,
) error {
	if platformError == nil {
		return nil
	}
	if platformError.InvocationID != invocationID || platformError.RootTaskID != rootTaskID || platformError.TraceID != traceID {
		return &InvocationSemanticValidationError{RuleID: InvocationRuleCorrelationMatches}
	}
	return nil
}

type invocationConformanceManifest struct {
	SchemaVersion string
	Cases         []invocationConformanceCase
}

type invocationConformanceCase struct {
	ID            string
	ContractKind  InvocationContractKind
	File          string
	ExpectedValid bool
	ViolatedRules []InvocationSemanticRuleID
}

type invocationConformanceManifestJSON struct {
	SchemaVersion *string                          `json:"schemaVersion"`
	Cases         *[]invocationConformanceCaseJSON `json:"cases"`
}

type invocationConformanceCaseJSON struct {
	ID            *string                     `json:"id"`
	ContractKind  *InvocationContractKind     `json:"contractKind"`
	File          *string                     `json:"file"`
	ExpectedValid *bool                       `json:"expectedValid"`
	ViolatedRules *[]InvocationSemanticRuleID `json:"violatedRules"`
}

func loadInvocationConformanceManifest() (invocationConformanceManifest, error) {
	data, err := invocationContractFiles.ReadFile("invocation/v1/conformance/manifest.json")
	if err != nil {
		return invocationConformanceManifest{}, fmt.Errorf("read Invocation conformance manifest: %w", err)
	}
	manifest, err := decodeInvocationConformanceManifest(data)
	if err != nil {
		return invocationConformanceManifest{}, err
	}
	for _, manifestCase := range manifest.Cases {
		fixturePath := path.Join("invocation/v1/conformance", manifestCase.File)
		info, err := fs.Stat(invocationContractFiles, fixturePath)
		if err != nil {
			return invocationConformanceManifest{}, fmt.Errorf("Invocation conformance case %q fixture: %w", manifestCase.ID, err)
		}
		if !info.Mode().IsRegular() {
			return invocationConformanceManifest{}, fmt.Errorf("Invocation conformance case %q fixture is not a regular file", manifestCase.ID)
		}
	}
	return manifest, nil
}

func decodeInvocationConformanceManifest(data []byte) (invocationConformanceManifest, error) {
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return invocationConformanceManifest{}, fmt.Errorf("decode Invocation conformance manifest: %w", err)
	}
	var document invocationConformanceManifestJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return invocationConformanceManifest{}, fmt.Errorf("decode Invocation conformance manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return invocationConformanceManifest{}, fmt.Errorf("decode Invocation conformance manifest: %w", err)
	}
	if document.SchemaVersion == nil || *document.SchemaVersion != invocationConformanceSchemaVersion {
		return invocationConformanceManifest{}, fmt.Errorf("Invocation conformance manifest requires schemaVersion %q", invocationConformanceSchemaVersion)
	}
	if document.Cases == nil || len(*document.Cases) == 0 {
		return invocationConformanceManifest{}, errors.New("Invocation conformance manifest requires non-empty cases")
	}

	manifest := invocationConformanceManifest{
		SchemaVersion: *document.SchemaVersion,
		Cases:         make([]invocationConformanceCase, 0, len(*document.Cases)),
	}
	caseIDs := make(map[string]struct{}, len(*document.Cases))
	fixtureFiles := make(map[string]struct{}, len(*document.Cases))
	for index, wireCase := range *document.Cases {
		manifestCase, err := decodeInvocationConformanceCase(index, wireCase)
		if err != nil {
			return invocationConformanceManifest{}, err
		}
		if _, exists := caseIDs[manifestCase.ID]; exists {
			return invocationConformanceManifest{}, fmt.Errorf("Invocation conformance manifest repeats case id %q", manifestCase.ID)
		}
		if _, exists := fixtureFiles[manifestCase.File]; exists {
			return invocationConformanceManifest{}, fmt.Errorf("Invocation conformance manifest repeats fixture file %q", manifestCase.File)
		}
		caseIDs[manifestCase.ID] = struct{}{}
		fixtureFiles[manifestCase.File] = struct{}{}
		manifest.Cases = append(manifest.Cases, manifestCase)
	}
	return manifest, nil
}

func decodeInvocationConformanceCase(index int, wireCase invocationConformanceCaseJSON) (invocationConformanceCase, error) {
	if wireCase.ID == nil || *wireCase.ID == "" {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %d requires a non-empty id", index)
	}
	if !safeIdentifierPattern.MatchString(*wireCase.ID) {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %d has invalid id", index)
	}
	if wireCase.ContractKind == nil || !isInvocationContractKind(*wireCase.ContractKind) {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %q has invalid contractKind", *wireCase.ID)
	}
	if wireCase.File == nil {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %q is missing file", *wireCase.ID)
	}
	if err := validateInvocationConformanceFixturePath(*wireCase.File); err != nil {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %q file: %w", *wireCase.ID, err)
	}
	if wireCase.ExpectedValid == nil {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %q is missing expectedValid", *wireCase.ID)
	}
	if wireCase.ViolatedRules == nil {
		return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %q is missing violatedRules", *wireCase.ID)
	}
	if *wireCase.ExpectedValid && len(*wireCase.ViolatedRules) != 0 {
		return invocationConformanceCase{}, fmt.Errorf("valid Invocation conformance case %q declares violated rules", *wireCase.ID)
	}
	if !*wireCase.ExpectedValid && len(*wireCase.ViolatedRules) != 1 {
		return invocationConformanceCase{}, fmt.Errorf("invalid Invocation conformance case %q must declare exactly one violated rule", *wireCase.ID)
	}
	for _, ruleID := range *wireCase.ViolatedRules {
		if ruleID != InvocationRuleCorrelationMatches {
			return invocationConformanceCase{}, fmt.Errorf("Invocation conformance case %q declares unknown rule %q", *wireCase.ID, ruleID)
		}
	}
	return invocationConformanceCase{
		ID:            *wireCase.ID,
		ContractKind:  *wireCase.ContractKind,
		File:          *wireCase.File,
		ExpectedValid: *wireCase.ExpectedValid,
		ViolatedRules: *wireCase.ViolatedRules,
	}, nil
}

func readInvocationConformanceFixture(manifestCase invocationConformanceCase) ([]byte, error) {
	fixturePath := path.Join("invocation/v1/conformance", manifestCase.File)
	data, err := invocationContractFiles.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("read Invocation conformance case %q: %w", manifestCase.ID, err)
	}
	return data, nil
}

func evaluateInvocationCorrelationFixture(
	contractKind InvocationContractKind,
	data []byte,
) ([]InvocationSemanticRuleID, error) {
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return nil, fmt.Errorf("decode Invocation conformance fixture: %w", err)
	}
	validator, err := resultContractDecodeValidator()
	if err != nil {
		return nil, err
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode Invocation conformance fixture: %w", err)
	}

	var schema *jsonschema.Schema
	switch contractKind {
	case InvocationContractEventV02:
		schema = validator.invocationEvent
	case InvocationContractStreamEventV1:
		schema = validator.invocationResultStreamEvent
	default:
		return nil, fmt.Errorf("unsupported Invocation contract kind %q", contractKind)
	}
	if err := schema.Validate(document); err != nil {
		return nil, fmt.Errorf("validate Invocation conformance fixture schema: %w", err)
	}

	var envelope struct {
		InvocationID string           `json:"invocationId"`
		RootTaskID   string           `json:"rootTaskId"`
		TraceID      TraceID          `json:"traceId"`
		Error        *PlatformErrorV2 `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode Invocation conformance fixture correlation: %w", err)
	}
	if err := validateNestedPlatformErrorCorrelation(envelope.InvocationID, envelope.RootTaskID, envelope.TraceID, envelope.Error); err != nil {
		return []InvocationSemanticRuleID{InvocationRuleCorrelationMatches}, nil
	}
	return []InvocationSemanticRuleID{}, nil
}

func isInvocationContractKind(contractKind InvocationContractKind) bool {
	return contractKind == InvocationContractEventV02 || contractKind == InvocationContractStreamEventV1
}

func validateInvocationConformanceFixturePath(fixturePath string) error {
	if fixturePath == "" || !fs.ValidPath(fixturePath) {
		return errors.New("fixture path must be a non-empty canonical relative path")
	}
	if strings.Contains(fixturePath, "\\") {
		return errors.New("fixture path must use forward slashes")
	}
	if strings.ContainsAny(fixturePath, "%?#<>\"|*:") {
		return errors.New("fixture path contains a nonportable character")
	}
	for _, character := range fixturePath {
		if character <= 0x1f || character == 0x7f {
			return errors.New("fixture path contains an ASCII control character")
		}
	}
	for _, segment := range strings.Split(fixturePath, "/") {
		if strings.TrimRight(segment, " .") != segment {
			return errors.New("fixture path contains a platform-equivalent segment")
		}
		baseName := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		switch baseName {
		case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
			return errors.New("fixture path contains a Windows reserved basename")
		}
	}
	if path.Ext(fixturePath) != ".json" || fixturePath == "manifest.json" {
		return errors.New("fixture path must name a JSON fixture")
	}
	return nil
}
