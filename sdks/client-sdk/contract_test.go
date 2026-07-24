package clientsdk

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

func TestFrozenPublicStructSurfaces(t *testing.T) {
	assertPublicFields(t, reflect.TypeFor[Config](), []fieldContract{
		{name: "HTTPClient", kind: reflect.TypeFor[*http.Client]()},
		{name: "GatewayOrigin", kind: reflect.TypeFor[string]()},
		{name: "WorkspaceID", kind: reflect.TypeFor[string]()},
		{name: "ApplicationCredential", kind: reflect.TypeFor[string]()},
		{name: "RequestLimitBytes", kind: reflect.TypeFor[int64]()},
		{name: "ResponseLimitBytes", kind: reflect.TypeFor[int64]()},
		{name: "StreamEventLimitBytes", kind: reflect.TypeFor[int64]()},
	})
	assertPublicFields(t, reflect.TypeFor[InvokeRequest](), []fieldContract{
		{name: "AgentID", kind: reflect.TypeFor[string]()},
		{name: "Capability", kind: reflect.TypeFor[string]()},
		{name: "Input", kind: reflect.TypeFor[json.RawMessage]()},
	})
	assertPublicFields(t, reflect.TypeFor[Result](), []fieldContract{
		{name: "InvocationID", kind: reflect.TypeFor[string]()},
		{name: "RootTaskID", kind: reflect.TypeFor[string]()},
		{name: "TraceID", kind: reflect.TypeFor[contracts.TraceID]()},
		{name: "Output", kind: reflect.TypeFor[json.RawMessage]()},
	})
	credentialField, exists := reflect.TypeFor[Config]().FieldByName("ApplicationCredential")
	if !exists || credentialField.Tag.Get("json") != "-" {
		t.Fatal("Config application credential must be excluded from JSON serialization")
	}
	forbidden := []string{"Workspace", "Credential", "Endpoint", "Router", "Version", "Release", "Digest", "Trace", "Invocation", "Task", "Secret"}
	requestType := reflect.TypeFor[InvokeRequest]()
	for index := range requestType.NumField() {
		for _, fragment := range forbidden {
			if strings.Contains(requestType.Field(index).Name, fragment) {
				t.Fatalf("InvokeRequest exposes forbidden routing/security field %s", requestType.Field(index).Name)
			}
		}
	}
}

func TestClientSDKImportDirectionAndRouteAreFrozen(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	routeOccurrences := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, "/apps/") || strings.Contains(path, "/sdks/agent-sdk") {
				t.Fatalf("%s imports forbidden implementation package %s", file, path)
			}
			if strings.HasPrefix(path, "github.com/Nene7ko/NeKiro/") && path != "github.com/Nene7ko/NeKiro/contracts" {
				t.Fatalf("%s imports non-contract repository package %s", file, path)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING {
				value, _ := strconv.Unquote(literal.Value)
				routeOccurrences += strings.Count(value, "/v4/workspaces/")
				if strings.Contains(value, "/internal/") || strings.Contains(value, "/agent/v1/") || strings.Contains(value, "/v3/workspaces/") {
					t.Fatalf("%s contains forbidden route %q", file, value)
				}
			}
			return true
		})
	}
	if routeOccurrences != 1 {
		t.Fatalf("active Gateway invocation route occurrences=%d, want 1", routeOccurrences)
	}
}

type fieldContract struct {
	name string
	kind reflect.Type
}

func assertPublicFields(t *testing.T, value reflect.Type, expected []fieldContract) {
	t.Helper()
	if value.NumField() != len(expected) {
		t.Fatalf("%s fields=%d, want %d", value, value.NumField(), len(expected))
	}
	for index, contract := range expected {
		field := value.Field(index)
		if field.Name != contract.name || field.Type != contract.kind || !field.IsExported() {
			t.Fatalf("%s field %d=%s %v exported=%v, want %s %v", value, index, field.Name, field.Type, field.IsExported(), contract.name, contract.kind)
		}
	}
}
