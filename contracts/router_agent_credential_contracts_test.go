package contracts

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestRouterAgentCredentialContractIsDiscoverableAndConformant(t *testing.T) {
	manifest, err := LoadRouterAgentCredentialConformanceManifestV1()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range manifest.Cases {
		t.Run(testCase.ID, func(t *testing.T) {
			path := filepath.ToSlash(filepath.Join("router-agent-credential/v1/conformance", testCase.File))
			data, err := fs.ReadFile(ContractFiles(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err := rejectDuplicateJSONMemberNames(data); err != nil {
				t.Fatal(err)
			}
			if testCase.Kind == "claims" {
				var claims RouterInvocationCredentialClaimsV1
				decoder := json.NewDecoder(bytes.NewReader(data))
				if err := decoder.Decode(&claims); err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateRouterInvocationCredentialClaimsV1(claims, time.Unix(1784700031, 0))
				if (validationErr == nil) != testCase.ExpectedValid {
					t.Fatalf("semantic validation error=%v expectedValid=%v", validationErr, testCase.ExpectedValid)
				}
				return
			}
			var headers map[string][]string
			if err := json.Unmarshal(data, &headers); err != nil {
				t.Fatal(err)
			}
			header := func(name string) []string {
				for candidate, values := range headers {
					if strings.EqualFold(candidate, name) {
						return values
					}
				}
				return nil
			}
			valid := len(header(RouterAgentAuthorizationHeader)) == 1 && len(header(RouterAgentWorkspaceHeader)) == 1 && len(header(RouterAgentTargetAgentHeader)) == 1 && len(header(RouterAgentCardVersionHeader)) == 1 && len(header(RouterAgentReleaseHeader)) == 1 && len(header(RouterAgentCardDigestHeader)) == 1 && len(header(RouterAgentCapabilityHeader)) == 1 && len(header(RouterAgentInvocationHeader)) == 1 && len(header(RouterAgentRootTaskHeader)) == 1 && len(header(RouterAgentTraceHeader)) == 1
			if parent := header(RouterAgentParentInvocationHeader); parent != nil {
				valid = valid && len(parent) == 1 && strings.TrimSpace(parent[0]) != ""
			}
			if valid != testCase.ExpectedValid {
				t.Fatalf("header validity=%v expectedValid=%v", valid, testCase.ExpectedValid)
			}
		})
	}
}

func TestRouterAgentCredentialSchemaRejectsUnknownMembersAndAcceptsValidClaims(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	commonData, err := fs.ReadFile(ContractFiles(), "schemas/common.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	commonDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(commonData))
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource(commonSchemaID, commonDocument); err != nil {
		t.Fatal(err)
	}
	schemaData, err := fs.ReadFile(ContractFiles(), "schemas/router-agent-credential.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("https://schemas.nekiro.dev/router-agent-credential/v1", schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("https://schemas.nekiro.dev/router-agent-credential/v1")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := fs.ReadFile(ContractFiles(), "router-agent-credential/v1/conformance/valid-root.json")
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("valid claims rejected: %v", err)
	}
	document.(map[string]any)["unknown"] = true
	if err := schema.Validate(document); err == nil {
		t.Fatal("unknown claim accepted")
	}
}

func TestRouterAgentCredentialFieldsStayOutOfPersistentAndResultContracts(t *testing.T) {
	for _, file := range []string{
		"schemas/agent-card.v0.2.schema.json",
		"schemas/invocation-event.v0.3.schema.json",
		"schemas/invocation-result.v1.schema.json",
		"schemas/invocation-result-stream-event.v2.schema.json",
	} {
		document := readContractJSONObject(t, file)
		properties := requiredJSONObject(t, document, "properties")
		assertObjectKeysAbsent(t, file, properties, "authorization", "credential", "jwt", "jti", "signature", "privateKey", "publicKey")
	}
}
