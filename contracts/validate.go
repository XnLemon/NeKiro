package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	commonSchemaID          = "https://schemas.nekiro.dev/common/v1"
	agentCardSchemaID       = "https://schemas.nekiro.dev/agent-card/v0.1"
	platformErrorSchemaID   = "https://schemas.nekiro.dev/platform-error/v1"
	installationSchemaID    = "https://schemas.nekiro.dev/installation/v1"
	invocationEventSchemaID = "https://schemas.nekiro.dev/invocation-event/v0.1"
	a2aProfileSchemaID      = "https://schemas.nekiro.dev/a2a-profile/v0.3.0"
)

//go:embed schemas/*.json a2a-profile/*.json openapi/*.yaml
var contractFiles embed.FS

type Validator struct {
	agentCard       *jsonschema.Schema
	platformError   *jsonschema.Schema
	installation    *jsonschema.Schema
	invocationEvent *jsonschema.Schema
	a2aProfile      *jsonschema.Schema
}

type SemanticIssue struct {
	Path    string
	Message string
}

type SemanticValidationError struct {
	Issues []SemanticIssue
}

func (e *SemanticValidationError) Error() string {
	return fmt.Sprintf("agent card semantic validation failed with %d issue(s)", len(e.Issues))
}

type SchemaValidationError struct {
	InstancePath string
	Keyword      string
}

func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf("contract schema validation failed at %s (%s)", e.InstancePath, e.Keyword)
}

func NewValidator() (*Validator, error) {
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

	resources := map[string]string{
		commonSchemaID:          "schemas/common.v1.schema.json",
		agentCardSchemaID:       "schemas/agent-card.v0.1.schema.json",
		platformErrorSchemaID:   "schemas/platform-error.v1.schema.json",
		installationSchemaID:    "schemas/installation.v1.schema.json",
		invocationEventSchemaID: "schemas/invocation-event.v0.1.schema.json",
		a2aProfileSchemaID:      "schemas/a2a-profile.v0.3.0.schema.json",
	}

	for id, path := range resources {
		document, err := readJSONDocument(path)
		if err != nil {
			return nil, err
		}
		if err := compiler.AddResource(id, document); err != nil {
			return nil, fmt.Errorf("add schema resource %s: %w", id, err)
		}
	}

	agentCard, err := compiler.Compile(agentCardSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Agent Card schema: %w", err)
	}
	platformError, err := compiler.Compile(platformErrorSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile platform error schema: %w", err)
	}
	installation, err := compiler.Compile(installationSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile installation schema: %w", err)
	}
	invocationEvent, err := compiler.Compile(invocationEventSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile invocation event schema: %w", err)
	}
	a2aProfile, err := compiler.Compile(a2aProfileSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile A2A profile schema: %w", err)
	}

	return &Validator{
		agentCard:       agentCard,
		platformError:   platformError,
		installation:    installation,
		invocationEvent: invocationEvent,
		a2aProfile:      a2aProfile,
	}, nil
}

func (v *Validator) ValidateAgentCard(card AgentCard) error {
	if err := validateMappedValue(v.agentCard, card); err != nil {
		return fmt.Errorf("validate Agent Card schema: %w", err)
	}

	issues := make([]SemanticIssue, 0)
	permissions := make(map[string]struct{}, len(card.Permissions))
	for index, permission := range card.Permissions {
		if _, exists := permissions[permission.ID]; exists {
			issues = append(issues, SemanticIssue{
				Path:    fmt.Sprintf("/permissions/%d/id", index),
				Message: "duplicate permission id",
			})
		}
		permissions[permission.ID] = struct{}{}
	}

	skills := make(map[string]struct{}, len(card.Skills))
	for skillIndex, skill := range card.Skills {
		if _, exists := skills[skill.ID]; exists {
			issues = append(issues, SemanticIssue{
				Path:    fmt.Sprintf("/skills/%d/id", skillIndex),
				Message: "duplicate skill id",
			})
		}
		skills[skill.ID] = struct{}{}

		for permissionIndex, permissionID := range skill.RequiredPermissions {
			if _, declared := permissions[permissionID]; !declared {
				issues = append(issues, SemanticIssue{
					Path:    fmt.Sprintf("/skills/%d/requiredPermissions/%d", skillIndex, permissionIndex),
					Message: "required permission is not declared",
				})
			}
		}
	}

	if len(issues) > 0 {
		return &SemanticValidationError{Issues: issues}
	}
	return nil
}

func (v *Validator) DecodeAgentCard(data []byte) (AgentCard, error) {
	var card AgentCard
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&card); err != nil {
		return AgentCard{}, fmt.Errorf("decode Agent Card: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return AgentCard{}, fmt.Errorf("decode Agent Card: %w", err)
	}
	if err := v.ValidateAgentCard(card); err != nil {
		return AgentCard{}, err
	}
	return card, nil
}

func (v *Validator) ValidateInstallation(installation Installation) error {
	return validateMappedValue(v.installation, installation)
}

func (v *Validator) ValidateInvocationEvent(event InvocationEvent) error {
	return validateMappedValue(v.invocationEvent, event)
}

func (v *Validator) ValidatePlatformError(platformError PlatformError) error {
	return validateMappedValue(v.platformError, platformError)
}

func (v *Validator) ValidateA2AProfile(profile A2AProfile) error {
	return validateMappedValue(v.a2aProfile, profile)
}

func LoadA2AProfile() (A2AProfile, error) {
	data, err := contractFiles.ReadFile("a2a-profile/v0.3.0.json")
	if err != nil {
		return A2AProfile{}, fmt.Errorf("read A2A profile: %w", err)
	}
	var profile A2AProfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return A2AProfile{}, fmt.Errorf("decode A2A profile: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return A2AProfile{}, fmt.Errorf("decode A2A profile: %w", err)
	}
	return profile, nil
}

func ContractFiles() fs.FS {
	return contractFiles
}

func readJSONDocument(path string) (any, error) {
	data, err := contractFiles.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return document, nil
}

func validateMappedValue(schema *jsonschema.Schema, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode mapped contract: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode mapped contract: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		var validationError *jsonschema.ValidationError
		if !errors.As(err, &validationError) {
			return fmt.Errorf("validate contract: %w", err)
		}
		violation := firstViolation(validationError)
		return &SchemaValidationError{
			InstancePath: "/" + strings.Join(violation.InstanceLocation, "/"),
			Keyword:      strings.Join(violation.ErrorKind.KeywordPath(), "/"),
		}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func firstViolation(validationError *jsonschema.ValidationError) *jsonschema.ValidationError {
	if len(validationError.Causes) == 0 {
		return validationError
	}
	return firstViolation(validationError.Causes[0])
}
