package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/getkin/kin-openapi/openapi3"
)

func TestAgentCardContract(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()

	if err := validator.ValidateAgentCard(card); err != nil {
		t.Fatalf("valid card rejected: %v", err)
	}

	encoded, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	decoded, err := validator.DecodeAgentCard(encoded)
	if err != nil {
		t.Fatalf("decode valid card: %v", err)
	}
	if decoded.AgentID != card.AgentID {
		t.Fatalf("decoded agent id = %q, want %q", decoded.AgentID, card.AgentID)
	}
	if _, err := validator.DecodeAgentCard(append(encoded, []byte(` {}`)...)); err == nil {
		t.Fatal("Agent Card with trailing JSON value was accepted")
	}
}

func TestAgentCardRejectsSecretsAndRuntimeState(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()
	encoded, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}

	for _, field := range []string{"apiKey", "token", "healthStatus", "latencyMs", "successRate"} {
		t.Run(field, func(t *testing.T) {
			candidate := make(map[string]any, len(document)+1)
			for key, value := range document {
				candidate[key] = value
			}
			candidate[field] = "forbidden"
			data, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("marshal invalid card: %v", err)
			}
			if _, err := validator.DecodeAgentCard(data); err == nil {
				t.Fatalf("card with %s was accepted", field)
			}
		})
	}
}

func TestAgentCardSemanticValidation(t *testing.T) {
	validator := mustValidator(t)

	duplicateSkill := validAgentCard()
	duplicateSkill.Skills = append(duplicateSkill.Skills, AgentSkill{
		ID:                  duplicateSkill.Skills[0].ID,
		Name:                "Duplicate",
		Description:         "Duplicate semantic identifier.",
		InputSchema:         JSONSchema{"type": "object"},
		OutputSchema:        JSONSchema{"type": "object"},
		RequiredPermissions: []string{"document.read"},
	})
	var semanticError *SemanticValidationError
	if err := validator.ValidateAgentCard(duplicateSkill); !errors.As(err, &semanticError) {
		t.Fatalf("duplicate skill error = %v, want SemanticValidationError", err)
	}

	undeclaredPermission := validAgentCard()
	undeclaredPermission.Skills[0].RequiredPermissions = []string{"database.write"}
	if err := validator.ValidateAgentCard(undeclaredPermission); !errors.As(err, &semanticError) {
		t.Fatalf("undeclared permission error = %v, want SemanticValidationError", err)
	}
}

func TestAgentCardRejectsInvalidSemver(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()
	card.Version = "1.0.0-01"
	if err := validator.ValidateAgentCard(card); err == nil {
		t.Fatal("invalid semantic version was accepted")
	}
}

func TestInstallationContract(t *testing.T) {
	validator := mustValidator(t)
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	installation := Installation{
		InstallationID:      "installation-1",
		WorkspaceID:         "workspace-1",
		AgentID:             "contract-review",
		VersionConstraint:   "^1.0.0",
		InstalledVersion:    "1.2.0",
		AcceptedPermissions: []string{"document.read"},
		Status:              "enabled",
		InstalledAt:         now,
		UpdatedAt:           now,
	}
	if err := validator.ValidateInstallation(installation); err != nil {
		t.Fatalf("valid installation rejected: %v", err)
	}
	installation.VersionConstraint = "not a range"
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("invalid version range was accepted")
	}
}

func TestInvocationEventLifecycleContract(t *testing.T) {
	validator := mustValidator(t)
	event := validStartedEvent()
	if err := validator.ValidateInvocationEvent(event); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	event.Status = "failed"
	if err := validator.ValidateInvocationEvent(event); err == nil {
		t.Fatal("inconsistent event status was accepted")
	}

	event = validStartedEvent()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	document["payload"] = map[string]any{"apiKey": "secret"}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal secret event: %v", err)
	}
	if err := validateJSONBytes(validator.invocationEvent, data); err == nil {
		t.Fatal("event with arbitrary payload was accepted")
	}
}

func TestPlatformErrorRejectsArbitraryDetails(t *testing.T) {
	validator := mustValidator(t)
	if err := validator.ValidatePlatformError(PlatformError{Code: "INTERNAL_ERROR", Message: "failed"}); err != nil {
		t.Fatalf("valid platform error rejected: %v", err)
	}
	data := []byte(`{"code":"INTERNAL_ERROR","message":"failed","details":{"token":"secret"}}`)
	if err := validateJSONBytes(validator.platformError, data); err == nil {
		t.Fatal("platform error with arbitrary details was accepted")
	}
}

func TestA2AProfileUsesOfficialSDK(t *testing.T) {
	validator := mustValidator(t)
	profile, err := LoadA2AProfile()
	if err != nil {
		t.Fatalf("load A2A profile: %v", err)
	}
	if err := validator.ValidateA2AProfile(profile); err != nil {
		t.Fatalf("validate A2A profile: %v", err)
	}

	wantMethods := []string{"message/send", "message/stream", "tasks/get", "tasks/cancel"}
	if len(profile.RequiredMethods) != len(wantMethods) {
		t.Fatalf("required method count = %d, want %d", len(profile.RequiredMethods), len(wantMethods))
	}
	for index := range wantMethods {
		if profile.RequiredMethods[index] != wantMethods[index] {
			t.Fatalf("required method %d = %q, want %q", index, profile.RequiredMethods[index], wantMethods[index])
		}
	}

	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "Review this contract."})
	params := a2a.MessageSendParams{Message: message}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal official A2A params: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("official A2A params encoded to an empty payload")
	}
}

func TestOpenAPIDocuments(t *testing.T) {
	for _, path := range []string{
		filepath.Join("openapi", "control-plane.v1.yaml"),
		filepath.Join("openapi", "router-internal.v1.yaml"),
	} {
		t.Run(path, func(t *testing.T) {
			loader := openapi3.NewLoader()
			loader.IsExternalRefsAllowed = true
			loader.ReadFromURIFunc = func(loader *openapi3.Loader, location *url.URL) ([]byte, error) {
				if location.Scheme == "https" && location.Host == "schemas.nekiro.dev" {
					schemaFiles := map[string]string{
						"/common/v1":             "schemas/common.v1.schema.json",
						"/agent-card/v0.1":       "schemas/agent-card.v0.1.schema.json",
						"/platform-error/v1":     "schemas/platform-error.v1.schema.json",
						"/installation/v1":       "schemas/installation.v1.schema.json",
						"/invocation-event/v0.1": "schemas/invocation-event.v0.1.schema.json",
					}
					localPath, exists := schemaFiles[location.Path]
					if !exists {
						return nil, fmt.Errorf("unsupported schema URI: %s", location.Redacted())
					}
					return contractFiles.ReadFile(localPath)
				}
				if location.Scheme == "" || location.Scheme == "file" {
					return openapi3.DefaultReadFromURI(loader, location)
				}
				return nil, fmt.Errorf("external URI is not allowed: %s", location.Redacted())
			}
			document, err := loader.LoadFromFile(path)
			if err != nil {
				t.Fatalf("load OpenAPI document: %v", err)
			}
			if err := document.Validate(context.Background()); err != nil {
				t.Fatalf("validate OpenAPI document: %v", err)
			}
		})
	}
}

func mustValidator(t *testing.T) *Validator {
	t.Helper()
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("create validator: %v", err)
	}
	return validator
}

func validAgentCard() AgentCard {
	return AgentCard{
		SchemaVersion: AgentCardSchemaVersion,
		AgentID:       "contract-review",
		Name:          "Contract Review Agent",
		Description:   "Reviews contracts against a declared checklist.",
		Owner:         AgentOwner{ID: "nene7ko", DisplayName: "Nene7ko"},
		Version:       "1.0.0",
		Protocol: AgentProtocol{
			Type: "a2a", Version: A2AProtocolVersion, Transport: "JSONRPC", Endpoint: "http://localhost:4101",
		},
		Skills: []AgentSkill{{
			ID:                  "contract.review",
			Name:                "Review contract",
			Description:         "Reviews a contract.",
			InputSchema:         JSONSchema{"type": "object"},
			OutputSchema:        JSONSchema{"type": "object"},
			RequiredPermissions: []string{"document.read"},
		}},
		Authentication: AgentAuthentication{Type: "none"},
		Permissions: []PermissionDeclaration{{
			ID: "document.read", Description: "Read the supplied document.",
		}},
		Limits: AgentLimits{TimeoutMS: 30_000, MaxInputBytes: 1_000_000, MaxOutputBytes: 1_000_000, Streaming: true},
	}
}

func validStartedEvent() InvocationEvent {
	return InvocationEvent{
		SchemaVersion:    InvocationEventSchemaVersion,
		EventID:          "event-1",
		Sequence:         1,
		OccurredAt:       time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		Type:             "started",
		Status:           "running",
		InvocationID:     "invocation-1",
		RootTaskID:       "task-1",
		TraceID:          "trace-1",
		Caller:           Caller{Type: "user", ID: "user-1"},
		WorkspaceID:      "workspace-1",
		TargetAgentID:    "contract-review",
		AgentCardVersion: "1.0.0",
		Capability:       "contract.review",
	}
}

func validateJSONBytes(schema interface{ Validate(any) error }, data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	return schema.Validate(document)
}
