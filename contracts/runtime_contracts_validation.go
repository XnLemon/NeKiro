package contracts

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type InvocationResultMode string

const (
	InvocationResultModeJSON InvocationResultMode = "json"
	InvocationResultModeSSE  InvocationResultMode = "sse"
)

var (
	ErrRuntimeMediaNotAcceptable = errors.New("invocation result media is not acceptable")
	ErrRuntimeSequenceTerminal   = errors.New("invocation lifecycle already has a terminal event")
	ErrRuntimeStreamInterrupted  = errors.New("result stream ended before a terminal event")
	ErrRuntimeStreamClosed       = errors.New("result stream validation is closed")
)

var platformErrorV4Messages = map[PlatformErrorCode]string{
	ErrorCodeValidationError:         "The request is invalid.",
	ErrorCodeUnauthenticated:         "Authentication is required.",
	ErrorCodeForbidden:               "The requested operation is not allowed.",
	ErrorCodeNotFound:                "The requested resource was not found.",
	ErrorCodeConflict:                "The requested operation conflicts with current state.",
	ErrorCodeNotAcceptable:           "The requested result mode is not acceptable.",
	ErrorCodePayloadTooLarge:         "The payload is too large.",
	ErrorCodeAgentNotInstalled:       "The Agent is not installed in this Workspace.",
	ErrorCodeInstallationDisabled:    "The Agent installation is disabled.",
	ErrorCodeAgentDisabled:           "The Agent version is disabled.",
	ErrorCodeAgentReleaseUnpublished: "The Agent release is not published.",
	ErrorCodeAgentReleaseSuspended:   "The Agent release is suspended.",
	ErrorCodeAgentReleaseRevoked:     "The Agent release is revoked.",
	ErrorCodeCapabilityNotAllowed:    "The requested capability is not allowed.",
	ErrorCodeRouteNotFound:           "No route is available for the Agent.",
	ErrorCodeAgentAuthUnsupported:    "The Agent authentication type is not supported for invocation.",
	ErrorCodeAgentResponseTooLarge:   "The Agent response is too large.",
	ErrorCodeA2AProtocol:             "The Agent returned an invalid A2A response.",
	ErrorCodeAgentUnavailable:        "The Agent is unavailable.",
	ErrorCodeAgentExecutionFailed:    "The Agent failed to complete the invocation.",
	ErrorCodeDependency:              "A required platform dependency failed.",
	ErrorCodeTimeout:                 "The invocation timed out.",
	ErrorCodeCanceled:                "The invocation was canceled.",
	ErrorCodeInternal:                "The platform could not complete the request.",
}

type RuntimeContractValidator struct {
	preCorrelation *jsonschema.Schema
	correlated     *jsonschema.Schema
	event          *jsonschema.Schema
	streamEvent    *jsonschema.Schema
}

func NewRuntimeContractValidator() (*RuntimeContractValidator, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.RegisterFormat(&jsonschema.Format{
		Name: "semver",
		Validate: func(value any) error {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			if _, err := semver.StrictNewVersion(text); err != nil {
				return errors.New("invalid semantic version")
			}
			return nil
		},
	})
	for _, resource := range []struct{ id, path string }{
		{"https://schemas.nekiro.dev/common/v1", "schemas/common.v1.schema.json"},
		{"https://schemas.nekiro.dev/platform-error/v4", "schemas/platform-error.v4.schema.json"},
		{"https://schemas.nekiro.dev/invocation-event/v0.3", "schemas/invocation-event.v0.3.schema.json"},
		{"https://schemas.nekiro.dev/invocation-result-stream-event/v2", "schemas/invocation-result-stream-event.v2.schema.json"},
	} {
		document, err := readJSONDocument(resource.path)
		if err != nil {
			return nil, err
		}
		if err := compiler.AddResource(resource.id, document); err != nil {
			return nil, fmt.Errorf("add runtime contract schema %s: %w", resource.id, err)
		}
	}
	compile := func(id string) (*jsonschema.Schema, error) {
		schema, err := compiler.Compile(id)
		if err != nil {
			return nil, fmt.Errorf("compile runtime contract schema %s: %w", id, err)
		}
		return schema, nil
	}
	pre, err := compile("https://schemas.nekiro.dev/platform-error/v4#/$defs/preCorrelation")
	if err != nil {
		return nil, err
	}
	correlated, err := compile("https://schemas.nekiro.dev/platform-error/v4#/$defs/correlated")
	if err != nil {
		return nil, err
	}
	event, err := compile("https://schemas.nekiro.dev/invocation-event/v0.3")
	if err != nil {
		return nil, err
	}
	streamEvent, err := compile("https://schemas.nekiro.dev/invocation-result-stream-event/v2")
	if err != nil {
		return nil, err
	}
	return &RuntimeContractValidator{preCorrelation: pre, correlated: correlated, event: event, streamEvent: streamEvent}, nil
}

func (v *RuntimeContractValidator) ValidatePreCorrelationPlatformErrorV4(platformError PreCorrelationPlatformErrorV4) error {
	return validateMappedValue(v.preCorrelation, platformError)
}

func (v *RuntimeContractValidator) ValidatePreCorrelationPlatformErrorV4JSON(data []byte) error {
	return validateRuntimeJSON(v.preCorrelation, data)
}

func (v *RuntimeContractValidator) ValidateCorrelatedPlatformErrorV4(platformError CorrelatedPlatformErrorV4) error {
	return validateMappedValue(v.correlated, platformError)
}

func (v *RuntimeContractValidator) ValidateCorrelatedPlatformErrorV4JSON(data []byte) error {
	return validateRuntimeJSON(v.correlated, data)
}

func (v *RuntimeContractValidator) ValidateInvocationEventV03(event InvocationEventV03) error {
	if err := validateMappedValue(v.event, event); err != nil {
		return err
	}
	if err := ValidateInvocationReleaseProvenance(event.AgentReleaseID, event.AgentCardDigest); err != nil {
		return err
	}
	return validateRuntimeNestedErrorCorrelation(event.InvocationID, event.RootTaskID, event.TraceID, event.Error)
}

// ValidateInvocationReleaseProvenance keeps the additive release metadata
// truthful: legacy invocations carry neither field, while trusted invocations
// carry a safe Release ID and the canonical lower-case SHA-256 Card digest.
// The absent pair is the wire-level legacy/unverified encoding retained by
// Invocation Event 0.3; it is never inferred by Catalog for new versions.
func ValidateInvocationReleaseProvenance(releaseID, cardDigest string) error {
	if (releaseID == "") != (cardDigest == "") {
		return errors.New("invocation release provenance must contain both release ID and card digest")
	}
	if releaseID == "" {
		return nil
	}
	if err := validateSafeContractIdentifier("Agent Release ID", releaseID); err != nil {
		return err
	}
	if len(cardDigest) != 64 || cardDigest != strings.ToLower(cardDigest) {
		return errors.New("agent card digest must be canonical lower-case SHA-256")
	}
	decoded, err := hex.DecodeString(cardDigest)
	if err != nil || len(decoded) != 32 {
		return errors.New("agent card digest must be canonical lower-case SHA-256")
	}
	return nil
}

func (v *RuntimeContractValidator) ValidateInvocationResultStreamEventV2(event InvocationResultStreamEventV2) error {
	if err := validateMappedValue(v.streamEvent, event); err != nil {
		return err
	}
	return validateRuntimeNestedErrorCorrelation(event.InvocationID, event.RootTaskID, event.TraceID, event.Error)
}

func NewPreCorrelationPlatformErrorV4(code PlatformErrorCode, traceID TraceID) (PreCorrelationPlatformErrorV4, error) {
	message, exists := platformErrorV4Messages[code]
	if !exists {
		return PreCorrelationPlatformErrorV4{}, fmt.Errorf("unknown Platform Error v4 code %q", code)
	}
	if _, err := ParseTraceID(string(traceID)); err != nil {
		return PreCorrelationPlatformErrorV4{}, err
	}
	return PreCorrelationPlatformErrorV4{Code: code, Message: message, TraceID: traceID}, nil
}

func NewCorrelatedPlatformErrorV4(code PlatformErrorCode, traceID TraceID, invocationID, rootTaskID string) (CorrelatedPlatformErrorV4, error) {
	pre, err := NewPreCorrelationPlatformErrorV4(code, traceID)
	if err != nil {
		return CorrelatedPlatformErrorV4{}, err
	}
	if err := validateSafeContractIdentifier("invocation id", invocationID); err != nil {
		return CorrelatedPlatformErrorV4{}, err
	}
	if err := validateSafeContractIdentifier("root task id", rootTaskID); err != nil {
		return CorrelatedPlatformErrorV4{}, err
	}
	return CorrelatedPlatformErrorV4{Code: pre.Code, Message: pre.Message, TraceID: traceID, InvocationID: invocationID, RootTaskID: rootTaskID}, nil
}

func NegotiateInvocationResultMode(stream bool, accept string) (InvocationResultMode, error) {
	if stream {
		if accept == "text/event-stream" {
			return InvocationResultModeSSE, nil
		}
		return "", ErrRuntimeMediaNotAcceptable
	}
	switch accept {
	case "application/json", "application/*", "*/*":
		return InvocationResultModeJSON, nil
	default:
		return "", ErrRuntimeMediaNotAcceptable
	}
}

func ValidateNestedInvocationCorrelation(parent InvocationRecordV4, child InvocationEventV03) error {
	if parent.Status != "running" {
		return errors.New("nested Invocation parent must be running")
	}
	if child.Type != "created" || child.Status != "pending" || child.Sequence != 0 {
		return errors.New("nested Invocation child must begin with created/pending sequence zero")
	}
	if child.ParentInvocationID != parent.InvocationID || child.InvocationID == parent.InvocationID {
		return errors.New("nested Invocation parent identity is invalid")
	}
	if child.RootTaskID != parent.RootTaskID || child.TraceID != parent.TraceID || child.WorkspaceID != parent.WorkspaceID {
		return errors.New("nested Invocation lineage correlation changed")
	}
	if child.Caller.Type != "agent" || child.Caller.ID != parent.TargetAgentID {
		return errors.New("nested Invocation caller does not match parent target Agent")
	}
	return nil
}

type RuntimeInvocationSequenceValidator struct {
	contracts      *RuntimeContractValidator
	last           *InvocationEventV03
	nextSequence   int64
	nextChunkIndex int64
	terminal       bool
}

func NewRuntimeInvocationSequenceValidator(contracts *RuntimeContractValidator) (*RuntimeInvocationSequenceValidator, error) {
	if contracts == nil {
		return nil, errors.New("runtime contract validator is required")
	}
	return &RuntimeInvocationSequenceValidator{contracts: contracts}, nil
}

func (v *RuntimeInvocationSequenceValidator) Accept(event InvocationEventV03) error {
	if v.terminal {
		return ErrRuntimeSequenceTerminal
	}
	if err := v.contracts.ValidateInvocationEventV03(event); err != nil {
		return fmt.Errorf("validate Invocation Event 0.3: %w", err)
	}
	if event.Sequence != v.nextSequence {
		return fmt.Errorf("invocation event sequence must be %d", v.nextSequence)
	}
	if v.last == nil {
		if event.Type != "created" || event.Status != "pending" {
			return errors.New("invocation lifecycle must begin with created/pending")
		}
	} else {
		if !sameRuntimeInvocationContext(*v.last, event) {
			return errors.New("invocation lifecycle context changed")
		}
		if !validRuntimeTransition(v.last.Status, event.Type, event.Status) {
			return fmt.Errorf("invalid Invocation transition %s -> %s/%s", v.last.Status, event.Type, event.Status)
		}
	}
	if event.Type == "stream" {
		if event.ChunkIndex == nil || *event.ChunkIndex != v.nextChunkIndex {
			return fmt.Errorf("invocation stream chunk index must be %d", v.nextChunkIndex)
		}
		v.nextChunkIndex++
	}
	v.nextSequence++
	copy := event
	v.last = &copy
	v.terminal = isRuntimeTerminalStatus(event.Status)
	return nil
}

func (v *RuntimeInvocationSequenceValidator) IsTerminal() bool { return v.terminal }

type RuntimeResultStreamSequenceValidator struct {
	contracts      *RuntimeContractValidator
	invocationID   string
	rootTaskID     string
	traceID        TraceID
	nextSequence   int64
	nextChunkIndex int64
	terminal       bool
	closed         bool
}

func NewRuntimeResultStreamSequenceValidator(contracts *RuntimeContractValidator, invocationID, rootTaskID string, traceID TraceID) (*RuntimeResultStreamSequenceValidator, error) {
	if contracts == nil {
		return nil, errors.New("runtime contract validator is required")
	}
	if err := validateSafeContractIdentifier("invocation id", invocationID); err != nil {
		return nil, err
	}
	if err := validateSafeContractIdentifier("root task id", rootTaskID); err != nil {
		return nil, err
	}
	if _, err := ParseTraceID(string(traceID)); err != nil {
		return nil, err
	}
	return &RuntimeResultStreamSequenceValidator{contracts: contracts, invocationID: invocationID, rootTaskID: rootTaskID, traceID: traceID}, nil
}

func (v *RuntimeResultStreamSequenceValidator) Accept(event InvocationResultStreamEventV2) error {
	if v.closed {
		return ErrRuntimeStreamClosed
	}
	if v.terminal {
		return ErrRuntimeSequenceTerminal
	}
	if err := v.contracts.ValidateInvocationResultStreamEventV2(event); err != nil {
		return fmt.Errorf("validate Invocation Result Stream Event v2: %w", err)
	}
	if event.InvocationID != v.invocationID || event.RootTaskID != v.rootTaskID || event.TraceID != v.traceID {
		return errors.New("result stream correlation changed")
	}
	if event.Sequence != v.nextSequence {
		return fmt.Errorf("result stream sequence must be %d", v.nextSequence)
	}
	if v.nextSequence == 0 && event.Type != ResultStreamEventAccepted {
		return errors.New("result stream must begin with accepted")
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
	v.terminal = isResultStreamTerminal(event.Type)
	return nil
}

func (v *RuntimeResultStreamSequenceValidator) IsTerminal() bool { return v.terminal }

func (v *RuntimeResultStreamSequenceValidator) Finish() error {
	if v.closed {
		return ErrRuntimeStreamClosed
	}
	v.closed = true
	if !v.terminal {
		return ErrRuntimeStreamInterrupted
	}
	return nil
}

func (v *RuntimeContractValidator) ValidateInvocationDetailResponseV4(workspaceID string, detail InvocationDetailResponseV4) error {
	if err := validateInvocationRecordV4(detail.Invocation); err != nil {
		return fmt.Errorf("validate Invocation projection: %w", err)
	}
	if detail.Invocation.WorkspaceID != workspaceID {
		return errors.New("invocation projection Workspace does not match the authorized Workspace")
	}
	sequence, err := NewRuntimeInvocationSequenceValidator(v)
	if err != nil {
		return err
	}
	if len(detail.Events) == 0 {
		return errors.New("invocation detail requires at least one committed event")
	}
	for _, event := range detail.Events {
		if err := sequence.Accept(event); err != nil {
			return err
		}
		if event.InvocationID != detail.Invocation.InvocationID || event.RootTaskID != detail.Invocation.RootTaskID ||
			event.ParentInvocationID != detail.Invocation.ParentInvocationID || event.TraceID != detail.Invocation.TraceID ||
			event.Caller != detail.Invocation.Caller || event.WorkspaceID != detail.Invocation.WorkspaceID ||
			event.TargetAgentID != detail.Invocation.TargetAgentID || event.AgentCardVersion != detail.Invocation.AgentCardVersion ||
			event.AgentReleaseID != detail.Invocation.AgentReleaseID || event.AgentCardDigest != detail.Invocation.AgentCardDigest ||
			event.Capability != detail.Invocation.Capability {
			return errors.New("invocation projection context does not match its events")
		}
	}
	if detail.Invocation.Status != detail.Events[len(detail.Events)-1].Status {
		return errors.New("invocation projection status does not match its last event")
	}
	return nil
}

func ValidateTraceResponseV4(workspaceID string, traceID TraceID, response TraceResponseV4) error {
	if response.TraceID != traceID {
		return errors.New("trace response correlation changed")
	}
	if len(response.Invocations) == 0 {
		return errors.New("trace response requires non-empty Invocation lineage")
	}
	rootTaskID := response.Invocations[0].RootTaskID
	identities := make(map[string]struct{}, len(response.Invocations))
	for _, invocation := range response.Invocations {
		if err := validateInvocationRecordV4(invocation); err != nil {
			return fmt.Errorf("validate Trace Invocation projection: %w", err)
		}
		if invocation.WorkspaceID != workspaceID || invocation.TraceID != traceID {
			return errors.New("trace Invocation is outside the authorized Workspace or Trace")
		}
		if invocation.RootTaskID != rootTaskID {
			return errors.New("trace response changes root Task within one lineage")
		}
		if _, exists := identities[invocation.InvocationID]; exists {
			return errors.New("trace response repeats an Invocation")
		}
		if invocation.ParentInvocationID != "" {
			if invocation.ParentInvocationID == invocation.InvocationID {
				return errors.New("trace response contains a self-parent Invocation")
			}
			if _, exists := identities[invocation.ParentInvocationID]; !exists {
				return errors.New("trace response child must follow its existing parent")
			}
		}
		identities[invocation.InvocationID] = struct{}{}
	}
	return nil
}

// validateInvocationRecordV4 mirrors the active language-neutral
// InvocationRecordV4 schema. Projection reads are decoded into Go structs, so
// schema validation of the surrounding response cannot distinguish an omitted
// required field from its zero value; enforce those required fields and their
// primitive constraints explicitly before exposing a 200 response.
func validateInvocationRecordV4(record InvocationRecordV4) error {
	if err := ValidateInvocationReleaseProvenance(record.AgentReleaseID, record.AgentCardDigest); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"invocation id":   record.InvocationID,
		"root task id":    record.RootTaskID,
		"workspace id":    record.WorkspaceID,
		"target agent id": record.TargetAgentID,
		"capability":      record.Capability,
		"caller id":       record.Caller.ID,
	} {
		if err := validateSafeContractIdentifier(name, value); err != nil {
			return err
		}
	}
	if record.ParentInvocationID != "" {
		if err := validateSafeContractIdentifier("parent invocation id", record.ParentInvocationID); err != nil {
			return err
		}
	}
	if _, err := ParseTraceID(string(record.TraceID)); err != nil {
		return err
	}
	if record.Caller.Type != "user" && record.Caller.Type != "agent" && record.Caller.Type != "service" {
		return errors.New("caller type is invalid")
	}
	if _, err := semver.StrictNewVersion(record.AgentCardVersion); err != nil {
		return fmt.Errorf("agent card version is invalid: %w", err)
	}
	switch record.Status {
	case "pending", "routing", "running", "succeeded", "failed", "canceled", "timed_out":
	default:
		return errors.New("invocation status is invalid")
	}
	if record.LatencyMS != nil && *record.LatencyMS < 0 {
		return errors.New("invocation latency must be non-negative")
	}
	if record.ErrorCode != "" {
		if _, exists := platformErrorV4Messages[record.ErrorCode]; !exists {
			return errors.New("invocation error code is invalid")
		}
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("invocation projection timestamps are required")
	}
	return nil
}

func validRuntimeTransition(from, eventType, to string) bool {
	switch from {
	case "pending":
		return (eventType == "routing" && to == "routing") || ((eventType == "canceled" || eventType == "timed_out") && eventType == to)
	case "routing":
		return (eventType == "started" && to == "running") || ((eventType == "failed" || eventType == "canceled" || eventType == "timed_out") && eventType == to)
	case "running":
		return (eventType == "stream" && to == "running") ||
			((eventType == "succeeded" || eventType == "failed" || eventType == "canceled" || eventType == "timed_out") && eventType == to)
	default:
		return false
	}
}

func sameRuntimeInvocationContext(left, right InvocationEventV03) bool {
	return left.InvocationID == right.InvocationID && left.RootTaskID == right.RootTaskID &&
		left.ParentInvocationID == right.ParentInvocationID && left.TraceID == right.TraceID &&
		left.Caller == right.Caller && left.WorkspaceID == right.WorkspaceID &&
		left.TargetAgentID == right.TargetAgentID && left.AgentCardVersion == right.AgentCardVersion &&
		left.AgentReleaseID == right.AgentReleaseID && left.AgentCardDigest == right.AgentCardDigest &&
		left.Capability == right.Capability
}

func isRuntimeTerminalStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "canceled", "timed_out":
		return true
	default:
		return false
	}
}

func validateRuntimeJSON(schema *jsonschema.Schema, data []byte) error {
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return err
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return schema.Validate(document)
}

func validateRuntimeNestedErrorCorrelation(invocationID, rootTaskID string, traceID TraceID, platformError *PlatformErrorV4) error {
	if platformError == nil {
		return nil
	}
	if platformError.InvocationID != invocationID || platformError.RootTaskID != rootTaskID || platformError.TraceID != traceID {
		return errors.New("nested Platform Error v4 correlation changed")
	}
	return nil
}
