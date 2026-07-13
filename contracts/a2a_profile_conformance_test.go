package contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	_ func(*a2aclient.Client, context.Context, *a2a.MessageSendParams) (a2a.SendMessageResult, error) = (*a2aclient.Client).SendMessage
	_ func(*a2aclient.Client, context.Context, *a2a.MessageSendParams) iter.Seq2[a2a.Event, error]    = (*a2aclient.Client).SendStreamingMessage
	_ func(*a2aclient.Client, context.Context, *a2a.TaskQueryParams) (*a2a.Task, error)               = (*a2aclient.Client).GetTask
	_ func(*a2aclient.Client, context.Context, *a2a.TaskIDParams) (*a2a.Task, error)                  = (*a2aclient.Client).CancelTask

	_ func(a2asrv.RequestHandler, context.Context, *a2a.MessageSendParams) (a2a.SendMessageResult, error) = a2asrv.RequestHandler.OnSendMessage
	_ func(a2asrv.RequestHandler, context.Context, *a2a.MessageSendParams) iter.Seq2[a2a.Event, error]    = a2asrv.RequestHandler.OnSendMessageStream
	_ func(a2asrv.RequestHandler, context.Context, *a2a.TaskQueryParams) (*a2a.Task, error)               = a2asrv.RequestHandler.OnGetTask
	_ func(a2asrv.RequestHandler, context.Context, *a2a.TaskIDParams) (*a2a.Task, error)                  = a2asrv.RequestHandler.OnCancelTask

	_ a2a.SendMessageResult = (*a2a.Message)(nil)
	_ a2a.SendMessageResult = (*a2a.Task)(nil)
	_ a2a.Event             = (*a2a.Message)(nil)
	_ a2a.Event             = (*a2a.Task)(nil)
	_ a2a.Event             = (*a2a.TaskStatusUpdateEvent)(nil)
	_ a2a.Event             = (*a2a.TaskArtifactUpdateEvent)(nil)
)

func TestA2AProfileConformanceMetadata(t *testing.T) {
	profile, err := LoadA2AProfileV02()
	if err != nil {
		t.Fatalf("load Profile v0.2: %v", err)
	}
	manifest, err := LoadA2AConformanceManifestV02()
	if err != nil {
		t.Fatalf("load conformance manifest: %v", err)
	}

	if profile.SchemaVersion != A2AProfileSchemaVersionV02 {
		t.Fatalf("profile schema version = %q, want %q", profile.SchemaVersion, A2AProfileSchemaVersionV02)
	}
	if profile.ProtocolVersion != A2AProfileProtocolVersion {
		t.Fatalf("protocol version = %q, want %q", profile.ProtocolVersion, A2AProfileProtocolVersion)
	}
	if profile.SDK.Module != A2AProfileSDKModule || profile.SDK.Version != A2AProfileSDKVersion {
		t.Fatalf("SDK pin = %s %s, want %s %s", profile.SDK.Module, profile.SDK.Version, A2AProfileSDKModule, A2AProfileSDKVersion)
	}
	if profile.Conformance.FixtureAuthority != "hand-authored" {
		t.Fatalf("fixture authority = %q, want hand-authored", profile.Conformance.FixtureAuthority)
	}

	wantMethods := map[string]bool{
		"message/send":   false,
		"message/stream": false,
		"tasks/get":      false,
		"tasks/cancel":   false,
	}
	for _, operation := range profile.Operations {
		if _, exists := wantMethods[operation.Method]; !exists {
			t.Fatalf("unexpected operation %q", operation.Method)
		}
		if wantMethods[operation.Method] {
			t.Fatalf("duplicate operation %q", operation.Method)
		}
		wantMethods[operation.Method] = true
	}
	for method, found := range wantMethods {
		if !found {
			t.Fatalf("required operation %q is missing", method)
		}
	}

	for _, states := range [][]A2AProfileTaskStateV02{
		profile.TaskStates.Transient,
		profile.TaskStates.Terminal,
		profile.TaskStates.Unsupported,
		{profile.TaskStates.Unspecified},
	} {
		for _, state := range states {
			if state.State == "timeout" {
				t.Fatal("timeout was declared as an A2A TaskState")
			}
		}
	}

	if manifest.ProfileSchemaVersion != profile.SchemaVersion || manifest.ProtocolVersion != profile.ProtocolVersion {
		t.Fatalf("manifest versions = profile %q protocol %q, want %q and %q", manifest.ProfileSchemaVersion, manifest.ProtocolVersion, profile.SchemaVersion, profile.ProtocolVersion)
	}
	caseIDs := make(map[string]struct{}, len(manifest.Cases))
	for _, testCase := range manifest.Cases {
		if _, exists := caseIDs[testCase.ID]; exists {
			t.Fatalf("duplicate manifest case id %q", testCase.ID)
		}
		caseIDs[testCase.ID] = struct{}{}
		if _, err := ReadA2AConformanceFixtureV02(testCase.File); err != nil {
			t.Fatalf("case %s fixture %q: %v", testCase.ID, testCase.File, err)
		}
		if testCase.RequestFile != "" {
			if _, err := ReadA2AConformanceFixtureV02(testCase.RequestFile); err != nil {
				t.Fatalf("case %s request fixture %q: %v", testCase.ID, testCase.RequestFile, err)
			}
		}
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	schemaDocument, err := readEmbeddedJSONDocument("schemas/a2a-profile.v0.2.schema.json")
	if err != nil {
		t.Fatalf("read Profile Schema v0.2: %v", err)
	}
	if err := compiler.AddResource("https://schemas.nekiro.dev/a2a-profile/v0.2", schemaDocument); err != nil {
		t.Fatalf("add Profile Schema v0.2: %v", err)
	}
	schema, err := compiler.Compile("https://schemas.nekiro.dev/a2a-profile/v0.2")
	if err != nil {
		t.Fatalf("compile Profile Schema v0.2: %v", err)
	}
	profileDocument, err := readEmbeddedJSONDocument("a2a-profile/v0.3.0/profile.v0.2.json")
	if err != nil {
		t.Fatalf("read Profile v0.2: %v", err)
	}
	if err := schema.Validate(profileDocument); err != nil {
		t.Fatalf("Profile v0.2 does not match its schema: %v", err)
	}
}

func TestA2AProfileConformanceTaskStateMapping(t *testing.T) {
	tests := []struct {
		state          a2a.TaskState
		classification A2ATaskStateClassification
		status         string
		errorCode      PlatformErrorCode
	}{
		{a2a.TaskStateSubmitted, A2ATaskStateTransient, "running", ""},
		{a2a.TaskStateWorking, A2ATaskStateTransient, "running", ""},
		{a2a.TaskStateCompleted, A2ATaskStateTerminal, "succeeded", ""},
		{a2a.TaskStateFailed, A2ATaskStateTerminal, "failed", ErrorCodeAgentExecutionFailed},
		{a2a.TaskStateCanceled, A2ATaskStateTerminal, "canceled", ErrorCodeCanceled},
		{a2a.TaskStateRejected, A2ATaskStateTerminal, "failed", ErrorCodeAgentExecutionFailed},
	}
	for _, testCase := range tests {
		t.Run(string(testCase.state), func(t *testing.T) {
			mapping, err := MapA2ATaskState(testCase.state)
			if err != nil {
				t.Fatalf("MapA2ATaskState(%q): %v", testCase.state, err)
			}
			if mapping.Classification != testCase.classification || mapping.InvocationStatus != testCase.status || mapping.ErrorCode != testCase.errorCode {
				t.Fatalf("mapping for %q = %+v", testCase.state, mapping)
			}
		})
	}

	for _, state := range []a2a.TaskState{
		a2a.TaskStateAuthRequired,
		a2a.TaskStateInputRequired,
		a2a.TaskStateUnknown,
		a2a.TaskStateUnspecified,
		a2a.TaskState("paused-by-provider"),
	} {
		t.Run("reject-"+string(state), func(t *testing.T) {
			_, err := MapA2ATaskState(state)
			var stateError *A2AProfileStateError
			if !errors.As(err, &stateError) {
				t.Fatalf("MapA2ATaskState(%q) error = %v, want A2AProfileStateError", state, err)
			}
			if stateError.ErrorCode != ErrorCodeA2AProtocol {
				t.Fatalf("MapA2ATaskState(%q) error code = %q, want %q", state, stateError.ErrorCode, ErrorCodeA2AProtocol)
			}
		})
	}

	for name, task := range map[string]*a2a.Task{
		"nil":             nil,
		"zero":            {},
		"missing context": {ID: "task-1", Status: a2a.TaskStatus{State: a2a.TaskStateWorking}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateA2ATask(task)
			var taskError *A2AProfileTaskError
			if !errors.As(err, &taskError) {
				t.Fatalf("ValidateA2ATask() error = %v, want A2AProfileTaskError", err)
			}
			if taskError.ErrorCode != ErrorCodeA2AProtocol {
				t.Fatalf("ValidateA2ATask() error code = %q, want %q", taskError.ErrorCode, ErrorCodeA2AProtocol)
			}
		})
	}
}

func TestA2AProfileConformanceMessageResult(t *testing.T) {
	tests := []struct {
		name    string
		message *a2a.Message
		valid   bool
	}{
		{name: "valid", message: &a2a.Message{ID: "message-1", Role: a2a.MessageRoleAgent, Parts: a2a.ContentParts{a2a.TextPart{Text: "result"}}}, valid: true},
		{name: "nil", message: nil},
		{name: "empty id", message: &a2a.Message{Role: a2a.MessageRoleAgent, Parts: a2a.ContentParts{a2a.TextPart{Text: "result"}}}},
		{name: "user role", message: &a2a.Message{ID: "message-1", Role: a2a.MessageRoleUser, Parts: a2a.ContentParts{a2a.TextPart{Text: "result"}}}},
		{name: "no parts", message: &a2a.Message{ID: "message-1", Role: a2a.MessageRoleAgent}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateA2AMessageResult(testCase.message)
			if testCase.valid && err != nil {
				t.Fatalf("valid Agent Message rejected: %v", err)
			}
			if !testCase.valid {
				var messageError *A2AProfileMessageError
				if !errors.As(err, &messageError) {
					t.Fatalf("message error = %v, want A2AProfileMessageError", err)
				}
				if messageError.ErrorCode != ErrorCodeA2AProtocol {
					t.Fatalf("message error code = %q, want %q", messageError.ErrorCode, ErrorCodeA2AProtocol)
				}
			}
		})
	}
}

func TestA2AProfileConformanceManifestStrictDecoding(t *testing.T) {
	validCase := `{"id":"request","file":"request.json","operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`
	validManifest := manifestDocument(validCase)
	tests := []struct {
		name     string
		document string
	}{
		{name: "missing root fields", document: `{}`},
		{name: "null schemaVersion", document: `{"schemaVersion":null,"profileSchemaVersion":"0.2","protocolVersion":"0.3.0","cases":[` + validCase + `]}`},
		{name: "null cases", document: `{"schemaVersion":"0.1","profileSchemaVersion":"0.2","protocolVersion":"0.3.0","cases":null}`},
		{name: "empty cases", document: `{"schemaVersion":"0.1","profileSchemaVersion":"0.2","protocolVersion":"0.3.0","cases":[]}`},
		{name: "unknown root member", document: strings.TrimSuffix(validManifest, `}`) + `,"extra":true}`},
		{name: "unknown case member", document: manifestDocument(strings.TrimSuffix(validCase, `}`) + `,"extra":true}`)},
		{name: "duplicate root member", document: `{"schemaVersion":"0.1","schemaVersion":"0.1","profileSchemaVersion":"0.2","protocolVersion":"0.3.0","cases":[` + validCase + `]}`},
		{name: "duplicate case member", document: manifestDocument(`{"id":"request","id":"other","file":"request.json","operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`)},
		{name: "escaped duplicate case member", document: manifestDocument(`{"id":"request","\u0069d":"other","file":"request.json","operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`)},
		{name: "trailing value", document: validManifest + `{}`},
		{name: "null required case field", document: manifestDocument(`{"id":null,"file":"request.json","operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`)},
		{name: "null optional case field", document: manifestDocument(`{"id":"request","file":"request.json","requestFile":null,"operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`)},
		{name: "duplicate case id", document: manifestDocument(validCase + `,` + strings.Replace(validCase, `"file":"request.json"`, `"file":"second.json"`, 1))},
	}
	corpus := testA2AConformanceCorpus()
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := DecodeA2AConformanceManifestV02([]byte(testCase.document), corpus); err == nil {
				t.Fatal("malformed manifest was accepted")
			}
		})
	}

	var canonicalRoot map[string]any
	if err := json.Unmarshal([]byte(validManifest), &canonicalRoot); err != nil {
		t.Fatalf("decode canonical root for required-field cases: %v", err)
	}
	for _, fieldName := range []string{"schemaVersion", "profileSchemaVersion", "protocolVersion", "cases"} {
		for _, mode := range []string{"omitted", "null"} {
			t.Run("root "+fieldName+" "+mode, func(t *testing.T) {
				candidate := cloneJSONMap(t, canonicalRoot)
				if mode == "omitted" {
					delete(candidate, fieldName)
				} else {
					candidate[fieldName] = nil
				}
				data, err := json.Marshal(candidate)
				if err != nil {
					t.Fatalf("encode root required-field candidate: %v", err)
				}
				if _, err := DecodeA2AConformanceManifestV02(data, corpus); err == nil {
					t.Fatal("manifest with absent required root field was accepted")
				}
			})
		}
	}

	var canonicalCase map[string]any
	if err := json.Unmarshal([]byte(validCase), &canonicalCase); err != nil {
		t.Fatalf("decode canonical case for required-field cases: %v", err)
	}
	for _, fieldName := range []string{"id", "file", "operation", "fixtureKind", "expectedValid", "mediaType", "rules"} {
		for _, mode := range []string{"omitted", "null"} {
			t.Run("case "+fieldName+" "+mode, func(t *testing.T) {
				candidate := cloneJSONMap(t, canonicalCase)
				if mode == "omitted" {
					delete(candidate, fieldName)
				} else {
					candidate[fieldName] = nil
				}
				caseData, err := json.Marshal(candidate)
				if err != nil {
					t.Fatalf("encode case required-field candidate: %v", err)
				}
				if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(string(caseData))), corpus); err == nil {
					t.Fatal("manifest with absent required case field was accepted")
				}
			})
		}
	}

	manifest, err := DecodeA2AConformanceManifestV02([]byte(validManifest), corpus)
	if err != nil {
		t.Fatalf("strict decoder rejected canonical manifest: %v", err)
	}
	if len(manifest.Cases) != 1 || !manifest.Cases[0].ExpectedValid {
		t.Fatalf("canonical manifest decoded as %+v", manifest)
	}
	invalidRequest := `{"id":"invalid-request","file":"request.json","operation":"message/send","fixtureKind":"request","expectedValid":false,"protocolError":"invalid-jsonrpc-version","mediaType":"application/json","rules":["jsonrpc-envelope"]}`
	manifest, err = DecodeA2AConformanceManifestV02([]byte(manifestDocument(invalidRequest)), corpus)
	if err != nil {
		t.Fatalf("decode explicit false expectedValid: %v", err)
	}
	if manifest.Cases[0].ExpectedValid {
		t.Fatal("explicit false expectedValid decoded as true")
	}

	t.Run("duplicate ids rejected before corpus access", func(t *testing.T) {
		countingCorpus := &countingConformanceFS{}
		duplicateIDs := manifestDocument(validCase + `,` + strings.Replace(validCase, `"file":"request.json"`, `"file":"second.json"`, 1))
		if _, err := DecodeA2AConformanceManifestV02([]byte(duplicateIDs), countingCorpus); err == nil {
			t.Fatal("duplicate case ids were accepted")
		}
		if countingCorpus.opens != 0 {
			t.Fatalf("duplicate ids accessed corpus %d time(s)", countingCorpus.opens)
		}
	})
}

func TestA2AProfileConformanceManifestPaths(t *testing.T) {
	paths := []string{
		"",
		"/absolute.json",
		"../outside.json",
		"nested/../outside.json",
		"nested/./fixture.json",
		"nested//fixture.json",
		`nested\fixture.json`,
		"C:/fixture.json",
		"//server/share/fixture.json",
		"https://example.test/fixture.json",
		"nested/name:fixture.json",
		"nested/name%20fixture.json",
		"nested/name?.json",
		"nested/name\x00.json",
		"nested/trailing. ",
		"nested/CON.json",
		"nested/lpt1.txt",
	}
	for _, fixturePath := range paths {
		t.Run(fmt.Sprintf("%q", fixturePath), func(t *testing.T) {
			corpus := &countingConformanceFS{}
			encodedPath, err := json.Marshal(fixturePath)
			if err != nil {
				t.Fatalf("encode fixture path: %v", err)
			}
			caseDocument := fmt.Sprintf(`{"id":"request","file":%s,"operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`, encodedPath)
			if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(caseDocument)), corpus); err == nil {
				t.Fatal("unsafe path was accepted")
			}
			if corpus.opens != 0 {
				t.Fatalf("unsafe path accessed corpus %d time(s)", corpus.opens)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(canonicalRequestCase("missing.json"))), fstest.MapFS{}); err == nil {
			t.Fatal("missing fixture was accepted")
		}
	})
	t.Run("directory", func(t *testing.T) {
		corpus := fstest.MapFS{"request.json": &fstest.MapFile{Mode: fs.ModeDir}}
		if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(canonicalRequestCase("request.json"))), corpus); err == nil {
			t.Fatal("directory fixture was accepted")
		}
	})
	t.Run("symbolic link", func(t *testing.T) {
		corpus := fstest.MapFS{
			"request.json": &fstest.MapFile{Data: []byte("target.json"), Mode: fs.ModeSymlink},
			"target.json":  &fstest.MapFile{Data: []byte(`{}`)},
		}
		if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(canonicalRequestCase("request.json"))), corpus); err == nil {
			t.Fatal("symbolic-link fixture was accepted as a regular file")
		}
	})
	t.Run("canonical nested regular file", func(t *testing.T) {
		corpus := fstest.MapFS{"nested/request.json": &fstest.MapFile{Data: []byte(`{}`)}}
		if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(canonicalRequestCase("nested/request.json"))), corpus); err != nil {
			t.Fatalf("canonical nested fixture rejected: %v", err)
		}
	})
}

func TestA2AProfileConformanceManifestMetadata(t *testing.T) {
	validRequest := canonicalRequestCase("request.json")
	validMessageResponse := `{"id":"response","file":"response.json","requestFile":"request.json","operation":"message/send","fixtureKind":"response","expectedValid":true,"wireResultKind":"message","goConcreteType":"*a2a.Message","mediaType":"application/json","rules":["jsonrpc-envelope","request-response-id","result-union","result-type","message-result"]}`
	invalidResponse := `{"id":"invalid","file":"response.json","requestFile":"request.json","operation":"message/send","fixtureKind":"response","expectedValid":false,"protocolError":"invalid-result-kind","mediaType":"application/json","rules":["result-union"]}`
	tests := []struct {
		name         string
		caseDocument string
	}{
		{name: "unsupported operation", caseDocument: strings.Replace(validRequest, `"message/send"`, `"tasks/list"`, 1)},
		{name: "unsupported fixture kind", caseDocument: strings.Replace(validRequest, `"fixtureKind":"request"`, `"fixtureKind":"batch"`, 1)},
		{name: "wrong media type", caseDocument: strings.Replace(validRequest, `"application/json"`, `"text/event-stream"`, 1)},
		{name: "requestFile forbidden for request", caseDocument: strings.Replace(validRequest, `"operation"`, `"requestFile":"request.json","operation"`, 1)},
		{name: "requestFile required for response", caseDocument: strings.Replace(validMessageResponse, `,"requestFile":"request.json"`, ``, 1)},
		{name: "valid response missing result pair", caseDocument: strings.Replace(strings.Replace(validMessageResponse, `,"wireResultKind":"message"`, ``, 1), `,"goConcreteType":"*a2a.Message"`, ``, 1)},
		{name: "message mapped to Task", caseDocument: strings.Replace(validMessageResponse, `*a2a.Message`, `*a2a.Task`, 1)},
		{name: "task mapped to Message", caseDocument: strings.Replace(strings.Replace(validMessageResponse, `"wireResultKind":"message"`, `"wireResultKind":"task"`, 1), `*a2a.Message`, `*a2a.Message`, 1)},
		{name: "invalid response declares result type", caseDocument: strings.Replace(invalidResponse, `"protocolError"`, `"wireResultKind":"message","goConcreteType":"*a2a.Message","protocolError"`, 1)},
		{name: "invalid response missing protocol error", caseDocument: strings.Replace(invalidResponse, `,"protocolError":"invalid-result-kind"`, ``, 1)},
		{name: "valid response declares protocol error", caseDocument: strings.Replace(validMessageResponse, `"mediaType"`, `"protocolError":"invalid-result-kind","mediaType"`, 1)},
		{name: "unknown protocol error", caseDocument: strings.Replace(invalidResponse, `invalid-result-kind`, `provider-paused`, 1)},
		{name: "protocol error lacks corresponding rule", caseDocument: strings.Replace(invalidResponse, `invalid-result-kind`, `invalid-message-result`, 1)},
		{name: "unknown rule", caseDocument: strings.Replace(validRequest, `"request-params"`, `"not-a-rule"`, 1)},
		{name: "duplicate rule", caseDocument: strings.Replace(validRequest, `"request-params"]`, `"request-params","request-params"]`, 1)},
		{name: "known but inapplicable rule", caseDocument: strings.Replace(validRequest, `"request-params"]`, `"request-params","five-context-headers"]`, 1)},
		{name: "valid case omits required executable rule", caseDocument: strings.Replace(validRequest, `,"request-params"`, ``, 1)},
		{name: "empty rules", caseDocument: strings.Replace(validRequest, `["jsonrpc-envelope","request-params"]`, `[]`, 1)},
		{name: "stream extension on JSON media", caseDocument: strings.Replace(validRequest, `request.json`, `request.sse`, 1)},
	}
	corpus := testA2AConformanceCorpus()
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := DecodeA2AConformanceManifestV02([]byte(manifestDocument(testCase.caseDocument)), corpus); err == nil {
				t.Fatal("invalid manifest metadata was accepted")
			}
		})
	}
}

func TestA2AProfileConformanceManifestClaimsMatchFixtures(t *testing.T) {
	manifest, err := LoadA2AConformanceManifestV02()
	if err != nil {
		t.Fatalf("load canonical manifest: %v", err)
	}
	byID := make(map[string]A2AConformanceCaseV02, len(manifest.Cases))
	for _, manifestCase := range manifest.Cases {
		byID[manifestCase.ID] = manifestCase
	}

	t.Run("requestFile operation", func(t *testing.T) {
		manifestCase := byID["message-send-message-result"]
		manifestCase.RequestFile = "tasks-get-request.json"
		if err := validateManifestCase(t, manifestCase); err == nil {
			t.Fatal("requestFile with a different operation was accepted")
		}
	})

	t.Run("wire result and Go concrete type", func(t *testing.T) {
		manifestCase := byID["message-send-message-result"]
		manifestCase.File = "message-send-task-response.json"
		if err := validateManifestCase(t, manifestCase); err == nil {
			t.Fatal("fixture whose concrete result contradicts manifest metadata was accepted")
		}
	})

	t.Run("protocol error must be established by execution", func(t *testing.T) {
		manifestCase := byID["message-send-invalid-kind"]
		manifestCase.File = "message-send-message-response.json"
		err := validateManifestCase(t, manifestCase)
		if manifestCaseExpectationSatisfied(manifestCase, err) {
			t.Fatal("invalid-case claim was satisfied without a corresponding rule failure")
		}
	})
}

func TestA2AProfileConformanceFixtures(t *testing.T) {
	manifest, err := LoadA2AConformanceManifestV02()
	if err != nil {
		t.Fatalf("load conformance manifest: %v", err)
	}
	for _, testCase := range manifest.Cases {
		t.Run(testCase.ID, func(t *testing.T) {
			err := validateManifestCase(t, testCase)
			if !manifestCaseExpectationSatisfied(testCase, err) {
				t.Fatalf("fixture validity = %t, want %t (rule error: %v)", err == nil, testCase.ExpectedValid, err)
			}
		})
	}
}

func manifestCaseExpectationSatisfied(testCase A2AConformanceCaseV02, ruleError error) bool {
	return testCase.ExpectedValid == (ruleError == nil)
}

func TestA2AProfileConformanceClientPaths(t *testing.T) {
	messageParams := mustMessageParamsFixture(t, "message-send-request.json")
	for _, testCase := range []struct {
		name     string
		fixture  string
		wantType any
	}{
		{"message result", "message-send-message-response.json", (*a2a.Message)(nil)},
		{"task result", "message-send-task-response.json", (*a2a.Task)(nil)},
	} {
		t.Run("message-send-"+testCase.name, func(t *testing.T) {
			server := newA2AFixtureServer(t, testCase.fixture, "message/send", nil)
			defer server.Close()
			client := newPublicA2AClient(t, server, nil)
			result, err := client.SendMessage(t.Context(), messageParams)
			if err != nil {
				t.Fatalf("SendMessage: %v", err)
			}
			if reflect.TypeOf(result) != reflect.TypeOf(testCase.wantType) {
				t.Fatalf("SendMessage result type = %T, want %T", result, testCase.wantType)
			}
		})
	}

	t.Run("message-stream-four-event-kinds", func(t *testing.T) {
		server := newA2AFixtureServer(t, "message-stream-valid.sse", "message/stream", nil)
		defer server.Close()
		client := newPublicA2AClient(t, server, nil)
		params := mustMessageParamsFixture(t, "message-stream-request.json")
		var events []a2a.Event
		for event, err := range client.SendStreamingMessage(t.Context(), params) {
			if err != nil {
				t.Fatalf("SendStreamingMessage: %v", err)
			}
			events = append(events, event)
		}
		assertFourStreamingEventKinds(t, events)
	})

	t.Run("tasks-get", func(t *testing.T) {
		server := newA2AFixtureServer(t, "tasks-get-response.json", "tasks/get", nil)
		defer server.Close()
		client := newPublicA2AClient(t, server, nil)
		task, err := client.GetTask(t.Context(), mustTaskQueryFixture(t, "tasks-get-request.json"))
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if _, err := ValidateA2ATask(task); err != nil {
			t.Fatalf("GetTask result violates Profile v0.2: %v", err)
		}
		if len(task.History) != 1 {
			t.Fatalf("GetTask history length = %d, want 1", len(task.History))
		}
	})

	t.Run("tasks-cancel", func(t *testing.T) {
		server := newA2AFixtureServer(t, "tasks-cancel-response.json", "tasks/cancel", nil)
		defer server.Close()
		client := newPublicA2AClient(t, server, nil)
		params := mustTaskIDFixture(t, "tasks-cancel-request.json")
		task, err := client.CancelTask(t.Context(), params)
		if err != nil {
			t.Fatalf("CancelTask: %v", err)
		}
		mapping, err := ValidateA2ATask(task)
		if err != nil {
			t.Fatalf("CancelTask result violates Profile v0.2: %v", err)
		}
		if task.ID != params.ID || mapping.State != a2a.TaskStateCanceled {
			t.Fatalf("CancelTask result = task %q state %q, want task %q canceled", task.ID, mapping.State, params.ID)
		}
	})

	for _, testCase := range []struct {
		name      string
		operation string
		fixture   string
		want      error
	}{
		{"get-not-found", "tasks/get", "tasks-get-not-found-response.json", a2a.ErrTaskNotFound},
		{"cancel-not-found", "tasks/cancel", "tasks-cancel-not-found-response.json", a2a.ErrTaskNotFound},
		{"cancel-not-cancelable", "tasks/cancel", "tasks-cancel-not-cancelable-response.json", a2a.ErrTaskNotCancelable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := newA2AFixtureServer(t, testCase.fixture, testCase.operation, nil)
			defer server.Close()
			client := newPublicA2AClient(t, server, nil)
			var err error
			if testCase.operation == "tasks/get" {
				_, err = client.GetTask(t.Context(), mustTaskQueryFixture(t, "tasks-get-request.json"))
			} else {
				_, err = client.CancelTask(t.Context(), mustTaskIDFixture(t, "tasks-cancel-request.json"))
			}
			if !errors.Is(err, testCase.want) {
				t.Fatalf("operation error = %v, want %v", err, testCase.want)
			}
		})
	}

	t.Run("five-context-headers", func(t *testing.T) {
		headers := mustContextHeadersFixture(t)
		server := newA2AFixtureServer(t, "tasks-get-response.json", "tasks/get", headers)
		defer server.Close()
		meta := make(a2aclient.CallMeta, len(headers))
		for name, value := range headers {
			meta[name] = []string{value}
		}
		client := newPublicA2AClient(t, server, []a2aclient.CallInterceptor{a2aclient.NewStaticCallMetaInjector(meta)})
		if _, err := client.GetTask(t.Context(), mustTaskQueryFixture(t, "tasks-get-request.json")); err != nil {
			t.Fatalf("GetTask with context headers: %v", err)
		}
	})
}

func TestA2AProfileConformanceServerPaths(t *testing.T) {
	handler := &profileFixtureHandler{
		sendResult: mustSendResultFixture(t, "message-send-message-response.json"),
		stream:     mustStreamEventsFixture(t, "message-stream-valid.sse", "message-stream-request.json"),
		getTask:    mustTaskResultFixture(t, "tasks-get-response.json"),
		cancelTask: mustTaskResultFixture(t, "tasks-cancel-response.json"),
	}
	server := httptest.NewServer(a2asrv.NewJSONRPCHandler(handler))
	defer server.Close()
	client := newPublicA2AClient(t, server, nil)

	t.Run("message-send", func(t *testing.T) {
		result, err := client.SendMessage(t.Context(), mustMessageParamsFixture(t, "message-send-request.json"))
		if err != nil {
			t.Fatalf("public SendMessage through a2asrv: %v", err)
		}
		message, ok := result.(*a2a.Message)
		if !ok {
			t.Fatalf("a2asrv message/send result type = %T, want *a2a.Message", result)
		}
		if err := ValidateA2AMessageResult(message); err != nil {
			t.Fatalf("a2asrv message/send result violates Profile v0.2: %v", err)
		}
		if handler.lastMessage == nil || handler.lastMessage.Message == nil || handler.lastMessage.Message.ID != "message-user-1" {
			t.Fatalf("a2asrv OnSendMessage params = %+v", handler.lastMessage)
		}
	})

	t.Run("message-stream", func(t *testing.T) {
		var events []a2a.Event
		for event, err := range client.SendStreamingMessage(t.Context(), mustMessageParamsFixture(t, "message-stream-request.json")) {
			if err != nil {
				t.Fatalf("public SendStreamingMessage through a2asrv: %v", err)
			}
			events = append(events, event)
		}
		assertFourStreamingEventKinds(t, events)
		if handler.lastStreamMessage == nil || handler.lastStreamMessage.Message == nil || handler.lastStreamMessage.Message.ID != "message-user-stream-1" {
			t.Fatalf("a2asrv OnSendMessageStream params = %+v", handler.lastStreamMessage)
		}
	})

	t.Run("tasks-get", func(t *testing.T) {
		task, err := client.GetTask(t.Context(), mustTaskQueryFixture(t, "tasks-get-request.json"))
		if err != nil {
			t.Fatalf("public GetTask through a2asrv: %v", err)
		}
		if _, err := ValidateA2ATask(task); err != nil {
			t.Fatalf("a2asrv tasks/get result violates Profile v0.2: %v", err)
		}
		if handler.lastQuery == nil || handler.lastQuery.ID != "task-1" || handler.lastQuery.HistoryLength == nil || *handler.lastQuery.HistoryLength != 1 {
			t.Fatalf("a2asrv OnGetTask params = %+v", handler.lastQuery)
		}
	})

	t.Run("tasks-cancel", func(t *testing.T) {
		task, err := client.CancelTask(t.Context(), mustTaskIDFixture(t, "tasks-cancel-request.json"))
		if err != nil {
			t.Fatalf("public CancelTask through a2asrv: %v", err)
		}
		mapping, err := ValidateA2ATask(task)
		if err != nil || mapping.State != a2a.TaskStateCanceled {
			t.Fatalf("a2asrv tasks/cancel result = %+v, mapping %+v, error %v", task, mapping, err)
		}
		if handler.lastTaskID == nil || handler.lastTaskID.ID != "task-1" {
			t.Fatalf("a2asrv OnCancelTask params = %+v", handler.lastTaskID)
		}
	})

	handler.getErr = a2a.ErrTaskNotFound
	t.Run("tasks-get-not-found", func(t *testing.T) {
		_, err := client.GetTask(t.Context(), mustTaskQueryFixture(t, "tasks-get-request.json"))
		if !errors.Is(err, a2a.ErrTaskNotFound) {
			t.Fatalf("public GetTask error = %v, want %v", err, a2a.ErrTaskNotFound)
		}
	})
	handler.getErr = nil

	for _, testCase := range []struct {
		name string
		err  error
	}{
		{"tasks-cancel-not-found", a2a.ErrTaskNotFound},
		{"tasks-cancel-not-cancelable", a2a.ErrTaskNotCancelable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler.cancelErr = testCase.err
			_, err := client.CancelTask(t.Context(), mustTaskIDFixture(t, "tasks-cancel-request.json"))
			if !errors.Is(err, testCase.err) {
				t.Fatalf("public CancelTask error = %v, want %v", err, testCase.err)
			}
		})
	}

	t.Run("supplemental-sse-framing-and-content-type", func(t *testing.T) {
		handler.cancelErr = nil
		body, mediaType := callA2AServerFixture(t, server.URL, "message-stream-request.json", "text/event-stream")
		if mediaType != "text/event-stream" {
			t.Fatalf("message/stream Content-Type = %q, want text/event-stream", mediaType)
		}
		if _, err := validateSSEStream(body, mustFixtureBytes(t, "message-stream-request.json")); err != nil {
			t.Fatalf("a2asrv message/stream framing: %v", err)
		}
	})
}

type wireEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func validateManifestCase(t *testing.T, testCase A2AConformanceCaseV02) error {
	t.Helper()
	fixture := mustFixtureBytes(t, testCase.File)
	var request []byte
	if testCase.RequestFile != "" {
		request = mustFixtureBytes(t, testCase.RequestFile)
		if err := validateRequestEnvelope(request, testCase.Operation); err != nil {
			return fmt.Errorf("requestFile %q does not describe %s: %w", testCase.RequestFile, testCase.Operation, err)
		}
	}

	executed := make(map[A2AConformanceRuleIDV02]struct{}, len(testCase.Rules))
	failed := make(map[A2AConformanceRuleIDV02]error)
	for _, ruleID := range testCase.Rules {
		if _, exists := executed[ruleID]; exists {
			return fmt.Errorf("rule %q executed more than once", ruleID)
		}
		executed[ruleID] = struct{}{}
		if err := assertA2AConformanceRule(testCase, fixture, request, ruleID); err != nil {
			failed[ruleID] = err
		}
	}
	if len(executed) != len(testCase.Rules) {
		return fmt.Errorf("executed %d rules, manifest declares %d", len(executed), len(testCase.Rules))
	}
	if testCase.ExpectedValid {
		for _, ruleID := range testCase.Rules {
			if err := failed[ruleID]; err != nil {
				return fmt.Errorf("rule %q: %w", ruleID, err)
			}
		}
		return nil
	}
	if len(failed) == 0 {
		return nil
	}
	for ruleID := range failed {
		if a2aProtocolErrorHasClaimedRule(testCase.ProtocolError, map[A2AConformanceRuleIDV02]struct{}{ruleID: {}}) {
			return failed[ruleID]
		}
	}
	t.Fatalf("fixture failed rules %v, but none establish protocolError %q", failed, testCase.ProtocolError)
	return nil
}

func assertA2AConformanceRule(testCase A2AConformanceCaseV02, fixture, request []byte, ruleID A2AConformanceRuleIDV02) error {
	switch ruleID {
	case A2ARuleJSONRPCEnvelope:
		return assertJSONRPCEnvelopeRule(testCase, fixture)
	case A2ARuleRequestParams:
		return validateRequestParams(fixture, testCase.Operation)
	case A2ARuleRequestResponseID:
		return assertRequestResponseIDRule(testCase, fixture, request)
	case A2ARuleResultXORError:
		return assertResultXORErrorRule(fixture)
	case A2ARuleResultUnion:
		return assertResultUnionRule(fixture)
	case A2ARuleResultType:
		return assertResultTypeRule(testCase, fixture)
	case A2ARuleMessageResult:
		return assertMessageResultRule(fixture)
	case A2ARuleTaskIdentity:
		return assertTaskIdentityRule(testCase, fixture)
	case A2ARuleTaskState:
		return assertTaskStateRule(testCase, fixture)
	case A2ARuleSSEFraming:
		_, err := parseSSEBlocks(fixture)
		return err
	case A2ARuleEventKinds:
		events, err := decodeSSEEventsForRule(fixture)
		if err != nil {
			return err
		}
		return requireFourStreamingEventKinds(events)
	case A2ARuleTaskContextStability:
		return assertTaskContextStabilityRule(fixture)
	case A2ARuleTerminalRequired:
		return assertTerminalRequiredRule(fixture)
	case A2ARuleTerminalLast:
		return assertTerminalLastRule(fixture)
	case A2ARuleArtifactOrder:
		return assertArtifactOrderRule(fixture)
	case A2ARuleArtifactLastChunk:
		return assertArtifactLastChunkRule(fixture)
	case A2ARuleHistoryLength:
		return assertHistoryLengthRule(testCase, fixture, request)
	case A2ARuleErrorOnly:
		envelope, err := decodeWireEnvelope(fixture)
		if err != nil {
			return err
		}
		if len(envelope.Result) > 0 || len(envelope.Error) == 0 {
			return errors.New("error fixture must contain error and no result")
		}
		return validateExpectedWireError(envelope.Error, testCase.ProtocolError)
	case A2ARuleRejectedMapping:
		task, err := decodeTaskResult(fixture)
		if err != nil {
			return err
		}
		mapping, err := MapA2ATaskState(task.Status.State)
		if err != nil {
			return err
		}
		if task.Status.State != a2a.TaskStateRejected || mapping.InvocationStatus != "failed" || mapping.ErrorCode != ErrorCodeAgentExecutionFailed {
			return fmt.Errorf("rejected mapping = %+v", mapping)
		}
		return nil
	case A2ARuleUnsupportedStateMapping:
		task, err := decodeTaskResult(fixture)
		if err != nil {
			return err
		}
		_, err = MapA2ATaskState(task.Status.State)
		var stateError *A2AProfileStateError
		if !errors.As(err, &stateError) || stateError.ErrorCode != ErrorCodeA2AProtocol {
			return fmt.Errorf("unsupported state mapping error = %v", err)
		}
		return nil
	case A2ARuleSameTask:
		task, err := decodeTaskResult(fixture)
		if err != nil {
			return err
		}
		params, err := decodeTaskIDRequest(request)
		if err != nil {
			return err
		}
		if task.ID != params.ID {
			return fmt.Errorf("task id = %q, request id = %q", task.ID, params.ID)
		}
		return nil
	case A2ARuleCanceledState:
		task, err := decodeTaskResult(fixture)
		if err != nil {
			return err
		}
		if task.Status.State != a2a.TaskStateCanceled {
			return fmt.Errorf("task state = %q, want canceled", task.Status.State)
		}
		return nil
	case A2ARuleFiveContextHeaders:
		return validateContextHeaderFixture(fixture)
	default:
		return fmt.Errorf("rule %q has no executable assertion", ruleID)
	}
}

func assertJSONRPCEnvelopeRule(testCase A2AConformanceCaseV02, fixture []byte) error {
	if testCase.FixtureKind == "stream" {
		blocks, err := parseSSEBlocks(fixture)
		if err != nil {
			return err
		}
		for index, block := range blocks {
			if err := assertJSONRPCVersionAndID(block); err != nil {
				return fmt.Errorf("stream event %d: %w", index, err)
			}
		}
		return nil
	}
	if err := assertJSONRPCVersionAndID(fixture); err != nil {
		return err
	}
	if testCase.FixtureKind == "request" {
		envelope, err := decodeWireEnvelope(fixture)
		if err != nil {
			return err
		}
		if envelope.Method != testCase.Operation {
			return fmt.Errorf("request method = %q, want %q", envelope.Method, testCase.Operation)
		}
	}
	return nil
}

func assertJSONRPCVersionAndID(data []byte) error {
	envelope, err := decodeWireEnvelope(data)
	if err != nil {
		return err
	}
	if envelope.JSONRPC != "2.0" {
		return fmt.Errorf("JSON-RPC version = %q", envelope.JSONRPC)
	}
	if len(envelope.ID) == 0 {
		return errors.New("JSON-RPC id is missing")
	}
	return nil
}

func assertRequestResponseIDRule(testCase A2AConformanceCaseV02, fixture, request []byte) error {
	requestEnvelope, err := decodeWireEnvelope(request)
	if err != nil {
		return err
	}
	responses := [][]byte{fixture}
	if testCase.FixtureKind == "stream" {
		responses, err = parseSSEBlocks(fixture)
		if err != nil {
			return err
		}
	}
	for index, response := range responses {
		responseEnvelope, err := decodeWireEnvelope(response)
		if err != nil {
			return err
		}
		equal, err := equalJSON(responseEnvelope.ID, requestEnvelope.ID)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("response %d id does not match request id", index)
		}
	}
	return nil
}

func assertResultXORErrorRule(fixture []byte) error {
	envelope, err := decodeWireEnvelope(fixture)
	if err != nil {
		return err
	}
	if (len(envelope.Result) > 0) == (len(envelope.Error) > 0) {
		return errors.New("response must contain exactly one of result or error")
	}
	return nil
}

func assertResultUnionRule(fixture []byte) error {
	envelope, err := decodeWireEnvelope(fixture)
	if err != nil {
		return err
	}
	event, err := a2a.UnmarshalEventJSON(envelope.Result)
	if err != nil {
		return err
	}
	switch event.(type) {
	case *a2a.Message, *a2a.Task:
		return nil
	default:
		return fmt.Errorf("message/send result type = %T", event)
	}
}

func assertResultTypeRule(testCase A2AConformanceCaseV02, fixture []byte) error {
	envelope, err := decodeWireEnvelope(fixture)
	if err != nil {
		return err
	}
	event, err := a2a.UnmarshalEventJSON(envelope.Result)
	if err != nil {
		return err
	}
	if got := fmt.Sprintf("%T", event); got != testCase.GoConcreteType {
		return fmt.Errorf("Go concrete type = %q, want %q", got, testCase.GoConcreteType)
	}
	kind := ""
	switch event.(type) {
	case *a2a.Message:
		kind = "message"
	case *a2a.Task:
		kind = "task"
	}
	if kind != testCase.WireResultKind {
		return fmt.Errorf("wire result kind = %q, want %q", kind, testCase.WireResultKind)
	}
	return nil
}

func assertMessageResultRule(fixture []byte) error {
	envelope, err := decodeWireEnvelope(fixture)
	if err != nil {
		return err
	}
	event, err := a2a.UnmarshalEventJSON(envelope.Result)
	if err != nil {
		return err
	}
	message, ok := event.(*a2a.Message)
	if !ok {
		return fmt.Errorf("result type = %T, want *a2a.Message", event)
	}
	return ValidateA2AMessageResult(message)
}

func assertTaskIdentityRule(testCase A2AConformanceCaseV02, fixture []byte) error {
	if testCase.FixtureKind == "stream" {
		events, err := decodeSSEEventsForRule(fixture)
		if err != nil {
			return err
		}
		for index, event := range events {
			info := event.TaskInfo()
			if info.TaskID == "" || info.ContextID == "" {
				return fmt.Errorf("stream event %d has empty task or context id", index)
			}
		}
		return nil
	}
	task, err := decodeTaskResult(fixture)
	if err != nil {
		return err
	}
	if task.ID == "" || task.ContextID == "" {
		return errors.New("task has empty task or context id")
	}
	return nil
}

func assertTaskStateRule(testCase A2AConformanceCaseV02, fixture []byte) error {
	if testCase.FixtureKind == "stream" {
		events, err := decodeSSEEventsForRule(fixture)
		if err != nil {
			return err
		}
		for index, event := range events {
			var state a2a.TaskState
			switch typed := event.(type) {
			case *a2a.Task:
				state = typed.Status.State
			case *a2a.TaskStatusUpdateEvent:
				state = typed.Status.State
			default:
				continue
			}
			if _, err := MapA2ATaskState(state); err != nil {
				return fmt.Errorf("stream event %d: %w", index, err)
			}
		}
		return nil
	}
	task, err := decodeTaskResult(fixture)
	if err != nil {
		return err
	}
	_, err = MapA2ATaskState(task.Status.State)
	return err
}

func assertTaskContextStabilityRule(fixture []byte) error {
	events, err := decodeSSEEventsForRule(fixture)
	if err != nil {
		return err
	}
	var taskID a2a.TaskID
	var contextID string
	for index, event := range events {
		info := event.TaskInfo()
		if index == 0 {
			taskID, contextID = info.TaskID, info.ContextID
			continue
		}
		if info.TaskID != taskID || info.ContextID != contextID {
			return fmt.Errorf("stream event %d changed task/context identity", index)
		}
	}
	return nil
}

func assertTerminalRequiredRule(fixture []byte) error {
	events, err := decodeSSEEventsForRule(fixture)
	if err != nil {
		return err
	}
	for _, event := range events {
		terminal, err := isA2ATerminalEvent(event)
		if err != nil {
			return err
		}
		if terminal {
			return nil
		}
	}
	return errors.New("stream reached EOF without terminal event")
}

func assertTerminalLastRule(fixture []byte) error {
	events, err := decodeSSEEventsForRule(fixture)
	if err != nil {
		return err
	}
	terminalIndex := -1
	for index, event := range events {
		terminal, err := isA2ATerminalEvent(event)
		if err != nil {
			return err
		}
		if terminal {
			if terminalIndex >= 0 {
				return fmt.Errorf("stream contains multiple terminal events")
			}
			terminalIndex = index
		}
	}
	if terminalIndex >= 0 && terminalIndex != len(events)-1 {
		return fmt.Errorf("event arrived after terminal event %d", terminalIndex)
	}
	return nil
}

func assertArtifactOrderRule(fixture []byte) error {
	events, err := decodeSSEEventsForRule(fixture)
	if err != nil {
		return err
	}
	type artifactState struct {
		finished bool
	}
	artifacts := make(map[a2a.ArtifactID]artifactState)
	for index, event := range events {
		artifactEvent, ok := event.(*a2a.TaskArtifactUpdateEvent)
		if !ok {
			continue
		}
		if artifactEvent.Artifact == nil || artifactEvent.Artifact.ID == "" {
			return fmt.Errorf("stream event %d has no artifact identity", index)
		}
		state, seen := artifacts[artifactEvent.Artifact.ID]
		if state.finished {
			return fmt.Errorf("stream event %d updated artifact after lastChunk", index)
		}
		if artifactEvent.Append && !seen {
			return fmt.Errorf("stream event %d appends before base artifact", index)
		}
		if !artifactEvent.Append && seen {
			return fmt.Errorf("stream event %d replaces an existing artifact", index)
		}
		artifacts[artifactEvent.Artifact.ID] = artifactState{finished: artifactEvent.LastChunk}
	}
	return nil
}

func assertArtifactLastChunkRule(fixture []byte) error {
	events, err := decodeSSEEventsForRule(fixture)
	if err != nil {
		return err
	}
	finished := make(map[a2a.ArtifactID]bool)
	seen := make(map[a2a.ArtifactID]bool)
	for index, event := range events {
		artifactEvent, ok := event.(*a2a.TaskArtifactUpdateEvent)
		if !ok {
			continue
		}
		if artifactEvent.Artifact == nil || artifactEvent.Artifact.ID == "" {
			return fmt.Errorf("stream event %d has no artifact identity", index)
		}
		artifactID := artifactEvent.Artifact.ID
		if finished[artifactID] {
			return fmt.Errorf("stream event %d arrived after artifact lastChunk", index)
		}
		seen[artifactID] = true
		finished[artifactID] = artifactEvent.LastChunk
	}
	for artifactID := range seen {
		if !finished[artifactID] {
			return fmt.Errorf("artifact %q did not receive lastChunk", artifactID)
		}
	}
	return nil
}

func assertHistoryLengthRule(testCase A2AConformanceCaseV02, fixture, request []byte) error {
	if testCase.FixtureKind == "request" {
		envelope, err := decodeWireEnvelope(fixture)
		if err != nil {
			return err
		}
		var params a2a.TaskQueryParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		if params.HistoryLength == nil || *params.HistoryLength != 1 {
			return errors.New("tasks/get request must declare historyLength=1")
		}
		return nil
	}
	task, err := decodeTaskResult(fixture)
	if err != nil {
		return err
	}
	requestEnvelope, err := decodeWireEnvelope(request)
	if err != nil {
		return err
	}
	var params a2a.TaskQueryParams
	if err := json.Unmarshal(requestEnvelope.Params, &params); err != nil {
		return err
	}
	if params.HistoryLength == nil || len(task.History) != *params.HistoryLength {
		return errors.New("tasks/get response does not preserve requested history length")
	}
	return nil
}

func isA2ATerminalEvent(event a2a.Event) (bool, error) {
	switch typed := event.(type) {
	case *a2a.Task:
		mapping, err := MapA2ATaskState(typed.Status.State)
		return mapping.Classification == A2ATaskStateTerminal, err
	case *a2a.TaskStatusUpdateEvent:
		mapping, err := MapA2ATaskState(typed.Status.State)
		if err != nil {
			return false, err
		}
		terminal := mapping.Classification == A2ATaskStateTerminal
		if typed.Final != terminal {
			return false, fmt.Errorf("status final flag contradicts state %q", typed.Status.State)
		}
		return terminal, nil
	default:
		return false, nil
	}
}

func decodeSSEEventsForRule(fixture []byte) ([]a2a.Event, error) {
	blocks, err := parseSSEBlocks(fixture)
	if err != nil {
		return nil, err
	}
	events := make([]a2a.Event, 0, len(blocks))
	for index, block := range blocks {
		envelope, err := decodeWireEnvelope(block)
		if err != nil {
			return nil, fmt.Errorf("stream event %d: %w", index, err)
		}
		if len(envelope.Result) == 0 || len(envelope.Error) > 0 {
			return nil, fmt.Errorf("stream event %d must contain only result", index)
		}
		event, err := a2a.UnmarshalEventJSON(envelope.Result)
		if err != nil {
			return nil, fmt.Errorf("stream event %d: %w", index, err)
		}
		events = append(events, event)
	}
	return events, nil
}

func decodeTaskResult(fixture []byte) (*a2a.Task, error) {
	envelope, err := decodeWireEnvelope(fixture)
	if err != nil {
		return nil, err
	}
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(envelope.Result, &kind); err != nil {
		return nil, err
	}
	if kind.Kind != "task" {
		return nil, fmt.Errorf("result kind = %q, want task", kind.Kind)
	}
	var task a2a.Task
	if err := json.Unmarshal(envelope.Result, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func decodeTaskIDRequest(request []byte) (*a2a.TaskIDParams, error) {
	envelope, err := decodeWireEnvelope(request)
	if err != nil {
		return nil, err
	}
	var params a2a.TaskIDParams
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func requireFourStreamingEventKinds(events []a2a.Event) error {
	kinds := map[string]bool{
		"message":         false,
		"task":            false,
		"status-update":   false,
		"artifact-update": false,
	}
	for _, event := range events {
		switch event.(type) {
		case *a2a.Message:
			kinds["message"] = true
		case *a2a.Task:
			kinds["task"] = true
		case *a2a.TaskStatusUpdateEvent:
			kinds["status-update"] = true
		case *a2a.TaskArtifactUpdateEvent:
			kinds["artifact-update"] = true
		default:
			return fmt.Errorf("unexpected stream event type %T", event)
		}
	}
	for kind, found := range kinds {
		if !found {
			return fmt.Errorf("stream contains no %s event", kind)
		}
	}
	return nil
}

func validateRequestEnvelope(data []byte, operation string) error {
	if err := assertJSONRPCVersionAndID(data); err != nil {
		return err
	}
	envelope, err := decodeWireEnvelope(data)
	if err != nil {
		return err
	}
	if envelope.Method != operation {
		return fmt.Errorf("request method = %q, want %q", envelope.Method, operation)
	}
	return validateRequestParams(data, operation)
}

func validateRequestParams(data []byte, operation string) error {
	envelope, err := decodeWireEnvelope(data)
	if err != nil {
		return err
	}
	if len(envelope.Params) == 0 {
		return errors.New("request params are missing")
	}

	switch operation {
	case "message/send", "message/stream":
		var params a2a.MessageSendParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return fmt.Errorf("decode MessageSendParams: %w", err)
		}
		if params.Message == nil || params.Message.ID == "" || params.Message.Role != a2a.MessageRoleUser || len(params.Message.Parts) == 0 {
			return errors.New("message/send params do not contain a concrete user message")
		}
	case "tasks/get":
		var params a2a.TaskQueryParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return fmt.Errorf("decode TaskQueryParams: %w", err)
		}
		if params.ID == "" || params.HistoryLength == nil || *params.HistoryLength != 1 {
			return errors.New("tasks/get params do not contain task id and historyLength=1")
		}
	case "tasks/cancel":
		var params a2a.TaskIDParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return fmt.Errorf("decode TaskIDParams: %w", err)
		}
		if params.ID == "" {
			return errors.New("tasks/cancel task id is empty")
		}
	default:
		return fmt.Errorf("operation %q is outside Profile v0.2", operation)
	}
	return nil
}

func validateResponseEnvelope(data, requestData []byte) (wireEnvelope, error) {
	envelope, err := decodeWireEnvelope(data)
	if err != nil {
		return wireEnvelope{}, err
	}
	if envelope.JSONRPC != "2.0" {
		return wireEnvelope{}, fmt.Errorf("JSON-RPC version = %q", envelope.JSONRPC)
	}
	if len(envelope.ID) == 0 {
		return wireEnvelope{}, errors.New("response id is missing")
	}
	if len(requestData) > 0 {
		request, err := decodeWireEnvelope(requestData)
		if err != nil {
			return wireEnvelope{}, fmt.Errorf("decode request for response: %w", err)
		}
		equal, err := equalJSON(envelope.ID, request.ID)
		if err != nil {
			return wireEnvelope{}, fmt.Errorf("compare request and response ids: %w", err)
		}
		if !equal {
			return wireEnvelope{}, errors.New("response id does not match request id")
		}
	}
	hasResult := len(envelope.Result) > 0
	hasError := len(envelope.Error) > 0
	if hasResult == hasError {
		return wireEnvelope{}, errors.New("response must contain exactly one of result or error")
	}
	return envelope, nil
}

func validateOperationResult(operation string, result, requestData []byte) error {
	var typedKind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(result, &typedKind); err != nil {
		return fmt.Errorf("decode result kind: %w", err)
	}

	switch operation {
	case "message/send":
		if typedKind.Kind != "message" && typedKind.Kind != "task" {
			return fmt.Errorf("message/send result kind = %q", typedKind.Kind)
		}
		event, err := a2a.UnmarshalEventJSON(result)
		if err != nil {
			return err
		}
		switch typed := event.(type) {
		case *a2a.Message:
			err = ValidateA2AMessageResult(typed)
		case *a2a.Task:
			task := typed
			_, err = ValidateA2ATask(task)
		}
		return err
	case "tasks/get", "tasks/cancel":
		if typedKind.Kind != "task" {
			return fmt.Errorf("%s result kind = %q", operation, typedKind.Kind)
		}
		var task a2a.Task
		if err := json.Unmarshal(result, &task); err != nil {
			return fmt.Errorf("decode task result: %w", err)
		}
		mapping, err := ValidateA2ATask(&task)
		if err != nil {
			return err
		}
		request, err := decodeWireEnvelope(requestData)
		if err != nil {
			return err
		}
		if operation == "tasks/get" {
			var params a2a.TaskQueryParams
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return err
			}
			if task.ID != params.ID || params.HistoryLength == nil || len(task.History) != *params.HistoryLength {
				return errors.New("tasks/get result does not preserve task id and requested history length")
			}
			return nil
		}
		var params a2a.TaskIDParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return err
		}
		if task.ID != params.ID || mapping.State != a2a.TaskStateCanceled {
			return errors.New("tasks/cancel result is not the same task in canceled state")
		}
		return nil
	default:
		return fmt.Errorf("operation %q has no result profile", operation)
	}
}

func validateExpectedWireError(data json.RawMessage, expected string) error {
	var rpcError wireError
	if err := json.Unmarshal(data, &rpcError); err != nil {
		return fmt.Errorf("decode JSON-RPC error: %w", err)
	}
	wantCode := map[string]int{
		"task-not-found":      -32001,
		"task-not-cancelable": -32002,
	}[expected]
	if wantCode == 0 {
		return fmt.Errorf("unknown expected protocol error %q", expected)
	}
	if rpcError.Code != wantCode || rpcError.Message == "" {
		return fmt.Errorf("JSON-RPC error = %d %q, want code %d", rpcError.Code, rpcError.Message, wantCode)
	}
	return nil
}

func validateSSEStream(data, requestData []byte) ([]a2a.Event, error) {
	request, err := decodeWireEnvelope(requestData)
	if err != nil {
		return nil, fmt.Errorf("decode stream request: %w", err)
	}
	blocks, err := parseSSEBlocks(data)
	if err != nil {
		return nil, err
	}

	var events []a2a.Event
	var taskID a2a.TaskID
	var contextID string
	terminal := false
	type artifactState struct {
		finished bool
	}
	artifacts := make(map[a2a.ArtifactID]artifactState)

	for index, block := range blocks {
		if terminal {
			return nil, fmt.Errorf("event %d arrived after terminal", index)
		}
		envelope, err := validateResponseEnvelope(block, requestData)
		if err != nil {
			return nil, fmt.Errorf("stream event %d envelope: %w", index, err)
		}
		equal, err := equalJSON(envelope.ID, request.ID)
		if err != nil || !equal {
			return nil, fmt.Errorf("stream event %d response id mismatch", index)
		}
		if len(envelope.Error) > 0 {
			return nil, fmt.Errorf("stream event %d is an error", index)
		}
		event, err := a2a.UnmarshalEventJSON(envelope.Result)
		if err != nil {
			return nil, fmt.Errorf("stream event %d: %w", index, err)
		}
		info := event.TaskInfo()
		if info.TaskID == "" || info.ContextID == "" {
			return nil, fmt.Errorf("stream event %d has empty task or context id", index)
		}
		if index == 0 {
			taskID = info.TaskID
			contextID = info.ContextID
		} else if info.TaskID != taskID || info.ContextID != contextID {
			return nil, fmt.Errorf("stream event %d changed task/context identity", index)
		}

		switch typed := event.(type) {
		case *a2a.Task:
			mapping, err := ValidateA2ATask(typed)
			if err != nil {
				return nil, fmt.Errorf("stream event %d task: %w", index, err)
			}
			terminal = mapping.Classification == A2ATaskStateTerminal
		case *a2a.Message:
			if typed.ID == "" || typed.Role != a2a.MessageRoleAgent || len(typed.Parts) == 0 {
				return nil, fmt.Errorf("stream event %d is not a concrete Agent message", index)
			}
		case *a2a.TaskStatusUpdateEvent:
			mapping, err := MapA2ATaskState(typed.Status.State)
			if err != nil {
				return nil, fmt.Errorf("stream event %d status: %w", index, err)
			}
			isTerminalState := mapping.Classification == A2ATaskStateTerminal
			if typed.Final != isTerminalState {
				return nil, fmt.Errorf("stream event %d final flag contradicts state %q", index, typed.Status.State)
			}
			terminal = typed.Final
		case *a2a.TaskArtifactUpdateEvent:
			if typed.Artifact == nil || typed.Artifact.ID == "" || len(typed.Artifact.Parts) == 0 {
				return nil, fmt.Errorf("stream event %d has an incomplete artifact", index)
			}
			state, seen := artifacts[typed.Artifact.ID]
			if state.finished {
				return nil, fmt.Errorf("stream event %d updated artifact %q after lastChunk", index, typed.Artifact.ID)
			}
			if typed.Append && !seen {
				return nil, fmt.Errorf("stream event %d appends artifact %q before its base", index, typed.Artifact.ID)
			}
			if !typed.Append && seen {
				return nil, fmt.Errorf("stream event %d replaces existing artifact %q", index, typed.Artifact.ID)
			}
			artifacts[typed.Artifact.ID] = artifactState{finished: typed.LastChunk}
		default:
			return nil, fmt.Errorf("stream event %d type %T is outside Profile v0.2", index, event)
		}
		events = append(events, event)
	}

	if !terminal {
		return nil, errors.New("stream reached EOF without terminal event")
	}
	for artifactID, state := range artifacts {
		if !state.finished {
			return nil, fmt.Errorf("artifact %q did not receive lastChunk", artifactID)
		}
	}
	return events, nil
}

func parseSSEBlocks(data []byte) ([][]byte, error) {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, ": end-of-stream\n")
	if !strings.HasSuffix(normalized, "\n\n") {
		return nil, errors.New("SSE stream does not end with a blank line")
	}
	normalized = strings.TrimSuffix(normalized, "\n\n")
	if normalized == "" {
		return nil, errors.New("SSE stream is empty")
	}
	rawBlocks := strings.Split(normalized, "\n\n")
	blocks := make([][]byte, 0, len(rawBlocks))
	eventIDs := make(map[string]struct{}, len(rawBlocks))
	for index, rawBlock := range rawBlocks {
		lines := strings.Split(rawBlock, "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "id: ") || !strings.HasPrefix(lines[1], "data: ") {
			return nil, fmt.Errorf("SSE block %d is not one id line and one data line", index)
		}
		eventID := strings.TrimPrefix(lines[0], "id: ")
		if eventID == "" {
			return nil, fmt.Errorf("SSE block %d has empty id", index)
		}
		if _, exists := eventIDs[eventID]; exists {
			return nil, fmt.Errorf("SSE block %d repeats id %q", index, eventID)
		}
		eventIDs[eventID] = struct{}{}
		blocks = append(blocks, []byte(strings.TrimPrefix(lines[1], "data: ")))
	}
	return blocks, nil
}

func validateContextHeaderFixture(data []byte) error {
	var headers map[string]string
	if err := json.Unmarshal(data, &headers); err != nil {
		return fmt.Errorf("decode context headers: %w", err)
	}
	profile, err := LoadA2AProfileV02()
	if err != nil {
		return err
	}
	wantNames := []string{
		profile.ContextHeaders.TraceID,
		profile.ContextHeaders.InvocationID,
		profile.ContextHeaders.RootTaskID,
		profile.ContextHeaders.ParentInvocationID,
		profile.ContextHeaders.WorkspaceID,
	}
	if len(headers) != len(wantNames) {
		return fmt.Errorf("context header count = %d, want %d", len(headers), len(wantNames))
	}
	for _, name := range wantNames {
		if headers[name] == "" {
			return fmt.Errorf("context header %q is missing or empty", name)
		}
	}
	return nil
}

func decodeWireEnvelope(data []byte) (wireEnvelope, error) {
	var envelope wireEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return wireEnvelope{}, fmt.Errorf("decode JSON-RPC envelope: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return wireEnvelope{}, fmt.Errorf("decode JSON-RPC envelope: %w", err)
	}
	return envelope, nil
}

func equalJSON(left, right []byte) (bool, error) {
	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false, err
	}
	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false, err
	}
	return reflect.DeepEqual(leftValue, rightValue), nil
}

func readEmbeddedJSONDocument(path string) (any, error) {
	data, err := a2aProfileV02Files.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(data))
}

func mustFixtureBytes(t *testing.T, file string) []byte {
	t.Helper()
	data, err := ReadA2AConformanceFixtureV02(file)
	if err != nil {
		t.Fatalf("read fixture %s: %v", file, err)
	}
	return data
}

func mustMessageParamsFixture(t *testing.T, file string) *a2a.MessageSendParams {
	t.Helper()
	envelope, err := decodeWireEnvelope(mustFixtureBytes(t, file))
	if err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	var params a2a.MessageSendParams
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		t.Fatalf("decode MessageSendParams from %s: %v", file, err)
	}
	return &params
}

func mustTaskQueryFixture(t *testing.T, file string) *a2a.TaskQueryParams {
	t.Helper()
	envelope, err := decodeWireEnvelope(mustFixtureBytes(t, file))
	if err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	var params a2a.TaskQueryParams
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		t.Fatalf("decode TaskQueryParams from %s: %v", file, err)
	}
	return &params
}

func mustTaskIDFixture(t *testing.T, file string) *a2a.TaskIDParams {
	t.Helper()
	envelope, err := decodeWireEnvelope(mustFixtureBytes(t, file))
	if err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	var params a2a.TaskIDParams
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		t.Fatalf("decode TaskIDParams from %s: %v", file, err)
	}
	return &params
}

func mustTaskResultFixture(t *testing.T, file string) *a2a.Task {
	t.Helper()
	envelope, err := decodeWireEnvelope(mustFixtureBytes(t, file))
	if err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	var task a2a.Task
	if err := json.Unmarshal(envelope.Result, &task); err != nil {
		t.Fatalf("decode task result from %s: %v", file, err)
	}
	return &task
}

func mustSendResultFixture(t *testing.T, file string) a2a.SendMessageResult {
	t.Helper()
	envelope, err := decodeWireEnvelope(mustFixtureBytes(t, file))
	if err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	event, err := a2a.UnmarshalEventJSON(envelope.Result)
	if err != nil {
		t.Fatalf("decode send result from %s: %v", file, err)
	}
	result, ok := event.(a2a.SendMessageResult)
	if !ok {
		t.Fatalf("fixture %s type %T is not SendMessageResult", file, event)
	}
	return result
}

func mustStreamEventsFixture(t *testing.T, streamFile, requestFile string) []a2a.Event {
	t.Helper()
	events, err := validateSSEStream(mustFixtureBytes(t, streamFile), mustFixtureBytes(t, requestFile))
	if err != nil {
		t.Fatalf("decode stream fixture %s: %v", streamFile, err)
	}
	return events
}

func mustContextHeadersFixture(t *testing.T) map[string]string {
	t.Helper()
	data := mustFixtureBytes(t, "context-headers.json")
	if err := validateContextHeaderFixture(data); err != nil {
		t.Fatalf("validate context headers fixture: %v", err)
	}
	var headers map[string]string
	if err := json.Unmarshal(data, &headers); err != nil {
		t.Fatalf("decode context headers fixture: %v", err)
	}
	return headers
}

func newA2AFixtureServer(t *testing.T, fixture, operation string, expectedHeaders map[string]string) *httptest.Server {
	t.Helper()
	body := mustFixtureBytes(t, fixture)
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestBody, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read %s request: %v", operation, err)
			return
		}
		if err := validateRequestEnvelope(requestBody, operation); err != nil {
			t.Errorf("%s client request violates Profile v0.2: %v", operation, err)
			return
		}
		for name, value := range expectedHeaders {
			if request.Header.Get(name) != value {
				t.Errorf("%s header %s = %q, want %q", operation, name, request.Header.Get(name), value)
			}
		}
		if strings.HasSuffix(fixture, ".sse") {
			if request.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("%s Accept = %q, want text/event-stream", operation, request.Header.Get("Accept"))
			}
			response.Header().Set("Content-Type", "text/event-stream")
		} else {
			response.Header().Set("Content-Type", "application/json")
		}
		_, _ = response.Write(body)
	}))
}

func newPublicA2AClient(t *testing.T, server *httptest.Server, interceptors []a2aclient.CallInterceptor) *a2aclient.Client {
	t.Helper()
	options := []a2aclient.FactoryOption{a2aclient.WithJSONRPCTransport(server.Client())}
	if len(interceptors) > 0 {
		options = append(options, a2aclient.WithInterceptors(interceptors...))
	}
	client, err := a2aclient.NewFromEndpoints(t.Context(), []a2a.AgentInterface{
		{URL: server.URL, Transport: a2a.TransportProtocolJSONRPC},
	}, options...)
	if err != nil {
		t.Fatalf("create public A2A client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Destroy(); err != nil {
			t.Errorf("destroy public A2A client: %v", err)
		}
	})
	return client
}

func assertFourStreamingEventKinds(t *testing.T, events []a2a.Event) {
	t.Helper()
	if err := requireFourStreamingEventKinds(events); err != nil {
		t.Fatal(err)
	}
}

func manifestDocument(caseDocuments string) string {
	return `{"schemaVersion":"0.1","profileSchemaVersion":"0.2","protocolVersion":"0.3.0","cases":[` + caseDocuments + `]}`
}

func canonicalRequestCase(fixturePath string) string {
	return fmt.Sprintf(`{"id":"request","file":%q,"operation":"message/send","fixtureKind":"request","expectedValid":true,"mediaType":"application/json","rules":["jsonrpc-envelope","request-params"]}`, fixturePath)
}

func cloneJSONMap(t *testing.T, source map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatalf("encode JSON map clone: %v", err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatalf("decode JSON map clone: %v", err)
	}
	return clone
}

func testA2AConformanceCorpus() fstest.MapFS {
	return fstest.MapFS{
		"request.json":  &fstest.MapFile{Data: []byte(`{}`)},
		"second.json":   &fstest.MapFile{Data: []byte(`{}`)},
		"response.json": &fstest.MapFile{Data: []byte(`{}`)},
		"stream.sse":    &fstest.MapFile{Data: []byte("data: {}\n\n")},
		"request.sse":   &fstest.MapFile{Data: []byte("data: {}\n\n")},
		"headers.json":  &fstest.MapFile{Data: []byte(`{}`)},
	}
}

type countingConformanceFS struct {
	opens int
}

func (f *countingConformanceFS) Open(string) (fs.File, error) {
	f.opens++
	return nil, fs.ErrNotExist
}

type profileFixtureHandler struct {
	a2asrv.RequestHandler

	sendResult a2a.SendMessageResult
	stream     []a2a.Event
	getTask    *a2a.Task
	cancelTask *a2a.Task
	getErr     error
	cancelErr  error

	lastMessage       *a2a.MessageSendParams
	lastStreamMessage *a2a.MessageSendParams
	lastQuery         *a2a.TaskQueryParams
	lastTaskID        *a2a.TaskIDParams
}

var _ a2asrv.RequestHandler = (*profileFixtureHandler)(nil)

func (h *profileFixtureHandler) OnSendMessage(_ context.Context, params *a2a.MessageSendParams) (a2a.SendMessageResult, error) {
	h.lastMessage = params
	return h.sendResult, nil
}

func (h *profileFixtureHandler) OnSendMessageStream(_ context.Context, params *a2a.MessageSendParams) iter.Seq2[a2a.Event, error] {
	h.lastStreamMessage = params
	return func(yield func(a2a.Event, error) bool) {
		for _, event := range h.stream {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func (h *profileFixtureHandler) OnGetTask(_ context.Context, params *a2a.TaskQueryParams) (*a2a.Task, error) {
	h.lastQuery = params
	if h.getErr != nil {
		return nil, h.getErr
	}
	return h.getTask, nil
}

func (h *profileFixtureHandler) OnCancelTask(_ context.Context, params *a2a.TaskIDParams) (*a2a.Task, error) {
	h.lastTaskID = params
	if h.cancelErr != nil {
		return nil, h.cancelErr
	}
	return h.cancelTask, nil
}

func callA2AServerFixture(t *testing.T, serverURL, fixture, accept string) ([]byte, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL, bytes.NewReader(mustFixtureBytes(t, fixture)))
	if err != nil {
		t.Fatalf("create A2A server request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call A2A server: %v", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close A2A server response: %v", err)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read A2A server response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("A2A server status = %s, body = %s", response.Status, body)
	}
	mediaType := response.Header.Get("Content-Type")
	if separator := strings.IndexByte(mediaType, ';'); separator >= 0 {
		mediaType = mediaType[:separator]
	}
	return body, mediaType
}

func assertJSONRPCErrorCode(t *testing.T, data []byte, want int) {
	t.Helper()
	envelope, err := decodeWireEnvelope(data)
	if err != nil {
		t.Fatalf("decode JSON-RPC error response: %v", err)
	}
	if len(envelope.Result) > 0 || len(envelope.Error) == 0 {
		t.Fatalf("JSON-RPC error response has result=%s error=%s", envelope.Result, envelope.Error)
	}
	var rpcError wireError
	if err := json.Unmarshal(envelope.Error, &rpcError); err != nil {
		t.Fatalf("decode JSON-RPC error: %v", err)
	}
	if rpcError.Code != want {
		t.Fatalf("JSON-RPC error code = %d, want %d", rpcError.Code, want)
	}
}
