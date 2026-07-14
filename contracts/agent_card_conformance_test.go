package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	agentCardV02SchemaID = "https://schemas.nekiro.dev/agent-card/v0.2"
	commonV1SchemaID     = "https://schemas.nekiro.dev/common/v1"
)

func TestAgentCardConformance(t *testing.T) {
	schema := compileAgentCardV02Schema(t)
	manifestPath := filepath.Join("agent-card", "v0.2", "conformance", "manifest.json")
	manifest := loadAgentCardConformanceManifest(t, manifestPath)
	requiredCaseIDs := []string{
		"valid-baseline",
		"valid-shared-permission",
		"valid-cross-version-permission-source",
		"invalid-duplicate-skill-id",
		"invalid-duplicate-permission-id",
		"invalid-undeclared-permission",
		"invalid-cross-version-permission",
		"invalid-case-mismatched-permission",
		"invalid-structural-missing-name",
		"invalid-endpoint-userinfo-credentials",
		"invalid-endpoint-userinfo-empty",
	}
	caseIDs := make(map[string]struct{}, len(manifest.Cases))

	for _, testCase := range manifest.Cases {
		t.Run(testCase.ID, func(t *testing.T) {
			caseIDs[testCase.ID] = struct{}{}
			fixturePath := filepath.Join(filepath.Dir(manifestPath), testCase.File)
			card, actualRuleIDs, structuralErr := evaluateAgentCardFixture(t, schema, fixturePath)
			if structuralErr != nil && len(testCase.ViolatedRules) > 0 {
				t.Fatalf("semantic fixture failed structural validation: %v", structuralErr)
			}

			crossVersionReferenceProven := false
			for _, contextFile := range testCase.ContextFiles {
				contextPath := filepath.Join(filepath.Dir(manifestPath), contextFile)
				contextCard, contextRuleIDs, contextStructuralErr := evaluateAgentCardFixture(t, schema, contextPath)
				if contextStructuralErr != nil {
					t.Fatalf("context fixture %q failed structural validation: %v", contextFile, contextStructuralErr)
				}
				if len(contextRuleIDs) > 0 {
					t.Fatalf("context fixture %q failed semantic validation with rules %v", contextFile, contextRuleIDs)
				}
				if contextCard.AgentID != card.AgentID {
					t.Fatalf("context fixture %q agent id = %q, primary agent id = %q", contextFile, contextCard.AgentID, card.AgentID)
				}
				if contextCard.Version == card.Version {
					t.Fatalf("context fixture %q uses primary Agent version %q", contextFile, card.Version)
				}
				if contextDeclaresMissingPermission(card, contextCard) {
					crossVersionReferenceProven = true
				}
			}
			if len(testCase.ContextFiles) > 0 && !crossVersionReferenceProven {
				t.Fatal("context fixtures do not declare a permission missing from the primary Card version")
			}

			actualValid := structuralErr == nil && len(actualRuleIDs) == 0
			if actualValid != testCase.Valid {
				t.Fatalf("combined validity = %t, want %t (structural error: %v, rules: %v)", actualValid, testCase.Valid, structuralErr, actualRuleIDs)
			}

			wantRuleIDs := slices.Clone(testCase.ViolatedRules)
			slices.Sort(wantRuleIDs)
			if !slices.Equal(actualRuleIDs, wantRuleIDs) {
				t.Fatalf("violated rules = %v, want %v", actualRuleIDs, wantRuleIDs)
			}
		})
	}

	for _, caseID := range requiredCaseIDs {
		if _, exists := caseIDs[caseID]; !exists {
			t.Errorf("Agent Card conformance manifest is missing required case %q", caseID)
		}
	}
}

func TestAgentCardConformanceManifestRequiresExplicitFields(t *testing.T) {
	testCases := []struct {
		name     string
		document string
	}{
		{name: "missing cases", document: `{}`},
		{name: "null cases", document: `{"cases":null}`},
		{name: "empty cases", document: `{"cases":[]}`},
		{name: "missing id", document: `{"cases":[{"file":"card.json","valid":false,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "null id", document: `{"cases":[{"id":null,"file":"card.json","valid":false,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "missing file", document: `{"cases":[{"id":"case","valid":false,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "null file", document: `{"cases":[{"id":"case","file":null,"valid":false,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "missing valid", document: `{"cases":[{"id":"case","file":"card.json","violatedRules":[],"contextFiles":[]}]}`},
		{name: "null valid", document: `{"cases":[{"id":"case","file":"card.json","valid":null,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "missing violated rules", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"contextFiles":[]}]}`},
		{name: "null violated rules", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":null,"contextFiles":[]}]}`},
		{name: "missing context files", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":[]}]}`},
		{name: "null context files", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":[],"contextFiles":null}]}`},
		{name: "unknown root field", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":[],"contextFiles":[]}],"unknown":true}`},
		{name: "unknown case field", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":[],"contextFiles":[],"unknown":true}]}`},
		{name: "unknown rule id", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":["AC-SEM-999"],"contextFiles":[]}]}`},
		{name: "valid with violated rule", document: `{"cases":[{"id":"case","file":"card.json","valid":true,"violatedRules":["AC-SEM-001"],"contextFiles":[]}]}`},
		{name: "duplicate case id", document: `{"cases":[{"id":"case","file":"one.json","valid":true,"violatedRules":[],"contextFiles":[]},{"id":"case","file":"two.json","valid":true,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "duplicate root member", document: `{"cases":[{"id":"case-one","file":"one.json","valid":true,"violatedRules":[],"contextFiles":[]}],"cases":[{"id":"case-two","file":"two.json","valid":true,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "duplicate case member", document: `{"cases":[{"id":"case","file":"card.json","valid":false,"valid":true,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "escaped duplicate case member", document: `{"cases":[{"id":"case","\u0069d":"other","file":"card.json","valid":false,"violatedRules":[],"contextFiles":[]}]}`},
		{name: "trailing JSON", document: `{"cases":[{"id":"case","file":"card.json","valid":true,"violatedRules":[],"contextFiles":[]}]} {}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := DecodeAgentCardConformanceManifest([]byte(testCase.document)); err == nil {
				t.Fatal("malformed Agent Card conformance manifest was accepted")
			}
		})
	}
}

func TestAgentCardConformanceManifestRejectsUnsafeFixturePaths(t *testing.T) {
	type pathTestCase struct {
		name string
		path string
	}
	testCases := []pathTestCase{
		{name: "empty", path: ""},
		{name: "absolute POSIX", path: "/card.json"},
		{name: "absolute Windows drive", path: "C:/card.json"},
		{name: "HTTP URI", path: "https://example.test/card.json"},
		{name: "file URI", path: "file:card.json"},
		{name: "backslash", path: `nested\card.json`},
		{name: "empty middle segment", path: "nested//card.json"},
		{name: "empty trailing segment", path: "nested/card.json/"},
		{name: "current directory segment", path: "nested/./card.json"},
		{name: "parent directory segment", path: "nested/../card.json"},
		{name: "encoded traversal", path: "nested/%2e%2e/card.json"},
		{name: "platform-equivalent traversal", path: "nested/.. /card.json"},
		{name: "nonportable colon", path: "nested/name:card.json"},
		{name: "reserved CON", path: "nested/CON"},
		{name: "reserved CON mixed case extension", path: "nested/CoN.json"},
		{name: "reserved PRN extension", path: "nested/prn.txt"},
		{name: "reserved AUX", path: "nested/AUX"},
		{name: "reserved NUL extension", path: "nested/nul.json"},
		{name: "trailing dot", path: "nested/card.json."},
		{name: "trailing space", path: "nested/card.json "},
	}
	for number := 1; number <= 9; number++ {
		testCases = append(testCases,
			pathTestCase{name: fmt.Sprintf("reserved COM%d", number), path: fmt.Sprintf("nested/CoM%d.json", number)},
			pathTestCase{name: fmt.Sprintf("reserved LPT%d", number), path: fmt.Sprintf("nested/LpT%d.log", number)},
		)
	}
	for _, character := range []rune{'<', '>', ':', '"', '|', '?', '*'} {
		testCases = append(testCases, pathTestCase{
			name: fmt.Sprintf("Windows-invalid %q", character),
			path: "nested/name" + string(character) + "card.json",
		})
	}
	for character := rune(0); character <= 0x1f; character++ {
		testCases = append(testCases, pathTestCase{
			name: fmt.Sprintf("ASCII control 0x%02X", character),
			path: "nested/name" + string(character) + "card.json",
		})
	}
	testCases = append(testCases, pathTestCase{
		name: "ASCII control DEL",
		path: "nested/name" + string(rune(0x7f)) + "card.json",
	})

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			encodedPath, err := json.Marshal(testCase.path)
			if err != nil {
				t.Fatalf("encode fixture path: %v", err)
			}

			primaryDocument := []byte(`{"cases":[{"id":"case","file":` + string(encodedPath) + `,"valid":false,"violatedRules":[],"contextFiles":[]}]}`)
			if _, err := DecodeAgentCardConformanceManifest(primaryDocument); err == nil {
				t.Fatal("unsafe primary fixture path was accepted")
			}

			contextDocument := []byte(`{"cases":[{"id":"case","file":"card.json","valid":false,"violatedRules":[],"contextFiles":[` + string(encodedPath) + `]}]}`)
			if _, err := DecodeAgentCardConformanceManifest(contextDocument); err == nil {
				t.Fatal("unsafe context fixture path was accepted")
			}
		})
	}

	for _, fixturePath := range []string{
		"card.json",
		"nested/card.json",
		".well-known/card.json",
		"devices/console.json",
		"devices/com0.json",
		"devices/com10.json",
		"devices/lpt0.json",
		"devices/lpt10.json",
		"devices/auxiliary.json",
		"devices/null.json",
	} {
		t.Run("allowed "+fixturePath, func(t *testing.T) {
			encodedPath, err := json.Marshal(fixturePath)
			if err != nil {
				t.Fatalf("encode fixture path: %v", err)
			}
			canonical := []byte(`{"cases":[{"id":"case","file":` + string(encodedPath) + `,"valid":false,"violatedRules":[],"contextFiles":["related/context.json"]}]}`)
			if _, err := DecodeAgentCardConformanceManifest(canonical); err != nil {
				t.Fatalf("canonical fixture path was rejected: %v", err)
			}
		})
	}
}

func TestAgentCardEndpointRejectsURIUserinfoForms(t *testing.T) {
	schema := compileAgentCardV02Schema(t)
	fixturePath := filepath.Join("agent-card", "v0.2", "conformance", "valid-baseline.json")
	card, ruleIDs, structuralErr := evaluateAgentCardFixture(t, schema, fixturePath)
	if structuralErr != nil || len(ruleIDs) > 0 {
		t.Fatalf("baseline fixture is not conformant: structural error %v, rules %v", structuralErr, ruleIDs)
	}

	for _, endpoint := range []string{
		"https://alice@agent.example.test/a2a",
		"https://alice:secret@agent.example.test/a2a",
		"https://@agent.example.test/a2a",
		"https://:@agent.example.test/a2a",
	} {
		t.Run(endpoint, func(t *testing.T) {
			candidate := card
			candidate.Protocol.Endpoint = endpoint
			if violations := EvaluateAgentCardSemantics(candidate); len(violations) > 0 {
				t.Fatalf("userinfo candidate unexpectedly failed semantic rules: %v", violations)
			}
			data, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("encode userinfo candidate: %v", err)
			}
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("decode userinfo candidate: %v", err)
			}
			if err := schema.Validate(document); err == nil {
				t.Fatal("endpoint URI userinfo was structurally accepted")
			}
		})
	}

	card.Protocol.Endpoint = "https://agent.example.test/a2a/@self?notify=user@example.test"
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("encode endpoint with non-authority at-sign: %v", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode endpoint with non-authority at-sign: %v", err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("endpoint with non-authority at-sign was rejected: %v", err)
	}
}

func TestAgentCardConformanceManifestPreservesExplicitZeroValues(t *testing.T) {
	document := []byte(`{"cases":[{"id":"structural-invalid","file":"card.json","valid":false,"violatedRules":[],"contextFiles":[]}]}`)
	manifest, err := DecodeAgentCardConformanceManifest(document)
	if err != nil {
		t.Fatalf("decode explicit false and empty arrays: %v", err)
	}
	manifestCase := manifest.Cases[0]
	if manifestCase.Valid {
		t.Fatal("explicit false validity decoded as true")
	}
	if manifestCase.ViolatedRules == nil {
		t.Fatal("explicit empty violatedRules decoded as absent")
	}
	if manifestCase.ContextFiles == nil {
		t.Fatal("explicit empty contextFiles decoded as absent")
	}
}

func compileAgentCardV02Schema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()

	commonSchema, err := readJSONDocument("schemas/common.v1.schema.json")
	if err != nil {
		t.Fatalf("read common schema: %v", err)
	}
	if err := compiler.AddResource(commonV1SchemaID, commonSchema); err != nil {
		t.Fatalf("add common schema: %v", err)
	}

	agentCardSchema, err := readJSONDocument("schemas/agent-card.v0.2.schema.json")
	if err != nil {
		t.Fatalf("read Agent Card v0.2 schema: %v", err)
	}
	if err := compiler.AddResource(agentCardV02SchemaID, agentCardSchema); err != nil {
		t.Fatalf("add Agent Card v0.2 schema: %v", err)
	}

	compiled, err := compiler.Compile(agentCardV02SchemaID)
	if err != nil {
		t.Fatalf("compile Agent Card v0.2 schema: %v", err)
	}
	return compiled
}

func loadAgentCardConformanceManifest(t *testing.T, path string) AgentCardConformanceManifest {
	t.Helper()
	data := readConformanceFixture(t, path)
	manifest, err := DecodeAgentCardConformanceManifest(data)
	if err != nil {
		t.Fatalf("decode Agent Card conformance manifest: %v", err)
	}
	return manifest
}

func evaluateAgentCardFixture(t *testing.T, schema *jsonschema.Schema, path string) (AgentCard, []AgentCardSemanticRuleID, error) {
	t.Helper()
	fixture := readConformanceFixture(t, path)
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(fixture))
	if err != nil {
		t.Fatalf("decode raw fixture %q: %v", path, err)
	}
	if err := schema.Validate(document); err != nil {
		return AgentCard{}, nil, err
	}

	var card AgentCard
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&card); err != nil {
		t.Fatalf("decode structurally valid fixture %q into Go mapping: %v", path, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		t.Fatalf("decode structurally valid fixture %q into Go mapping: %v", path, err)
	}
	return card, uniqueSemanticRuleIDs(EvaluateAgentCardSemantics(card)), nil
}

func contextDeclaresMissingPermission(primary AgentCard, context AgentCard) bool {
	primaryPermissions := make(map[string]struct{}, len(primary.Permissions))
	for _, permission := range primary.Permissions {
		primaryPermissions[permission.ID] = struct{}{}
	}
	contextPermissions := make(map[string]struct{}, len(context.Permissions))
	for _, permission := range context.Permissions {
		contextPermissions[permission.ID] = struct{}{}
	}

	for _, skill := range primary.Skills {
		for _, permissionID := range skill.RequiredPermissions {
			_, declaredByPrimary := primaryPermissions[permissionID]
			_, declaredByContext := contextPermissions[permissionID]
			if !declaredByPrimary && declaredByContext {
				return true
			}
		}
	}
	return false
}

func readConformanceFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func uniqueSemanticRuleIDs(violations []AgentCardSemanticViolation) []AgentCardSemanticRuleID {
	seen := make(map[AgentCardSemanticRuleID]struct{}, len(violations))
	ruleIDs := make([]AgentCardSemanticRuleID, 0, len(violations))
	for _, violation := range violations {
		if _, exists := seen[violation.RuleID]; exists {
			continue
		}
		seen[violation.RuleID] = struct{}{}
		ruleIDs = append(ruleIDs, violation.RuleID)
	}
	slices.Sort(ruleIDs)
	return ruleIDs
}
