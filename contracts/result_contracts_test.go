package contracts

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInvocationResultAndChunksPreserveArbitraryJSONValues(t *testing.T) {
	validator := mustResultContractValidator(t)
	testCases := []struct {
		name  string
		value string
	}{
		{name: "null", value: `null`},
		{name: "false", value: `false`},
		{name: "zero", value: `0`},
		{name: "string", value: `"answer"`},
		{name: "array", value: `[1,false,null,{"nested":"value"}]`},
		{name: "object", value: `{"answer":42,"items":["a","b"]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := InvocationResult{
				SchemaVersion: InvocationResultSchemaVersion,
				InvocationID:  "inv-result",
				RootTaskID:    "task-result",
				TraceID:       "trace-result",
				Status:        "succeeded",
				Result:        json.RawMessage(testCase.value),
			}
			if err := validator.ValidateInvocationResult(result); err != nil {
				t.Fatalf("validate Invocation Result: %v", err)
			}
			assertRawJSONFieldEquals(t, result, "result", testCase.value)

			sequence := mustResultStreamSequenceValidator(t, validator, "inv-stream", "task-stream", "trace-stream")
			if err := sequence.Accept(resultStreamEvent(ResultStreamEventAccepted, 0)); err != nil {
				t.Fatalf("accept stream start: %v", err)
			}
			chunkIndex := int64(0)
			chunk := resultStreamEvent(ResultStreamEventChunk, 1)
			chunk.ChunkIndex = &chunkIndex
			chunk.Chunk = json.RawMessage(testCase.value)
			if err := sequence.Accept(chunk); err != nil {
				t.Fatalf("accept result chunk: %v", err)
			}
			assertRawJSONFieldEquals(t, chunk, "chunk", testCase.value)
			if err := sequence.Accept(resultStreamEvent(ResultStreamEventCompleted, 2)); err != nil {
				t.Fatalf("accept stream completion: %v", err)
			}
			if err := sequence.Finish(); err != nil {
				t.Fatalf("finish complete stream: %v", err)
			}
		})
	}
}

func TestInvocationResultRequiresPresentJSONValue(t *testing.T) {
	validator := mustResultContractValidator(t)
	result := InvocationResult{
		SchemaVersion: InvocationResultSchemaVersion,
		InvocationID:  "inv-result",
		RootTaskID:    "task-result",
		TraceID:       "trace-result",
		Status:        "succeeded",
	}
	if err := validator.ValidateInvocationResult(result); err == nil {
		t.Fatal("Invocation Result accepted a missing JSON value")
	}

	result.Result = json.RawMessage("null")
	if err := validator.ValidateInvocationResult(result); err != nil {
		t.Fatalf("Invocation Result rejected explicit JSON null: %v", err)
	}
}

func TestStrictResultContractDecodingRejectsMissingNullAndUnknownFields(t *testing.T) {
	platformError, err := NewPlatformErrorV2(ErrorCodeInternal, "trace-decode")
	if err != nil {
		t.Fatalf("create Platform Error v2: %v", err)
	}
	chunkIndex := int64(0)
	chunkEvent := resultStreamEvent(ResultStreamEventChunk, 1)
	chunkEvent.ChunkIndex = &chunkIndex
	chunkEvent.Chunk = json.RawMessage(`{"piece":1}`)
	streamFailureError, err := NewCorrelatedPlatformErrorV2(
		ErrorCodeAgentExecutionFailed,
		"trace-stream",
		"inv-stream",
		"task-stream",
	)
	if err != nil {
		t.Fatalf("create stream failure error: %v", err)
	}
	failedStreamEvent := resultStreamEvent(ResultStreamEventFailed, 1)
	failedStreamEvent.Error = &streamFailureError

	chunkBytes := int64(12)
	ledgerStreamEvent := validInvocationEventV02("stream", "running", nil)
	ledgerStreamEvent.ChunkIndex = &chunkIndex
	ledgerStreamEvent.ChunkBytes = &chunkBytes
	ledgerSucceededEvent := validInvocationEventV02("succeeded", "succeeded", nil)
	ledgerFailureError := mustCorrelatedPlatformErrorV2(t, ErrorCodeAgentExecutionFailed)
	ledgerFailedEvent := validInvocationEventV02("failed", "failed", &ledgerFailureError)
	ledgerStartedEvent := validInvocationEventV02("started", "running", nil)
	invocationResult := InvocationResult{
		SchemaVersion: InvocationResultSchemaVersion,
		InvocationID:  "inv-decode",
		RootTaskID:    "task-decode",
		TraceID:       "trace-decode",
		Status:        "succeeded",
		Result:        json.RawMessage(`{"ok":true}`),
	}
	routerEnvelope := RouterEventEnvelopeV02{Event: ledgerStartedEvent}
	resolveRequest := ResolveAgentRequestV1{
		InvocationID: "inv-resolve",
		RootTaskID:   "task-resolve",
		TraceID:      "trace-resolve",
		WorkspaceID:  "workspace-resolve",
		AgentID:      "agent-resolve",
		Version:      "1.2.3",
		Capability:   "answer",
	}

	testCases := []struct {
		name        string
		data        []byte
		destination func() any
	}{
		{
			name: "Invocation Result missing required result",
			data: mutateContractJSON(t, invocationResult, func(document map[string]any) {
				delete(document, "result")
			}),
			destination: func() any { return &InvocationResult{} },
		},
		{
			name: "Invocation Result null non-nullable traceId",
			data: mutateContractJSON(t, invocationResult, func(document map[string]any) {
				document["traceId"] = nil
			}),
			destination: func() any { return &InvocationResult{} },
		},
		{
			name: "Invocation Result unknown field",
			data: mutateContractJSON(t, invocationResult, func(document map[string]any) {
				document["replayToken"] = "unsupported"
			}),
			destination: func() any { return &InvocationResult{} },
		},
		{
			name:        "Invocation Result trailing JSON",
			data:        appendTrailingJSONObject(t, invocationResult),
			destination: func() any { return &InvocationResult{} },
		},
		{
			name: "Platform Error missing required traceId",
			data: mutateContractJSON(t, platformError, func(document map[string]any) {
				delete(document, "traceId")
			}),
			destination: func() any { return &PlatformErrorV2{} },
		},
		{
			name: "Platform Error null invocationId",
			data: mutateContractJSON(t, platformError, func(document map[string]any) {
				document["invocationId"] = nil
			}),
			destination: func() any { return &PlatformErrorV2{} },
		},
		{
			name: "Platform Error null rootTaskId",
			data: mutateContractJSON(t, platformError, func(document map[string]any) {
				document["rootTaskId"] = nil
			}),
			destination: func() any { return &PlatformErrorV2{} },
		},
		{
			name: "Platform Error unknown field",
			data: mutateContractJSON(t, platformError, func(document map[string]any) {
				document["details"] = "dependency detail"
			}),
			destination: func() any { return &PlatformErrorV2{} },
		},
		{
			name: "result stream event missing required rootTaskId",
			data: mutateContractJSON(t, chunkEvent, func(document map[string]any) {
				delete(document, "rootTaskId")
			}),
			destination: func() any { return &InvocationResultStreamEvent{} },
		},
		{
			name: "result stream event null chunkIndex",
			data: mutateContractJSON(t, chunkEvent, func(document map[string]any) {
				document["chunkIndex"] = nil
			}),
			destination: func() any { return &InvocationResultStreamEvent{} },
		},
		{
			name: "result stream event null error",
			data: mutateContractJSON(t, failedStreamEvent, func(document map[string]any) {
				document["error"] = nil
			}),
			destination: func() any { return &InvocationResultStreamEvent{} },
		},
		{
			name: "result stream event unknown field",
			data: mutateContractJSON(t, chunkEvent, func(document map[string]any) {
				document["cursor"] = "replay-token"
			}),
			destination: func() any { return &InvocationResultStreamEvent{} },
		},
		{
			name: "Invocation Event missing required invocationId",
			data: mutateContractJSON(t, ledgerStartedEvent, func(document map[string]any) {
				delete(document, "invocationId")
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Invocation Event null parentInvocationId",
			data: mutateContractJSON(t, ledgerStartedEvent, func(document map[string]any) {
				document["parentInvocationId"] = nil
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Invocation Event null chunkIndex",
			data: mutateContractJSON(t, ledgerStreamEvent, func(document map[string]any) {
				document["chunkIndex"] = nil
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Invocation Event null chunkBytes",
			data: mutateContractJSON(t, ledgerStreamEvent, func(document map[string]any) {
				document["chunkBytes"] = nil
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Invocation Event null latencyMs",
			data: mutateContractJSON(t, ledgerSucceededEvent, func(document map[string]any) {
				document["latencyMs"] = nil
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Invocation Event null error",
			data: mutateContractJSON(t, ledgerFailedEvent, func(document map[string]any) {
				document["error"] = nil
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Invocation Event unknown field",
			data: mutateContractJSON(t, ledgerStartedEvent, func(document map[string]any) {
				document["result"] = map[string]any{"secret": true}
			}),
			destination: func() any { return &InvocationEventV02{} },
		},
		{
			name: "Router Event Envelope missing required event",
			data: mutateContractJSON(t, routerEnvelope, func(document map[string]any) {
				delete(document, "event")
			}),
			destination: func() any { return &RouterEventEnvelopeV02{} },
		},
		{
			name: "Router Event Envelope null event",
			data: mutateContractJSON(t, routerEnvelope, func(document map[string]any) {
				document["event"] = nil
			}),
			destination: func() any { return &RouterEventEnvelopeV02{} },
		},
		{
			name: "Router Event Envelope unknown field",
			data: mutateContractJSON(t, routerEnvelope, func(document map[string]any) {
				document["result"] = "forbidden"
			}),
			destination: func() any { return &RouterEventEnvelopeV02{} },
		},
		{
			name: "Router Event Envelope malformed nested event",
			data: mutateContractJSON(t, routerEnvelope, func(document map[string]any) {
				document["event"] = map[string]any{"schemaVersion": InvocationEventV02SchemaVersion}
			}),
			destination: func() any { return &RouterEventEnvelopeV02{} },
		},
		{
			name:        "Router Event Envelope trailing JSON",
			data:        appendTrailingJSONObject(t, routerEnvelope),
			destination: func() any { return &RouterEventEnvelopeV02{} },
		},
		{
			name: "Resolve Agent Request missing invocationId",
			data: mutateContractJSON(t, resolveRequest, func(document map[string]any) {
				delete(document, "invocationId")
			}),
			destination: func() any { return &ResolveAgentRequestV1{} },
		},
		{
			name: "Resolve Agent Request null traceId",
			data: mutateContractJSON(t, resolveRequest, func(document map[string]any) {
				document["traceId"] = nil
			}),
			destination: func() any { return &ResolveAgentRequestV1{} },
		},
		{
			name: "Resolve Agent Request unknown field",
			data: mutateContractJSON(t, resolveRequest, func(document map[string]any) {
				document["replacementInvocationId"] = "inv-replacement"
			}),
			destination: func() any { return &ResolveAgentRequestV1{} },
		},
		{
			name:        "Resolve Agent Request trailing JSON",
			data:        appendTrailingJSONObject(t, resolveRequest),
			destination: func() any { return &ResolveAgentRequestV1{} },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := json.Unmarshal(testCase.data, testCase.destination()); err == nil {
				t.Fatalf("strict decode accepted invalid document: %s", testCase.data)
			}
		})
	}
}

func TestResolveAgentRequestV1PreservesExistingCorrelation(t *testing.T) {
	document := []byte(`{
		"invocationId":"inv-resolve",
		"rootTaskId":"task-root",
		"traceId":"trace-resolve",
		"workspaceId":"workspace-resolve",
		"agentId":"agent-resolve",
		"version":"1.2.3",
		"capability":"answer"
	}`)
	var request ResolveAgentRequestV1
	if err := json.Unmarshal(document, &request); err != nil {
		t.Fatalf("decode Resolve Agent Request v1: %v", err)
	}
	if request.InvocationID != "inv-resolve" || request.RootTaskID != "task-root" || request.TraceID != "trace-resolve" {
		t.Fatalf(
			"decoded Resolve Agent correlation = invocation %q root %q trace %q",
			request.InvocationID,
			request.RootTaskID,
			request.TraceID,
		)
	}
	if err := ValidateResolveAgentRequestV1(request); err != nil {
		t.Fatalf("validate Resolve Agent Request v1: %v", err)
	}
}

func TestResolveAgentErrorRequiresExactRequestCorrelation(t *testing.T) {
	validator := mustResultContractValidator(t)
	request := ResolveAgentRequestV1{
		InvocationID: "inv-resolve",
		RootTaskID:   "task-root",
		TraceID:      "trace-resolve",
		WorkspaceID:  "workspace-resolve",
		AgentID:      "agent-resolve",
		Version:      "1.2.3",
		Capability:   "answer",
	}
	platformError, err := NewCorrelatedPlatformErrorV2(
		ErrorCodeDependency,
		request.TraceID,
		request.InvocationID,
		request.RootTaskID,
	)
	if err != nil {
		t.Fatalf("create Resolve Agent error: %v", err)
	}
	if err := validator.ValidateResolveAgentErrorCorrelation(request, platformError); err != nil {
		t.Fatalf("matching Resolve Agent error rejected: %v", err)
	}

	testCases := []struct {
		name   string
		mutate func(*PlatformErrorV2)
	}{
		{name: "invocation id", mutate: func(value *PlatformErrorV2) { value.InvocationID = "inv-other" }},
		{name: "root task id", mutate: func(value *PlatformErrorV2) { value.RootTaskID = "task-other" }},
		{name: "trace id", mutate: func(value *PlatformErrorV2) { value.TraceID = "trace-other" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mismatched := platformError
			testCase.mutate(&mismatched)
			if err := validator.ValidateResolveAgentErrorCorrelation(request, mismatched); err == nil || !strings.Contains(err.Error(), "correlation changed") {
				t.Fatalf("mismatched %s error = %v, want exact-correlation rejection", testCase.name, err)
			}
		})
	}
}

func TestInvocationCorrelationConformanceCorpus(t *testing.T) {
	manifest, err := loadInvocationConformanceManifest()
	if err != nil {
		t.Fatalf("load Invocation conformance manifest: %v", err)
	}
	if len(manifest.Cases) != 8 {
		t.Fatalf("Invocation conformance cases = %d, want 8", len(manifest.Cases))
	}
	validator := mustResultContractValidator(t)
	validCases := 0
	invalidCases := 0

	for _, manifestCase := range manifest.Cases {
		t.Run(manifestCase.ID, func(t *testing.T) {
			data, err := readInvocationConformanceFixture(manifestCase)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			violatedRules, err := evaluateInvocationCorrelationFixture(manifestCase.ContractKind, data)
			if err != nil {
				t.Fatalf("evaluate fixture: %v", err)
			}
			if !reflect.DeepEqual(violatedRules, manifestCase.ViolatedRules) {
				t.Fatalf("violated rules = %v, want %v", violatedRules, manifestCase.ViolatedRules)
			}

			var decodeErr error
			switch manifestCase.ContractKind {
			case InvocationContractEventV02:
				var event InvocationEventV02
				decodeErr = json.Unmarshal(data, &event)
				if decodeErr == nil {
					decodeErr = validator.ValidateInvocationEvent(event)
				}
			case InvocationContractStreamEventV1:
				var event InvocationResultStreamEvent
				decodeErr = json.Unmarshal(data, &event)
				if decodeErr == nil {
					decodeErr = validator.ValidateInvocationResultStreamEvent(event)
				}
			default:
				t.Fatalf("unsupported contract kind %q", manifestCase.ContractKind)
			}

			if manifestCase.ExpectedValid {
				validCases++
				if decodeErr != nil {
					t.Fatalf("valid fixture rejected by Go DTO validation: %v", decodeErr)
				}
				return
			}
			invalidCases++
			var semanticError *InvocationSemanticValidationError
			if !errors.As(decodeErr, &semanticError) {
				t.Fatalf("invalid fixture error = %v, want Invocation semantic validation error", decodeErr)
			}
			if semanticError.RuleID != InvocationRuleCorrelationMatches {
				t.Fatalf("semantic rule = %q, want %q", semanticError.RuleID, InvocationRuleCorrelationMatches)
			}
		})
	}
	if validCases != 2 || invalidCases != 6 {
		t.Fatalf("Invocation conformance corpus has %d valid and %d invalid cases, want 2 and 6", validCases, invalidCases)
	}
}

func TestInvocationConformanceManifestStrictDecoding(t *testing.T) {
	validCase := `{"id":"case-one","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":true,"violatedRules":[]}`
	testCases := []struct {
		name string
		data string
	}{
		{name: "missing schema version", data: `{"cases":[` + validCase + `]}`},
		{name: "null cases", data: `{"schemaVersion":"1","cases":null}`},
		{name: "unknown manifest field", data: `{"schemaVersion":"1","cases":[` + validCase + `],"fallbackCases":[]}`},
		{name: "duplicate manifest member", data: `{"schemaVersion":"1","schemaVersion":"1","cases":[` + validCase + `]}`},
		{name: "missing case id", data: `{"schemaVersion":"1","cases":[{"contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":true,"violatedRules":[]}]}`},
		{name: "null expected validity", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":null,"violatedRules":[]}]}`},
		{name: "unknown case field", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":true,"violatedRules":[],"legacyFile":"old.json"}]}`},
		{name: "duplicate case member", data: `{"schemaVersion":"1","cases":[{"id":"case-one","id":"case-two","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":true,"violatedRules":[]}]}`},
		{name: "unknown contract kind", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.1","file":"case-one.json","expectedValid":true,"violatedRules":[]}]}`},
		{name: "unsafe fixture path", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.2","file":"../case-one.json","expectedValid":true,"violatedRules":[]}]}`},
		{name: "duplicate case id", data: `{"schemaVersion":"1","cases":[` + validCase + `,{"id":"case-one","contractKind":"invocation-result-stream-event-v1","file":"case-two.json","expectedValid":true,"violatedRules":[]}]}`},
		{name: "duplicate fixture file", data: `{"schemaVersion":"1","cases":[` + validCase + `,{"id":"case-two","contractKind":"invocation-result-stream-event-v1","file":"case-one.json","expectedValid":true,"violatedRules":[]}]}`},
		{name: "unknown rule", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":false,"violatedRules":["INV-CORR-999"]}]}`},
		{name: "valid case with violation", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":true,"violatedRules":["INV-CORR-001"]}]}`},
		{name: "invalid case without violation", data: `{"schemaVersion":"1","cases":[{"id":"case-one","contractKind":"invocation-event-v0.2","file":"case-one.json","expectedValid":false,"violatedRules":[]}]}`},
		{name: "trailing JSON", data: `{"schemaVersion":"1","cases":[` + validCase + `]} {}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := decodeInvocationConformanceManifest([]byte(testCase.data)); err == nil {
				t.Fatal("invalid Invocation conformance manifest was accepted")
			}
		})
	}
}

func TestStrictResultContractDecodingPreservesExplicitNullPayloads(t *testing.T) {
	var result InvocationResult
	if err := json.Unmarshal([]byte(`{
		"schemaVersion":"1",
		"invocationId":"inv-null",
		"rootTaskId":"task-null",
		"traceId":"trace-null",
		"status":"succeeded",
		"result":null
	}`), &result); err != nil {
		t.Fatalf("decode explicit null result: %v", err)
	}
	if string(result.Result) != "null" {
		t.Fatalf("decoded result = %q, want explicit null", result.Result)
	}
	if err := mustResultContractValidator(t).ValidateInvocationResult(result); err != nil {
		t.Fatalf("validate explicit null result: %v", err)
	}

	var chunk InvocationResultStreamEvent
	if err := json.Unmarshal([]byte(`{
		"schemaVersion":"1",
		"sequence":1,
		"type":"chunk",
		"status":"running",
		"invocationId":"inv-null",
		"rootTaskId":"task-null",
		"traceId":"trace-null",
		"chunkIndex":0,
		"chunk":null
	}`), &chunk); err != nil {
		t.Fatalf("decode explicit null chunk: %v", err)
	}
	if string(chunk.Chunk) != "null" {
		t.Fatalf("decoded chunk = %q, want explicit null", chunk.Chunk)
	}
}

func TestInvocationEventV02StrictDecodePreservesChildLineage(t *testing.T) {
	child := validInvocationEventV02("started", "running", nil)
	child.InvocationID = "inv-child"
	child.ParentInvocationID = "inv-parent"
	child.RootTaskID = "task-root"
	child.TraceID = "trace-root"
	child.Caller = Caller{Type: "agent", ID: "agent-parent"}

	data, err := json.Marshal(child)
	if err != nil {
		t.Fatalf("marshal child Invocation Event: %v", err)
	}
	var decoded InvocationEventV02
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("strict decode child Invocation Event: %v", err)
	}
	if decoded.ParentInvocationID != child.ParentInvocationID || decoded.RootTaskID != child.RootTaskID || decoded.TraceID != child.TraceID {
		t.Fatalf(
			"decoded lineage = parent %q root %q trace %q, want parent %q root %q trace %q",
			decoded.ParentInvocationID,
			decoded.RootTaskID,
			decoded.TraceID,
			child.ParentInvocationID,
			child.RootTaskID,
			child.TraceID,
		)
	}
	if err := mustResultContractValidator(t).ValidateInvocationEvent(decoded); err != nil {
		t.Fatalf("validate decoded child Invocation Event: %v", err)
	}
}

func TestResultStreamFirstTerminalWins(t *testing.T) {
	validator := mustResultContractValidator(t)
	sequence := mustResultStreamSequenceValidator(t, validator, "inv-stream", "task-stream", "trace-stream")
	if err := sequence.Accept(resultStreamEvent(ResultStreamEventAccepted, 0)); err != nil {
		t.Fatalf("accept stream start: %v", err)
	}
	if err := sequence.Accept(resultStreamEvent(ResultStreamEventCompleted, 1)); err != nil {
		t.Fatalf("accept first terminal event: %v", err)
	}

	timeoutError, err := NewCorrelatedPlatformErrorV2(ErrorCodeTimeout, "trace-stream", "inv-stream", "task-stream")
	if err != nil {
		t.Fatalf("create timeout error: %v", err)
	}
	lateTimeout := resultStreamEvent(ResultStreamEventTimedOut, 2)
	lateTimeout.Error = &timeoutError
	if err := sequence.Accept(lateTimeout); !errors.Is(err, ErrResultStreamTerminated) {
		t.Fatalf("late terminal error = %v, want ErrResultStreamTerminated", err)
	}
	if sequence.TerminalType() != ResultStreamEventCompleted {
		t.Fatalf("terminal type = %q, want completed", sequence.TerminalType())
	}
	if err := sequence.Finish(); err != nil {
		t.Fatalf("finish first-terminal-wins stream: %v", err)
	}
}

func TestResultStreamInterruptedClosesValidation(t *testing.T) {
	validator := mustResultContractValidator(t)
	sequence := mustResultStreamSequenceValidator(t, validator, "inv-stream", "task-stream", "trace-stream")
	if err := sequence.Accept(resultStreamEvent(ResultStreamEventAccepted, 0)); err != nil {
		t.Fatalf("accept stream start: %v", err)
	}
	chunkIndex := int64(0)
	chunk := resultStreamEvent(ResultStreamEventChunk, 1)
	chunk.ChunkIndex = &chunkIndex
	chunk.Chunk = json.RawMessage(`{"partial":true}`)
	if err := sequence.Accept(chunk); err != nil {
		t.Fatalf("accept partial chunk: %v", err)
	}
	outOfOrder := resultStreamEvent(ResultStreamEventCompleted, 3)
	if err := sequence.Accept(outOfOrder); err == nil || !strings.Contains(err.Error(), "sequence must be 2") {
		t.Fatalf("out-of-order error = %v, want sequence rejection", err)
	}
	if err := sequence.Finish(); !errors.Is(err, ErrResultStreamInterrupted) {
		t.Fatalf("interrupted finish error = %v, want ErrResultStreamInterrupted", err)
	}

	correctNextTerminal := resultStreamEvent(ResultStreamEventCompleted, 2)
	if err := sequence.Accept(correctNextTerminal); !errors.Is(err, ErrResultStreamClosed) {
		t.Fatalf("post-Finish terminal error = %v, want ErrResultStreamClosed", err)
	}
	if sequence.TerminalType() != "" {
		t.Fatalf("interrupted stream terminal type changed to %q", sequence.TerminalType())
	}
}

func TestPlatformErrorV2UsesFixedSecretSafeMessages(t *testing.T) {
	validator := mustResultContractValidator(t)
	for code, message := range platformErrorV2Messages {
		platformError, err := NewPlatformErrorV2(code, "trace-error")
		if err != nil {
			t.Fatalf("create %s error: %v", code, err)
		}
		if platformError.Message != message {
			t.Fatalf("message for %s = %q, want %q", code, platformError.Message, message)
		}
		if err := validator.ValidatePlatformError(platformError); err != nil {
			t.Fatalf("validate %s error: %v", code, err)
		}
	}

	correlated, err := NewCorrelatedPlatformErrorV2(
		ErrorCodeAgentExecutionFailed,
		"trace-error",
		"inv-error",
		"task-error",
	)
	if err != nil {
		t.Fatalf("create correlated error: %v", err)
	}
	if err := validator.ValidatePlatformError(correlated); err != nil {
		t.Fatalf("validate correlated error: %v", err)
	}
	encoded, err := json.Marshal(correlated)
	if err != nil {
		t.Fatalf("marshal correlated error: %v", err)
	}
	for _, forbidden := range []string{"secret", "token", "endpoint", "details", "stack", "result", "input"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("public error contains forbidden content %q: %s", forbidden, encoded)
		}
	}

	if _, err := NewPlatformErrorV2(PlatformErrorCode("UNKNOWN"), "trace-error"); err == nil {
		t.Fatal("unknown Platform Error v2 code was accepted")
	}
	if _, err := NewCorrelatedPlatformErrorV2(ErrorCodeInternal, "trace-error", "token=secret", "task-error"); err == nil {
		t.Fatal("unsafe invocation correlation was accepted")
	}

	invalidDocuments := []string{
		`{"code":"INTERNAL_ERROR","message":"raw database failure","traceId":"trace-error"}`,
		`{"code":"INTERNAL_ERROR","message":"The platform could not complete the request.","traceId":"trace-error","details":"token=secret"}`,
		`{"code":"INTERNAL_ERROR","message":"The platform could not complete the request.","traceId":"trace-error","invocationId":"inv-error"}`,
	}
	for _, document := range invalidDocuments {
		if err := validateResultJSONBytes(validator.platformError, []byte(document)); err == nil {
			t.Fatalf("invalid public error was accepted: %s", document)
		}
	}
}

func TestInvocationEventV02ContainsNoResultContent(t *testing.T) {
	validator := mustResultContractValidator(t)
	event := validInvocationEventV02("started", "running", nil)
	if err := validator.ValidateInvocationEvent(event); err != nil {
		t.Fatalf("validate metadata-only event: %v", err)
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	for _, forbidden := range []string{"input", "result", "chunk", "output"} {
		if _, exists := document[forbidden]; exists {
			t.Fatalf("Invocation Event contains forbidden field %q", forbidden)
		}
	}

	document["result"] = map[string]any{"secret": true}
	withResult, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal event with result: %v", err)
	}
	if err := validateResultJSONBytes(validator.invocationEvent, withResult); err == nil {
		t.Fatal("Invocation Event v0.2 accepted persisted result content")
	}
}

func assertRawJSONFieldEquals(t *testing.T, value any, field string, expected string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal mapped value: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode mapped value: %v", err)
	}
	var actualValue any
	if err := json.Unmarshal(document[field], &actualValue); err != nil {
		t.Fatalf("decode %s: %v", field, err)
	}
	var expectedValue any
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("decode expected JSON: %v", err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("%s = %#v, want %#v", field, actualValue, expectedValue)
	}
}

func validateResultJSONBytes(schema interface{ Validate(any) error }, data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	return schema.Validate(document)
}

func mutateContractJSON(t *testing.T, value any, mutate func(map[string]any)) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal contract value: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode contract value: %v", err)
	}
	mutate(document)
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal mutated contract value: %v", err)
	}
	return data
}

func appendTrailingJSONObject(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal contract value: %v", err)
	}
	return append(data, []byte(` {}`)...)
}

func mustResultContractValidator(t *testing.T) *ResultContractValidator {
	t.Helper()
	validator, err := NewResultContractValidator()
	if err != nil {
		t.Fatalf("create result contract validator: %v", err)
	}
	return validator
}

func mustResultStreamSequenceValidator(
	t *testing.T,
	validator *ResultContractValidator,
	invocationID string,
	rootTaskID string,
	traceID TraceID,
) *ResultStreamSequenceValidator {
	t.Helper()
	sequence, err := NewResultStreamSequenceValidator(validator, invocationID, rootTaskID, traceID)
	if err != nil {
		t.Fatalf("create result stream sequence validator: %v", err)
	}
	return sequence
}

func resultStreamEvent(eventType ResultStreamEventType, sequence int64) InvocationResultStreamEvent {
	return InvocationResultStreamEvent{
		SchemaVersion: InvocationResultStreamEventSchemaVersion,
		Sequence:      sequence,
		Type:          eventType,
		Status:        resultStreamStatus(eventType),
		InvocationID:  "inv-stream",
		RootTaskID:    "task-stream",
		TraceID:       "trace-stream",
	}
}

func resultStreamStatus(eventType ResultStreamEventType) string {
	switch eventType {
	case ResultStreamEventAccepted:
		return "pending"
	case ResultStreamEventChunk:
		return "running"
	case ResultStreamEventCompleted:
		return "succeeded"
	case ResultStreamEventFailed:
		return "failed"
	case ResultStreamEventCanceled:
		return "canceled"
	case ResultStreamEventTimedOut:
		return "timed_out"
	default:
		panic("unsupported result stream event type in test")
	}
}

func validInvocationEventV02(eventType string, status string, platformError *PlatformErrorV2) InvocationEventV02 {
	latency := int64(12)
	event := InvocationEventV02{
		SchemaVersion:    InvocationEventV02SchemaVersion,
		EventID:          "event-v02",
		Sequence:         3,
		OccurredAt:       time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC),
		Type:             eventType,
		Status:           status,
		InvocationID:     "inv-event",
		RootTaskID:       "task-event",
		TraceID:          "trace-event",
		Caller:           Caller{Type: "user", ID: "user-event"},
		WorkspaceID:      "workspace-event",
		TargetAgentID:    "agent-event",
		AgentCardVersion: "1.0.0",
		Capability:       "answer",
		Error:            platformError,
	}
	if eventType == "succeeded" || eventType == "failed" || eventType == "canceled" || eventType == "timed_out" {
		event.LatencyMS = &latency
	}
	return event
}
