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
	agentCardSchemaID       = "https://schemas.nekiro.dev/agent-card/v0.2"
	workspaceSchemaID       = "https://schemas.nekiro.dev/workspace/v1"
	platformErrorSchemaID   = platformErrorV2SchemaID
	installationSchemaID    = "https://schemas.nekiro.dev/installation/v2"
	invocationEventSchemaID = invocationEventV02SchemaID
	a2aProfileSchemaID      = "https://schemas.nekiro.dev/a2a-profile/v0.2"
)

//go:embed schemas/*.json openapi/*.yaml a2a-profile/*.json a2a-profile/v0.3.0/*.json a2a-profile/v0.3.0/conformance/*.json a2a-profile/v0.3.0/conformance/*.sse agent-card/v0.2/semantic-rules.md agent-card/v0.2/conformance/*.json invocation/v1/semantic-rules.md invocation/v1/conformance/*.json invocation-runtime/v1/semantic-rules.md invocation-runtime/v1/conformance/*.json installation/v2/semantic-rules.md
var contractFiles embed.FS

type Validator struct {
	agentCard                   *jsonschema.Schema
	workspace                   *jsonschema.Schema
	platformError               *jsonschema.Schema
	installation                *jsonschema.Schema
	invocationEvent             *jsonschema.Schema
	invocationResult            *jsonschema.Schema
	invocationResultStreamEvent *jsonschema.Schema
	a2aProfile                  *jsonschema.Schema
	resultContracts             *ResultContractValidator
}

type SemanticIssue struct {
	RuleID  AgentCardSemanticRuleID
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
		commonSchemaID:       "schemas/common.v1.schema.json",
		agentCardSchemaID:    "schemas/agent-card.v0.2.schema.json",
		workspaceSchemaID:    "schemas/workspace.v1.schema.json",
		installationSchemaID: "schemas/installation.v2.schema.json",
		a2aProfileSchemaID:   "schemas/a2a-profile.v0.2.schema.json",
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
	workspace, err := compiler.Compile(workspaceSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Workspace schema: %w", err)
	}
	installation, err := compiler.Compile(installationSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile installation schema: %w", err)
	}
	a2aProfile, err := compiler.Compile(a2aProfileSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile A2A profile schema: %w", err)
	}

	resultContracts, err := NewResultContractValidator()
	if err != nil {
		return nil, fmt.Errorf("compile active result contracts: %w", err)
	}

	return &Validator{
		agentCard:                   agentCard,
		workspace:                   workspace,
		platformError:               resultContracts.platformError,
		installation:                installation,
		invocationEvent:             resultContracts.invocationEvent,
		invocationResult:            resultContracts.invocationResult,
		invocationResultStreamEvent: resultContracts.invocationResultStreamEvent,
		a2aProfile:                  a2aProfile,
		resultContracts:             resultContracts,
	}, nil
}

func (v *Validator) ValidateAgentCard(card AgentCard) error {
	inputLimitError := validatePositiveJSONInteger(card.Limits.MaxInputBytes, "/limits/maxInputBytes")
	outputLimitError := validatePositiveJSONInteger(card.Limits.MaxOutputBytes, "/limits/maxOutputBytes")
	if inputLimitError != nil {
		return inputLimitError
	}
	if outputLimitError != nil {
		return outputLimitError
	}

	schemaCard := card
	schemaCard.Limits.MaxInputBytes = json.Number("1")
	schemaCard.Limits.MaxOutputBytes = json.Number("1")
	if err := validateMappedValue(v.agentCard, schemaCard); err != nil {
		return fmt.Errorf("validate Agent Card schema: %w", err)
	}

	violations := EvaluateAgentCardSemantics(card)
	issues := make([]SemanticIssue, 0, len(violations))
	for _, violation := range violations {
		issues = append(issues, SemanticIssue{
			RuleID:  violation.RuleID,
			Path:    violation.Path,
			Message: fmt.Sprintf("violates %s", violation.RuleID),
		})
	}

	if len(issues) > 0 {
		return &SemanticValidationError{Issues: issues}
	}
	return nil
}

func validatePositiveJSONInteger(number json.Number, path string) error {
	text := number.String()
	if text == "" {
		return agentCardLimitValidationError(path, "type")
	}

	index := 0
	negative := false
	if text[index] == '-' {
		negative = true
		index++
		if index == len(text) {
			return agentCardLimitValidationError(path, "type")
		}
	}
	coefficientNonZero := false
	coefficientTrailingZeros := 0
	fractionDigits := 0

	if text[index] == '0' {
		index++
		coefficientTrailingZeros = 1
		if index < len(text) && text[index] >= '0' && text[index] <= '9' {
			return agentCardLimitValidationError(path, "type")
		}
	} else if text[index] >= '1' && text[index] <= '9' {
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			if text[index] == '0' {
				coefficientTrailingZeros++
			} else {
				coefficientNonZero = true
				coefficientTrailingZeros = 0
			}
			index++
		}
	} else {
		return agentCardLimitValidationError(path, "type")
	}

	if index < len(text) && text[index] == '.' {
		index++
		fractionStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			if text[index] == '0' {
				coefficientTrailingZeros++
			} else {
				coefficientNonZero = true
				coefficientTrailingZeros = 0
			}
			fractionDigits++
			index++
		}
		if index == fractionStart {
			return agentCardLimitValidationError(path, "type")
		}
	}

	exponentNegative := false
	exponentMagnitude := 0
	exponentTooLarge := false
	if index < len(text) && (text[index] == 'e' || text[index] == 'E') {
		index++
		if index < len(text) && (text[index] == '+' || text[index] == '-') {
			exponentNegative = text[index] == '-'
			index++
		}
		exponentStart := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			digit := int(text[index] - '0')
			if !exponentTooLarge {
				limit := len(text)
				if exponentMagnitude > limit/10 ||
					(exponentMagnitude == limit/10 && digit > limit%10) {
					exponentTooLarge = true
				} else {
					exponentMagnitude = exponentMagnitude*10 + digit
				}
			}
			index++
		}
		if index == exponentStart {
			return agentCardLimitValidationError(path, "type")
		}
	}
	if index != len(text) {
		return agentCardLimitValidationError(path, "type")
	}
	if !coefficientNonZero {
		return agentCardLimitValidationError(path, "minimum")
	}
	if negative {
		return agentCardLimitValidationError(path, "minimum")
	}

	// coefficient * 10^(exponent-fractionDigits) is integral exactly when the
	// exponent covers every coefficient digit that is not already a trailing zero.
	minimumExponent := fractionDigits - coefficientTrailingZeros
	if !exponentAtLeast(exponentNegative, exponentMagnitude, exponentTooLarge, minimumExponent) {
		return agentCardLimitValidationError(path, "type")
	}
	return nil
}

func exponentAtLeast(negative bool, magnitude int, tooLarge bool, minimum int) bool {
	if magnitude == 0 && !tooLarge {
		negative = false
	}
	if !negative {
		return minimum <= 0 || tooLarge || magnitude >= minimum
	}
	if minimum >= 0 || tooLarge {
		return false
	}
	return magnitude <= -minimum
}

func agentCardLimitValidationError(path, keyword string) error {
	return fmt.Errorf("validate Agent Card schema: %w", &SchemaValidationError{
		InstancePath: path,
		Keyword:      keyword,
	})
}

func (v *Validator) DecodeAgentCard(data []byte) (AgentCard, error) {
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return AgentCard{}, fmt.Errorf("decode Agent Card: %w", err)
	}
	if err := rejectQuotedAgentCardLimitValues(data); err != nil {
		return AgentCard{}, fmt.Errorf("decode Agent Card: %w", err)
	}
	var card AgentCard
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
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

func (v *Validator) DecodeRegisterAgentRequest(data []byte) (RegisterAgentRequest, error) {
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return RegisterAgentRequest{}, fmt.Errorf("decode register Agent request: %w", err)
	}
	if err := rejectQuotedRegisterAgentCardLimitValues(data); err != nil {
		return RegisterAgentRequest{}, fmt.Errorf("decode register Agent request: %w", err)
	}
	var request RegisterAgentRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return RegisterAgentRequest{}, fmt.Errorf("decode register Agent request: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return RegisterAgentRequest{}, fmt.Errorf("decode register Agent request: %w", err)
	}
	if err := v.ValidateAgentCard(request.Card); err != nil {
		return RegisterAgentRequest{}, err
	}
	return request, nil
}

func rejectQuotedAgentCardLimitValues(data []byte) error {
	var document struct {
		Limits struct {
			MaxInputBytes  json.RawMessage `json:"maxInputBytes"`
			MaxOutputBytes json.RawMessage `json:"maxOutputBytes"`
		} `json:"limits"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	for _, field := range []struct {
		value json.RawMessage
		path  string
	}{
		{value: document.Limits.MaxInputBytes, path: "/limits/maxInputBytes"},
		{value: document.Limits.MaxOutputBytes, path: "/limits/maxOutputBytes"},
	} {
		if trimmed := bytes.TrimSpace(field.value); len(trimmed) > 0 && trimmed[0] == '"' {
			return agentCardLimitValidationError(field.path, "type")
		}
	}
	return nil
}

func rejectQuotedRegisterAgentCardLimitValues(data []byte) error {
	var envelope struct {
		Card json.RawMessage `json:"card"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Card) == 0 {
		return nil
	}
	return rejectQuotedAgentCardLimitValues(envelope.Card)
}

func (v *Validator) ValidateInstallation(installation Installation) error {
	if err := validateMappedValue(v.installation, installation); err != nil {
		return err
	}
	return validateInstallationV2Semantics(installation)
}

func (v *Validator) ValidateWorkspace(workspace Workspace) error {
	return validateMappedValue(v.workspace, workspace)
}

func (v *Validator) ValidateResolveAgentResponseForRequest(request ResolveAgentRequestV2, response ResolveAgentResponse) error {
	if err := ValidateResolveAgentRequestV1(request); err != nil {
		return err
	}
	if err := v.ValidateAgentCard(response.Card); err != nil {
		return err
	}
	if response.Card.AgentID != request.AgentID || response.Card.Version != request.Version {
		return errors.New("resolved Card identity does not match request")
	}
	if err := validateSafeContractIdentifier("installation id", response.Installation.InstallationID); err != nil {
		return err
	}
	if response.Installation.WorkspaceID != request.WorkspaceID ||
		response.Installation.AgentID != request.AgentID ||
		response.Installation.InstalledVersion != request.Version ||
		response.Installation.Status != "enabled" {
		return errors.New("resolved Installation identity does not match request")
	}
	if err := ValidateInvocationReleaseProvenance(response.Installation.InstalledReleaseID, response.Installation.AgentCardDigest); err != nil {
		return fmt.Errorf("resolved Installation release provenance is invalid: %w", err)
	}
	for index := 1; index < len(response.Installation.AcceptedPermissions); index++ {
		if response.Installation.AcceptedPermissions[index-1] >= response.Installation.AcceptedPermissions[index] {
			return errors.New("resolved Installation permissions are not canonical")
		}
	}
	declaredPermissions := make(map[string]struct{}, len(response.Card.Permissions))
	for _, permission := range response.Card.Permissions {
		declaredPermissions[permission.ID] = struct{}{}
	}
	acceptedPermissions := make(map[string]struct{}, len(response.Installation.AcceptedPermissions))
	for _, permission := range response.Installation.AcceptedPermissions {
		if err := validateSafeContractIdentifier("accepted permission", permission); err != nil {
			return err
		}
		if _, declared := declaredPermissions[permission]; !declared {
			return errors.New("resolved Installation contains an undeclared permission")
		}
		acceptedPermissions[permission] = struct{}{}
	}
	for _, skill := range response.Card.Skills {
		if skill.ID != request.Capability {
			continue
		}
		for _, permission := range skill.RequiredPermissions {
			if _, accepted := acceptedPermissions[permission]; !accepted {
				return errors.New("resolved Installation does not authorize requested capability")
			}
		}
		return nil
	}
	return errors.New("requested capability is not declared by resolved Card")
}

func (v *Validator) ValidateInvocationEvent(event InvocationEvent) error {
	return v.resultContracts.ValidateInvocationEvent(event)
}

func (v *Validator) ValidatePlatformError(platformError PlatformError) error {
	return v.resultContracts.ValidatePlatformError(platformError)
}

func (v *Validator) ValidatePlatformErrorV3(platformError PlatformErrorV3) error {
	return v.resultContracts.ValidatePlatformErrorV3(platformError)
}

func (v *Validator) ValidateInvocationResult(result InvocationResult) error {
	return v.resultContracts.ValidateInvocationResult(result)
}

func (v *Validator) ValidateInvocationResultForRequest(
	result InvocationResult,
	invocationID string,
	rootTaskID string,
	traceID TraceID,
) error {
	return v.resultContracts.ValidateInvocationResultForRequest(result, invocationID, rootTaskID, traceID)
}

func (v *Validator) ValidateInvocationResultStreamEvent(event InvocationResultStreamEvent) error {
	return v.resultContracts.ValidateInvocationResultStreamEvent(event)
}

func (v *Validator) ValidateA2AProfile(profile A2AProfile) error {
	return validateMappedValue(v.a2aProfile, profile)
}

func LoadA2AProfile() (A2AProfile, error) {
	return LoadA2AProfileV02()
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
