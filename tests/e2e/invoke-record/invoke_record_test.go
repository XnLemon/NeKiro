//go:build e2e

package invokerecord_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const acceptanceWorkspace = "workspace-acceptance"

var routerCredentialPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_-])([A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)($|[^A-Za-z0-9_-])`)
var ed25519SignaturePattern = regexp.MustCompile(`(^|[^A-Za-z0-9_-])([A-Za-z0-9_-]{86})($|[^A-Za-z0-9_-])`)

type acceptanceEnv struct {
	controlPlane        string
	routerURL           string
	routerToken         string
	ownerToken          string
	userToken           string
	otherToken          string
	databaseURL         string
	composeFile         string
	credentialForbidden []string
	forbidden           []string
}

type httpResult struct {
	status int
	header http.Header
	body   []byte
}

func TestInvokeToRecordAcceptance(t *testing.T) {
	env := loadAcceptanceEnv(t)
	env.credentialForbidden = []string{
		"acceptance-owner-token", "acceptance-user-token", "acceptance-other-token",
		"router-internal-token", "control-plane-internal-token", "runtime-a-router-token",
		"acceptance-only-password",
		"rtj_", "nekiro-router+jwt",
	}
	env.credentialForbidden = append(env.credentialForbidden, env.ownerToken, env.userToken, env.otherToken, env.routerToken)
	for _, name := range []string{
		"NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN",
		"NEKIRO_CONTROL_PLANE_SERVICE_TOKEN",
		"RUNTIME_A_ROUTER_TOKEN",
		"NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID",
		"NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL",
		"NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL",
	} {
		env.credentialForbidden = append(env.credentialForbidden, requiredEnv(t, name))
	}
	databaseURL, err := url.Parse(env.databaseURL)
	if err != nil {
		t.Fatalf("parse E2E database URL for secrecy scan: %v", err)
	}
	if databaseURL.User == nil {
		t.Fatal("E2E database URL must contain credentials for secrecy scan")
	}
	password, ok := databaseURL.User.Password()
	if !ok || password == "" {
		t.Fatal("E2E database URL must contain a non-empty password for secrecy scan")
	}
	env.credentialForbidden = append(env.credentialForbidden, password)
	env.forbidden = append([]string{
		"direct-json-value", "direct-sse-value", "nested-value", "concurrent-",
		"policy-content-secret", "protocol-content-secret", "agent-content-secret",
		"route-content-secret", "timeout-content-secret", "cancel-content-secret",
		"interrupted-content-secret", "dependency-content-secret", "dependency-raw-secret",
	}, env.credentialForbidden...)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Timeout: 45 * time.Second}
	if result := doRequest(t, client, env.controlPlane+"/readyz", http.MethodGet, "", "", nil); result.status != http.StatusNoContent {
		t.Fatalf("Control Plane readiness status=%d body=%s", result.status, result.body)
	}
	assertDirectAgentRequestIsRejected(t, env.composeFile)

	runtimeA := acceptanceCard("runtime-a", "Runtime A", "http://runtime-a:8091", "runtime.cross", nil, false)
	runtimeB := acceptanceCard("runtime-b", "Runtime B", "http://runtime-b:8092", "runtime.echo", []string{"text.read"}, true)
	runtimeProtocol := acceptanceCard("runtime-protocol", "Runtime Protocol Fixture", "http://runtime-b:8092", "runtime.protocol", nil, false)
	runtimeRoute := acceptanceCard("runtime-route", "Runtime Route Fixture", "http://runtime-b:8092/unavailable", "runtime.route", nil, false)
	runtimeTimeout := acceptanceCardWithTimeout("runtime-timeout", "Runtime Timeout Fixture", "http://runtime-b:8092", "runtime.timeout", nil, true, 50)
	runtimeInterrupted := acceptanceCard("runtime-interrupted", "Runtime Interrupted Fixture", "http://runtime-b:8092", "runtime.interrupted", nil, true)
	registerAndPublish(t, client, env, runtimeA)
	registerAndPublish(t, client, env, runtimeB)
	registerAndPublish(t, client, env, runtimeProtocol)
	registerAndPublish(t, client, env, runtimeRoute)
	registerAndPublish(t, client, env, runtimeTimeout)
	registerAndPublish(t, client, env, runtimeInterrupted)

	discovery := doRequest(t, client, env.controlPlane+"/v3/agents?capability=runtime.echo", http.MethodGet, env.userToken, "", nil)
	if discovery.status != http.StatusOK || !bytes.Contains(discovery.body, []byte(`"agentId":"runtime-b"`)) {
		t.Fatalf("discovery status=%d body=%s", discovery.status, discovery.body)
	}
	assertNoForbiddenBody(t, discovery.body, env.forbidden, "Discovery Card response")
	createWorkspace(t, client, env, acceptanceWorkspace, env.ownerToken)
	install(t, client, env, acceptanceWorkspace, "runtime-a", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-b", []string{"text.read"})
	install(t, client, env, acceptanceWorkspace, "runtime-protocol", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-route", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-timeout", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-interrupted", []string{})

	direct := invokeJSON(t, client, env, "runtime-b", "runtime.echo", map[string]any{"fixture": "success", "value": "direct-json-value"})
	if direct.result.Status != "succeeded" || !bytes.Contains(direct.result.Result, []byte("direct-json-value")) {
		t.Fatalf("direct JSON result=%s", direct.result.Result)
	}
	assertRecord(t, client, env, direct.result.InvocationID, acceptanceWorkspace, "runtime-b", "succeeded", "")

	stream := invokeSSE(t, client, env, "runtime-b", "runtime.echo", map[string]any{"fixture": "stream-success", "value": "direct-sse-value"})
	if len(stream) < 3 || stream[0].Type != contracts.ResultStreamEventAccepted || stream[len(stream)-1].Type != contracts.ResultStreamEventCompleted {
		t.Fatalf("SSE sequence=%#v", stream)
	}
	for index, event := range stream {
		if event.Sequence != int64(index) || event.InvocationID == "" || event.RootTaskID == "" || event.TraceID == "" {
			t.Fatalf("SSE event[%d]=%#v", index, event)
		}
	}
	assertRecord(t, client, env, stream[0].InvocationID, acceptanceWorkspace, "runtime-b", "succeeded", "")

	nested := invokeJSON(t, client, env, "runtime-a", "runtime.cross", map[string]any{"fixture": "success", "value": "nested-value"})
	if nested.result.Status != "succeeded" || !bytes.Contains(nested.result.Result, []byte(`"runtime-a"`)) || !bytes.Contains(nested.result.Result, []byte(`"childInvocationId"`)) {
		t.Fatalf("nested result=%s", nested.result.Result)
	}
	trace := readTrace(t, client, env, nested.result.TraceID)
	if len(trace.Invocations) != 2 {
		t.Fatalf("nested trace invocations=%d body=%s", len(trace.Invocations), trace.raw)
	}
	if trace.Invocations[0].ParentInvocationID != "" || trace.Invocations[1].ParentInvocationID != trace.Invocations[0].InvocationID || trace.Invocations[0].RootTaskID != trace.Invocations[1].RootTaskID || trace.Invocations[0].TraceID != trace.Invocations[1].TraceID {
		t.Fatalf("nested lineage=%#v", trace.Invocations)
	}
	assertRecord(t, client, env, nested.result.InvocationID, acceptanceWorkspace, "runtime-a", "succeeded", "")

	otherWorkspace := "workspace-other"
	createWorkspace(t, client, env, otherWorkspace, env.otherToken)
	isolation := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/traces/%s", acceptanceWorkspace, nested.result.TraceID), http.MethodGet, env.otherToken, "", nil)
	if isolation.status != http.StatusForbidden {
		t.Fatalf("foreign trace read status=%d body=%s", isolation.status, isolation.body)
	}
	assertNoForbiddenBody(t, isolation.body, env.forbidden, "foreign Workspace response")
	invocationIsolation := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/invocations/%s", acceptanceWorkspace, nested.result.InvocationID), http.MethodGet, env.otherToken, "", nil)
	if invocationIsolation.status != http.StatusForbidden {
		t.Fatalf("foreign Invocation read status=%d body=%s", invocationIsolation.status, invocationIsolation.body)
	}
	assertNoForbiddenBody(t, invocationIsolation.body, env.forbidden, "foreign Invocation response")

	restartRouter(t, env.composeFile)
	waitForReady(t, client, env.routerURL+"/readyz", http.StatusOK)
	readAfterRestart := readTrace(t, client, env, nested.result.TraceID)
	if len(readAfterRestart.Invocations) != 2 {
		t.Fatalf("trace after Router restart=%#v", readAfterRestart.Invocations)
	}
	assertTraceRecords(t, client, env, readAfterRestart)

	assertFailureMatrix(t, client, env)
	assertConcurrentCalls(t, client, env)
	assertStorageAndLogsAreMetadataOnly(t, env)
}

func loadAcceptanceEnv(t *testing.T) acceptanceEnv {
	t.Helper()
	composeFile := requiredEnv(t, "NEKIRO_E2E_COMPOSE_FILE")
	if !filepath.IsAbs(composeFile) {
		t.Fatalf("NEKIRO_E2E_COMPOSE_FILE must be an absolute path")
	}
	return acceptanceEnv{
		controlPlane: requiredEnv(t, "NEKIRO_E2E_CONTROL_PLANE_URL"),
		routerURL:    requiredEnv(t, "NEKIRO_E2E_ROUTER_URL"),
		routerToken:  requiredEnv(t, "NEKIRO_E2E_ROUTER_TOKEN"),
		ownerToken:   requiredEnv(t, "NEKIRO_E2E_OWNER_TOKEN"),
		userToken:    requiredEnv(t, "NEKIRO_E2E_USER_TOKEN"),
		otherToken:   requiredEnv(t, "NEKIRO_E2E_OTHER_TOKEN"),
		databaseURL:  requiredEnv(t, "NEKIRO_E2E_DATABASE_URL"),
		composeFile:  composeFile,
	}
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value, exists := os.LookupEnv(name)
	if !exists || value == "" || strings.TrimSpace(value) != value {
		t.Fatalf("%s must be explicitly configured", name)
	}
	return value
}

func acceptanceCard(agentID, name, endpoint, capability string, permissions []string, streaming bool) []byte {
	return acceptanceCardWithTimeout(agentID, name, endpoint, capability, permissions, streaming, 30000)
}

func acceptanceCardWithTimeout(agentID, name, endpoint, capability string, permissions []string, streaming bool, timeoutMS int64) []byte {
	if permissions == nil {
		permissions = []string{}
	}
	card := contracts.AgentCard{
		SchemaVersion: contracts.AgentCardSchemaVersion, AgentID: agentID, Name: name,
		Description: "Deterministic acceptance Agent", Owner: contracts.AgentOwner{ID: "acceptance-owner", DisplayName: "Acceptance Owner"}, Version: "1.0.0",
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: contracts.A2AProtocolVersion, Transport: "JSONRPC", Endpoint: endpoint},
		Skills:         []contracts.AgentSkill{{ID: capability, Name: capability, Description: "Acceptance capability", InputSchema: contracts.JSONSchema{"type": "object"}, OutputSchema: contracts.JSONSchema{"type": "object"}, RequiredPermissions: permissions}},
		Authentication: contracts.AgentAuthentication{Type: "http_bearer"}, Limits: contracts.AgentLimits{TimeoutMS: timeoutMS, MaxInputBytes: json.Number("1048576"), MaxOutputBytes: json.Number("1048576"), Streaming: streaming},
	}
	card.Permissions = make([]contracts.PermissionDeclaration, 0, len(permissions))
	for _, permission := range permissions {
		card.Permissions = append(card.Permissions, contracts.PermissionDeclaration{ID: permission, Description: permission})
	}
	encoded, err := json.Marshal(card)
	if err != nil {
		panic(err)
	}
	return encoded
}

func assertDirectAgentRequestIsRejected(t *testing.T, composeFile string) {
	t.Helper()
	payload := `{"jsonrpc":"2.0","id":"direct-unauthenticated","method":"message/send","params":{"message":{"kind":"message","messageId":"direct-unauthenticated","role":"user","parts":[{"kind":"data","data":{"fixture":"success","value":"must-not-execute"}}]}}}`
	command := exec.CommandContext(t.Context(), "docker", "compose", "--file", composeFile, "exec", "-T", "runtime-b", "wget", "--server-response", "--output-document=-", "--post-data="+payload, "http://127.0.0.1:8092/")
	output, err := command.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("401 Unauthorized")) {
		t.Fatalf("direct unauthenticated Agent request was not rejected exactly: err=%v output=%s", err, output)
	}
}

func registerAndPublish(t *testing.T, client *http.Client, env acceptanceEnv, card []byte) {
	t.Helper()
	registered := doRequest(t, client, env.controlPlane+"/v3/agents", http.MethodPost, env.ownerToken, "application/json", map[string]any{"card": json.RawMessage(card)})
	if registered.status != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registered.status, registered.body)
	}
	assertNoForbiddenBody(t, registered.body, env.forbidden, "registered Card response")
	var value contracts.AgentCard
	if err := json.Unmarshal(card, &value); err != nil {
		t.Fatal(err)
	}
	const providerID = "provider-acceptance"
	bindingResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/agents/%s/endpoint-bindings", providerID, value.AgentID), http.MethodPost, env.ownerToken, "application/json", contracts.CreateEndpointBindingRequest{Endpoint: value.Protocol.Endpoint, Method: "http_well_known", Version: value.Version})
	if bindingResult.status != http.StatusCreated {
		t.Fatalf("create binding %s status=%d body=%s", value.AgentID, bindingResult.status, bindingResult.body)
	}
	assertNoForbiddenBody(t, bindingResult.body, env.forbidden, "endpoint binding response")
	var binding contracts.EndpointBindingResponse
	if err := json.Unmarshal(bindingResult.body, &binding); err != nil || binding.BindingID == "" || binding.VerificationStatus != "pending" {
		t.Fatalf("decode pending binding %s: value=%#v error=%v", value.AgentID, binding, err)
	}
	challengeResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/endpoint-bindings/%s/challenges", providerID, binding.BindingID), http.MethodPost, env.ownerToken, "", nil)
	if challengeResult.status != http.StatusCreated {
		t.Fatalf("create challenge %s status=%d", value.AgentID, challengeResult.status)
	}
	assertNoForbiddenBody(t, challengeResult.body, env.forbidden, "verification challenge response")
	var challenge contracts.VerificationChallengeResponse
	if err := json.Unmarshal(challengeResult.body, &challenge); err != nil || challenge.ChallengeID == "" || challenge.Proof == "" {
		t.Fatalf("decode challenge %s error=%v", value.AgentID, err)
	}
	service := challengeService(t, value.Protocol.Endpoint)
	completed := func() httpResult {
		writeChallengeProof(t, env.composeFile, service, challenge.ChallengeID, challenge.Proof)
		defer removeChallengeProof(t, env.composeFile, service, challenge.ChallengeID)
		return doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/endpoint-bindings/%s/challenges/%s/complete", providerID, binding.BindingID, challenge.ChallengeID), http.MethodPost, env.ownerToken, "", nil)
	}()
	if completed.status != http.StatusOK {
		t.Fatalf("complete challenge %s status=%d body=%s", value.AgentID, completed.status, completed.body)
	}
	assertNoForbiddenBody(t, completed.body, env.forbidden, "verified binding response")
	var verified contracts.EndpointBindingResponse
	if err := json.Unmarshal(completed.body, &verified); err != nil || verified.VerificationStatus != "verified" || verified.VerificationEvidenceDigest == nil {
		t.Fatalf("decode verified binding %s: value=%#v error=%v", value.AgentID, verified, err)
	}
	releaseResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/agents/%s/releases", providerID, value.AgentID), http.MethodPost, env.ownerToken, "application/json", contracts.CreateAgentReleaseRequest{Version: value.Version, EndpointBindingID: binding.BindingID})
	if releaseResult.status != http.StatusCreated {
		t.Fatalf("create release %s status=%d body=%s", value.AgentID, releaseResult.status, releaseResult.body)
	}
	assertNoForbiddenBody(t, releaseResult.body, env.forbidden, "agent release response")
	var release contracts.AgentReleaseResponse
	if err := json.Unmarshal(releaseResult.body, &release); err != nil || release.ReleaseID == "" || release.State != contracts.ReleaseStateVerified || release.VerificationEvidenceDigest == nil {
		t.Fatalf("decode verified release %s: value=%#v error=%v", value.AgentID, release, err)
	}
	publishedResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/releases/%s/publish", release.ReleaseID), http.MethodPost, env.ownerToken, "", nil)
	if publishedResult.status != http.StatusOK {
		t.Fatalf("publish release %s status=%d body=%s", value.AgentID, publishedResult.status, publishedResult.body)
	}
	assertNoForbiddenBody(t, publishedResult.body, env.forbidden, "published Card response")
	var published contracts.AgentReleaseResponse
	if err := json.Unmarshal(publishedResult.body, &published); err != nil || published.State != contracts.ReleaseStatePublished || published.PublishedAt == nil {
		t.Fatalf("decode published release %s: value=%#v error=%v", value.AgentID, published, err)
	}
}

func challengeService(t *testing.T, endpoint string) string {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse challenge endpoint: %v", err)
	}
	switch parsed.Hostname() {
	case "runtime-a":
		return "runtime-a"
	case "runtime-b":
		return "runtime-b"
	default:
		t.Fatalf("endpoint %q has no acceptance challenge service", endpoint)
		return ""
	}
}

func writeChallengeProof(t *testing.T, composeFile, service, challengeID, proof string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "docker", "compose", "--file", composeFile, "exec", "-T", service, "sh", "-c", `umask 077; cat > "$NEKIRO_AGENT_CHALLENGE_DIRECTORY/$1"`, "sh", challengeID)
	command.Stdin = strings.NewReader(proof)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("write %s challenge proof: %v output=%s", service, err, output)
	}
}

func removeChallengeProof(t *testing.T, composeFile, service, challengeID string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "docker", "compose", "--file", composeFile, "exec", "-T", service, "sh", "-c", `rm "$NEKIRO_AGENT_CHALLENGE_DIRECTORY/$1"`, "sh", challengeID)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("remove %s challenge proof: %v output=%s", service, err, output)
	}
}

func createWorkspace(t *testing.T, client *http.Client, env acceptanceEnv, workspaceID, token string) {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+"/v3/workspaces", http.MethodPost, token, "application/json", map[string]any{"workspaceId": workspaceID})
	if result.status != http.StatusCreated {
		t.Fatalf("create Workspace %s status=%d body=%s", workspaceID, result.status, result.body)
	}
}

func install(t *testing.T, client *http.Client, env acceptanceEnv, workspaceID, agentID string, permissions []string) {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v3/workspaces/%s/installations", workspaceID), http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": agentID, "versionConstraint": "=1.0.0", "acceptedPermissions": permissions})
	if result.status != http.StatusCreated {
		t.Fatalf("install %s status=%d body=%s", agentID, result.status, result.body)
	}
}

type jsonInvocation struct {
	result contracts.InvocationResult
}

func invokeJSON(t *testing.T, client *http.Client, env acceptanceEnv, agentID, capability string, input map[string]any) jsonInvocation {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": agentID, "capability": capability, "input": input, "stream": false})
	if result.status != http.StatusOK {
		t.Fatalf("JSON invoke %s status=%d body=%s", agentID, result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.credentialForbidden, "JSON invocation response")
	var invocation contracts.InvocationResult
	if err := json.Unmarshal(result.body, &invocation); err != nil {
		t.Fatalf("decode JSON invocation: %v body=%s", err, result.body)
	}
	if invocation.InvocationID == "" || invocation.RootTaskID == "" || invocation.TraceID == "" {
		t.Fatalf("JSON correlation=%#v", invocation)
	}
	return jsonInvocation{result: invocation}
}

func invokeSSE(t *testing.T, client *http.Client, env acceptanceEnv, agentID, capability string, input map[string]any) []contracts.InvocationResultStreamEventV2 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"agentId": agentID, "capability": capability, "input": input, "stream": true})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+env.ownerToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("SSE invoke status=%d body=%s", response.StatusCode, data)
	}
	reader := bufio.NewReader(response.Body)
	var events []contracts.InvocationResultStreamEventV2
	terminalSeen := false
	for {
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && line == "" {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data: ") || !strings.HasSuffix(line, "\n") {
			t.Fatalf("invalid SSE data line=%q", line)
		}
		blank, blankErr := reader.ReadString('\n')
		if blankErr != nil || blank != "\n" {
			t.Fatalf("invalid SSE delimiter=%q err=%v", blank, blankErr)
		}
		eventBody := []byte(strings.TrimSuffix(strings.TrimPrefix(line, "data: "), "\n"))
		assertNoForbiddenBody(t, eventBody, env.credentialForbidden, "SSE invocation response")
		var event contracts.InvocationResultStreamEventV2
		if err := json.Unmarshal(eventBody, &event); err != nil {
			t.Fatalf("decode SSE event: %v", err)
		}
		if terminalSeen {
			t.Fatalf("SSE emitted an event after terminal: %#v", event)
		}
		events = append(events, event)
		if event.Type == contracts.ResultStreamEventCompleted || event.Type == contracts.ResultStreamEventFailed || event.Type == contracts.ResultStreamEventCanceled || event.Type == contracts.ResultStreamEventTimedOut {
			terminalSeen = true
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if !terminalSeen {
		t.Fatalf("SSE ended without terminal event: %#v", events)
	}
	return events
}

type traceRead struct {
	contracts.TraceResponseV4
	raw []byte
}

func readTrace(t *testing.T, client *http.Client, env acceptanceEnv, traceID contracts.TraceID) traceRead {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/traces/%s", acceptanceWorkspace, traceID), http.MethodGet, env.ownerToken, "", nil)
	if result.status != http.StatusOK {
		t.Fatalf("trace read status=%d body=%s", result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, "Trace metadata response")
	var trace contracts.TraceResponseV4
	if err := json.Unmarshal(result.body, &trace); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	return traceRead{TraceResponseV4: trace, raw: result.body}
}

func assertRecord(t *testing.T, client *http.Client, env acceptanceEnv, invocationID, workspaceID, agentID, status, errorCode string) {
	t.Helper()
	detail, _ := readInvocationDetail(t, client, env, invocationID, workspaceID)
	if err := validateInvocationDetail(detail, invocationID, workspaceID, agentID, status, errorCode); err != nil {
		t.Fatal(err)
	}
}

func readInvocationDetail(t *testing.T, client *http.Client, env acceptanceEnv, invocationID, workspaceID string) (contracts.InvocationDetailResponseV4, []byte) {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/invocations/%s", workspaceID, invocationID), http.MethodGet, env.ownerToken, "", nil)
	if result.status != http.StatusOK {
		t.Fatalf("record read status=%d body=%s", result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, "Invocation metadata response")
	var detail contracts.InvocationDetailResponseV4
	if err := json.Unmarshal(result.body, &detail); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	return detail, result.body
}

func validateInvocationDetail(detail contracts.InvocationDetailResponseV4, invocationID, workspaceID, agentID, status, errorCode string) error {
	if detail.Invocation.InvocationID != invocationID || detail.Invocation.WorkspaceID != workspaceID || detail.Invocation.TargetAgentID != agentID || detail.Invocation.Status != status || len(detail.Events) == 0 {
		return fmt.Errorf("record projection=%#v events=%#v", detail.Invocation, detail.Events)
	}
	if errorCode != "" && string(detail.Invocation.ErrorCode) != errorCode {
		return fmt.Errorf("record error=%q want=%q", detail.Invocation.ErrorCode, errorCode)
	}
	terminalCount := 0
	for index, event := range detail.Events {
		if event.Sequence != int64(index) || event.InvocationID != invocationID || event.RootTaskID != detail.Invocation.RootTaskID || event.ParentInvocationID != detail.Invocation.ParentInvocationID || event.TraceID != detail.Invocation.TraceID || event.Caller != detail.Invocation.Caller || event.WorkspaceID != workspaceID || event.TargetAgentID != detail.Invocation.TargetAgentID || event.AgentCardVersion != detail.Invocation.AgentCardVersion || event.Capability != detail.Invocation.Capability {
			return fmt.Errorf("record event[%d]=%#v", index, event)
		}
		switch event.Type {
		case "succeeded", "failed", "canceled", "timed_out":
			terminalCount++
			if index != len(detail.Events)-1 {
				return fmt.Errorf("record terminal event[%d] is not last: %#v", index, event)
			}
			terminalErrorCode := contracts.PlatformErrorCode("")
			if event.Error != nil {
				terminalErrorCode = event.Error.Code
			}
			if string(terminalErrorCode) != errorCode && errorCode != "" {
				return fmt.Errorf("record terminal event error=%q want=%q", terminalErrorCode, errorCode)
			}
		}
	}
	if terminalCount != 1 {
		return fmt.Errorf("record terminal event count=%d want=1 events=%#v", terminalCount, detail.Events)
	}
	if detail.Events[len(detail.Events)-1].Status != status {
		return fmt.Errorf("record terminal status=%q want=%q", detail.Events[len(detail.Events)-1].Status, status)
	}
	return nil
}

func assertTraceRecords(t *testing.T, client *http.Client, env acceptanceEnv, trace traceRead) {
	t.Helper()
	for _, projection := range trace.Invocations {
		detail, _ := readInvocationDetail(t, client, env, projection.InvocationID, acceptanceWorkspace)
		if detail.Invocation.TraceID != trace.TraceID || detail.Invocation.RootTaskID != projection.RootTaskID || detail.Invocation.ParentInvocationID != projection.ParentInvocationID {
			t.Fatalf("restart lineage projection=%#v trace=%#v", detail.Invocation, trace)
		}
		if err := validateInvocationDetail(detail, projection.InvocationID, acceptanceWorkspace, projection.TargetAgentID, projection.Status, string(projection.ErrorCode)); err != nil {
			t.Fatal(err)
		}
	}
}

func assertNoForbiddenBody(t *testing.T, body []byte, forbidden []string, surface string) {
	t.Helper()
	if err := forbiddenBodyError(body, forbidden, surface); err != nil {
		t.Fatal(err)
	}
}

func forbiddenBodyError(body []byte, forbidden []string, surface string) error {
	for _, literal := range forbidden {
		if bytes.Contains(body, []byte(literal)) {
			return fmt.Errorf("forbidden literal %q appeared in %s", literal, surface)
		}
	}
	return routerCredentialLeakError(body, surface)
}

func routerCredentialLeakError(body []byte, surface string) error {
	for _, match := range routerCredentialPattern.FindAllSubmatch(body, -1) {
		segments := bytes.Split(match[2], []byte("."))
		headerJSON, headerErr := base64.RawURLEncoding.Strict().DecodeString(string(segments[0]))
		claimsJSON, claimsErr := base64.RawURLEncoding.Strict().DecodeString(string(segments[1]))
		if headerErr != nil || claimsErr != nil {
			continue
		}
		var header struct {
			Algorithm string `json:"alg"`
			Type      string `json:"typ"`
		}
		var claims struct {
			JWTID string `json:"jti"`
		}
		if json.Unmarshal(headerJSON, &header) == nil && header.Algorithm == contracts.RouterAgentCredentialAlgorithm && header.Type == contracts.RouterAgentCredentialType {
			return fmt.Errorf("encoded Router credential appeared in %s", surface)
		}
		if json.Unmarshal(claimsJSON, &claims) == nil && strings.HasPrefix(claims.JWTID, "rtj_") {
			return fmt.Errorf("encoded Router credential token ID appeared in %s", surface)
		}
	}
	for _, match := range ed25519SignaturePattern.FindAllSubmatch(body, -1) {
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(string(match[2]))
		if err == nil && len(decoded) == ed25519.SignatureSize {
			return fmt.Errorf("ed25519 signature encoding appeared in %s", surface)
		}
	}
	return nil
}

func TestRouterCredentialLeakDetectorRejectsEncodedJWTMaterial(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"nekiro-router+jwt","kid":"acceptance-key"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"jti":"rtj_acceptance_dynamic"}`))
	signature := base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	credential := []byte("Bearer " + header + "." + claims + "." + signature)
	if err := routerCredentialLeakError(credential, "detector fixture"); err == nil {
		t.Fatal("encoded Router credential was not detected")
	}
	if err := routerCredentialLeakError([]byte("signature="+signature), "detector fixture"); err == nil {
		t.Fatal("standalone Ed25519 signature encoding was not detected")
	}
	if err := forbiddenBodyError([]byte("jti=rtj_plaintext"), []string{"rtj_"}, "detector fixture"); err == nil {
		t.Fatal("plaintext Router token ID was not detected")
	}
	if err := routerCredentialLeakError([]byte(`{"status":"succeeded"}`), "clean fixture"); err != nil {
		t.Fatal(err)
	}
	benign := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if err := routerCredentialLeakError([]byte("digest="+benign), "clean fixture"); err != nil {
		t.Fatal(err)
	}
}

func assertFailureMatrix(t *testing.T, client *http.Client, env acceptanceEnv) {
	t.Helper()
	missing := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "not-installed", "capability": "runtime.echo", "input": map[string]any{"fixture": "success", "value": "policy-content-secret"}, "stream": false})
	assertErrorCode(t, missing, contracts.ErrorCodeAgentNotInstalled, env.forbidden)
	capabilityDenied := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "runtime-b", "capability": "runtime.denied", "input": map[string]any{"fixture": "success", "value": "policy-content-secret"}, "stream": false})
	assertErrorCode(t, capabilityDenied, contracts.ErrorCodeCapabilityNotAllowed, env.forbidden)
	protocol := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "runtime-protocol", "capability": "runtime.protocol", "input": map[string]any{"fixture": "protocol", "value": "protocol-content-secret"}, "stream": false})
	assertErrorCode(t, protocol, contracts.ErrorCodeA2AProtocol, env.forbidden)
	agent := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "runtime-b", "capability": "runtime.echo", "input": map[string]any{"fixture": "failure", "value": "agent-content-secret"}, "stream": false})
	assertErrorCode(t, agent, contracts.ErrorCodeAgentExecutionFailed, env.forbidden)
	route := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "runtime-route", "capability": "runtime.route", "input": map[string]any{"fixture": "success", "value": "route-content-secret"}, "stream": false})
	assertErrorCode(t, route, contracts.ErrorCodeAgentUnavailable, env.forbidden)
	timedOut := invokeSSE(t, client, env, "runtime-timeout", "runtime.timeout", map[string]any{"fixture": "hold", "value": "timeout-content-secret"})
	timeoutInvocationID := assertStreamTerminal(t, timedOut, contracts.ResultStreamEventTimedOut, contracts.ErrorCodeTimeout, env.forbidden)
	assertRecord(t, client, env, timeoutInvocationID, acceptanceWorkspace, "runtime-timeout", "timed_out", string(contracts.ErrorCodeTimeout))
	canceledInvocationID := invokeCanceledSSE(t, client, env, "runtime-timeout", "runtime.timeout")
	waitForRecord(t, client, env, canceledInvocationID, "canceled", string(contracts.ErrorCodeCanceled))
	interrupted := invokeSSE(t, client, env, "runtime-interrupted", "runtime.interrupted", map[string]any{"fixture": "interrupted", "value": "interrupted-content-secret"})
	interruptedInvocationID := assertStreamTerminal(t, interrupted, contracts.ResultStreamEventFailed, contracts.ErrorCodeA2AProtocol, env.forbidden)
	assertRecord(t, client, env, interruptedInvocationID, acceptanceWorkspace, "runtime-interrupted", "failed", string(contracts.ErrorCodeA2AProtocol))
	assertDependencyFailure(t, client, env)
}

func assertStreamTerminal(t *testing.T, events []contracts.InvocationResultStreamEventV2, wantType contracts.ResultStreamEventType, wantCode contracts.PlatformErrorCode, forbidden []string) string {
	t.Helper()
	if len(events) == 0 || events[0].Type != contracts.ResultStreamEventAccepted {
		t.Fatalf("stream did not start with accepted: %#v", events)
	}
	terminal := events[len(events)-1]
	if terminal.Type != wantType || terminal.Error == nil || terminal.Error.Code != wantCode {
		t.Fatalf("stream terminal=%#v want type=%q code=%q", terminal, wantType, wantCode)
	}
	terminalCount := 0
	for index, event := range events {
		if event.Sequence != int64(index) || event.InvocationID != terminal.InvocationID || event.RootTaskID != terminal.RootTaskID || event.TraceID != terminal.TraceID {
			t.Fatalf("stream event[%d]=%#v", index, event)
		}
		if event.Type == contracts.ResultStreamEventCompleted || event.Type == contracts.ResultStreamEventFailed || event.Type == contracts.ResultStreamEventCanceled || event.Type == contracts.ResultStreamEventTimedOut {
			terminalCount++
			if index != len(events)-1 {
				t.Fatalf("stream terminal event[%d] is not last: %#v", index, event)
			}
		}
	}
	if terminalCount != 1 {
		t.Fatalf("stream terminal count=%d want=1 events=%#v", terminalCount, events)
	}
	assertNoForbiddenBody(t, terminalBody(terminal), forbidden, "SSE terminal response")
	return terminal.InvocationID
}

func terminalBody(event contracts.InvocationResultStreamEventV2) []byte {
	encoded, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return encoded
}

func assertDependencyFailure(t *testing.T, client *http.Client, env acceptanceEnv) {
	t.Helper()
	stop := exec.CommandContext(t.Context(), "docker", "compose", "--file", env.composeFile, "stop", "control-plane")
	if output, err := stop.CombinedOutput(); err != nil {
		t.Fatalf("stop Control Plane for dependency fixture: %v output=%s", err, output)
	}
	request := contracts.DispatchInvocationRequestV4{
		InvocationID: "dependency-check-invocation", RootTaskID: "dependency-check-task", TraceID: "trc_dependency_check_1",
		Caller: contracts.Caller{Type: "user", ID: "acceptance-owner"}, WorkspaceID: acceptanceWorkspace,
		TargetAgentID: "runtime-b", AgentCardVersion: "1.0.0", Capability: "runtime.echo",
		Input: json.RawMessage(`{"fixture":"success","value":"dependency-raw-secret"}`), Stream: false,
	}
	result, requestErr := doRequestRaw(t.Context(), client, env.routerURL+"/internal/v4/invocations", http.MethodPost, env.routerToken, "application/json", request)
	start := exec.CommandContext(t.Context(), "docker", "compose", "--file", env.composeFile, "start", "control-plane")
	if output, err := start.CombinedOutput(); err != nil {
		t.Fatalf("restart Control Plane after dependency fixture: %v output=%s", err, output)
	}
	waitForReady(t, client, env.controlPlane+"/readyz", http.StatusNoContent)
	if requestErr != nil {
		t.Fatalf("dependency fixture request: %v", requestErr)
	}
	assertErrorCode(t, result, contracts.ErrorCodeDependency, env.forbidden)
}

func invokeCanceledSSE(t *testing.T, client *http.Client, env acceptanceEnv, agentID, capability string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body, err := json.Marshal(map[string]any{"agentId": agentID, "capability": capability, "input": map[string]any{"fixture": "hold", "value": "cancel-content-secret"}, "stream": true})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+env.ownerToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "data: ") {
		response.Body.Close()
		t.Fatalf("cancel stream accepted read err=%v line=%q", err, line)
	}
	blank, err := reader.ReadString('\n')
	if err != nil || blank != "\n" {
		response.Body.Close()
		t.Fatalf("cancel stream delimiter=%q err=%v", blank, err)
	}
	var accepted contracts.InvocationResultStreamEventV2
	eventBody := []byte(strings.TrimSuffix(strings.TrimPrefix(line, "data: "), "\n"))
	assertNoForbiddenBody(t, eventBody, env.forbidden, "SSE cancellation response")
	if err := json.Unmarshal(eventBody, &accepted); err != nil || accepted.Type != contracts.ResultStreamEventAccepted {
		response.Body.Close()
		t.Fatalf("cancel stream accepted=%#v err=%v", accepted, err)
	}
	cancel()
	_ = response.Body.Close()
	return accepted.InvocationID
}

func waitForRecord(t *testing.T, client *http.Client, env acceptanceEnv, invocationID, status, errorCode string) contracts.InvocationDetailResponseV4 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/invocations/%s", acceptanceWorkspace, invocationID), http.MethodGet, env.ownerToken, "", nil)
		if result.status == http.StatusOK {
			var detail contracts.InvocationDetailResponseV4
			if err := json.Unmarshal(result.body, &detail); err == nil && detail.Invocation.Status == status {
				assertNoForbiddenBody(t, result.body, env.forbidden, "Invocation metadata response")
				if err := validateInvocationDetail(detail, invocationID, acceptanceWorkspace, detail.Invocation.TargetAgentID, status, errorCode); err != nil {
					t.Fatal(err)
				}
				return detail
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Invocation %s did not reach %s", invocationID, status)
	return contracts.InvocationDetailResponseV4{}
}

func assertErrorCode(t *testing.T, result httpResult, want contracts.PlatformErrorCode, forbidden ...[]string) {
	t.Helper()
	if result.status == http.StatusOK {
		t.Fatalf("failure unexpectedly succeeded: %s", result.body)
	}
	var value struct {
		Code contracts.PlatformErrorCode `json:"code"`
	}
	if err := json.Unmarshal(result.body, &value); err != nil || value.Code != want {
		t.Fatalf("error status=%d code=%q want=%q body=%s err=%v", result.status, value.Code, want, result.body, err)
	}
	if len(forbidden) == 1 {
		assertNoForbiddenBody(t, result.body, forbidden[0], "failure response")
	}
}

func assertConcurrentCalls(t *testing.T, client *http.Client, env acceptanceEnv) {
	t.Helper()
	const calls = 100
	start := make(chan struct{})
	type outcome struct {
		index  int
		result httpResult
		err    error
	}
	results := make(chan outcome, calls)
	var wait sync.WaitGroup
	wait.Add(calls)
	for index := 0; index < calls; index++ {
		index := index
		go func() {
			defer wait.Done()
			<-start
			result, err := doRequestRaw(t.Context(), client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "runtime-b", "capability": "runtime.echo", "input": map[string]any{"fixture": "success", "value": fmt.Sprintf("concurrent-%03d", index)}, "stream": false})
			results <- outcome{index: index, result: result, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	ids := make([]string, 0, calls)
	rootTasks := make(map[string]struct{}, calls)
	traces := make(map[string]struct{}, calls)
	values := make(map[string]struct{}, calls)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent request: %v", result.err)
		}
		if result.result.status != http.StatusOK {
			t.Fatalf("concurrent status=%d body=%s", result.result.status, result.result.body)
		}
		var value contracts.InvocationResult
		if err := json.Unmarshal(result.result.body, &value); err != nil {
			t.Fatalf("concurrent decode: %v", err)
		}
		expectedValue := fmt.Sprintf("concurrent-%03d", result.index)
		if value.InvocationID == "" || value.RootTaskID == "" || value.TraceID == "" || !bytes.Contains(value.Result, []byte(expectedValue)) {
			t.Fatalf("concurrent response index=%d expected value=%q result=%#v", result.index, expectedValue, value)
		}
		ids = append(ids, value.InvocationID)
		if _, exists := values[expectedValue]; exists {
			t.Fatalf("duplicate concurrent input value=%q", expectedValue)
		}
		values[expectedValue] = struct{}{}
		if _, exists := rootTasks[string(value.RootTaskID)]; exists {
			t.Fatalf("duplicate concurrent root task=%s", value.RootTaskID)
		}
		rootTasks[string(value.RootTaskID)] = struct{}{}
		if _, exists := traces[string(value.TraceID)]; exists {
			t.Fatalf("duplicate concurrent trace=%s", value.TraceID)
		}
		traces[string(value.TraceID)] = struct{}{}
	}
	sort.Strings(ids)
	for index := 1; index < len(ids); index++ {
		if ids[index] == ids[index-1] {
			t.Fatalf("duplicate concurrent Invocation ID=%s", ids[index])
		}
	}
	if len(ids) != calls {
		t.Fatalf("concurrent accepted=%d want=%d", len(ids), calls)
	}
	if len(values) != calls || len(rootTasks) != calls || len(traces) != calls {
		t.Fatalf("concurrent correlation values=%d roots=%d traces=%d want=%d", len(values), len(rootTasks), len(traces), calls)
	}
	readErrors := make(chan error, calls)
	for _, invocationID := range ids {
		invocationID := invocationID
		go func() {
			result, err := doRequestRaw(t.Context(), client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/invocations/%s", acceptanceWorkspace, invocationID), http.MethodGet, env.ownerToken, "", nil)
			if err != nil {
				readErrors <- err
				return
			}
			if result.status != http.StatusOK {
				readErrors <- fmt.Errorf("Invocation %s read status=%d body=%s", invocationID, result.status, result.body)
				return
			}
			if err := forbiddenBodyError(result.body, env.forbidden, "concurrent Invocation metadata response"); err != nil {
				readErrors <- err
				return
			}
			var detail contracts.InvocationDetailResponseV4
			if err := json.Unmarshal(result.body, &detail); err != nil {
				readErrors <- fmt.Errorf("decode concurrent Invocation %s: %w", invocationID, err)
				return
			}
			readErrors <- validateInvocationDetail(detail, invocationID, acceptanceWorkspace, "runtime-b", "succeeded", "")
		}()
	}
	for index := 0; index < calls; index++ {
		if err := <-readErrors; err != nil {
			t.Fatal(err)
		}
	}
	assertConcurrentLedger(t, env, ids)
}

func assertConcurrentLedger(t *testing.T, env acceptanceEnv, invocationIDs []string) {
	t.Helper()
	database, err := sql.Open("pgx", env.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	placeholders := make([]string, len(invocationIDs))
	args := make([]any, 0, len(invocationIDs)+1)
	args = append(args, acceptanceWorkspace)
	for index, invocationID := range invocationIDs {
		placeholders[index] = fmt.Sprintf("$%d", index+2)
		args = append(args, invocationID)
	}
	query := `SELECT count(*), count(*) FILTER (WHERE status IN ('succeeded','failed','canceled','timed_out')) FROM ledger.invocations WHERE workspace_id = $1 AND invocation_id IN (` + strings.Join(placeholders, ",") + `)`
	var projectionCount, terminalProjectionCount int
	if err := database.QueryRowContext(ctx, query, args...).Scan(&projectionCount, &terminalProjectionCount); err != nil {
		t.Fatal(err)
	}
	if projectionCount != len(invocationIDs) || terminalProjectionCount != len(invocationIDs) {
		t.Fatalf("concurrent Ledger projections=%d terminal=%d want=%d", projectionCount, terminalProjectionCount, len(invocationIDs))
	}
	eventQuery := `SELECT count(*) FROM ledger.invocation_events WHERE workspace_id = $1 AND invocation_id IN (` + strings.Join(placeholders, ",") + `) AND event_type IN ('succeeded','failed','canceled','timed_out')`
	var terminalEventCount int
	if err := database.QueryRowContext(ctx, eventQuery, args...).Scan(&terminalEventCount); err != nil {
		t.Fatal(err)
	}
	if terminalEventCount != len(invocationIDs) {
		t.Fatalf("concurrent terminal events=%d want=%d", terminalEventCount, len(invocationIDs))
	}
}

func assertStorageAndLogsAreMetadataOnly(t *testing.T, env acceptanceEnv) {
	t.Helper()
	database, err := sql.Open("pgx", env.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := database.QueryContext(ctx, `SELECT event_id, event_type, status, invocation_id, root_task_id, COALESCE(parent_invocation_id, ''), trace_id, caller_id, workspace_id, target_agent_id, agent_card_version, capability, COALESCE(error_code, '') FROM ledger.invocation_events`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fields [13]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := rows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		serialized := strings.Join(fields[:], "|")
		assertNoForbiddenBody(t, []byte(serialized), env.forbidden, "Ledger row")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	projectionRows, err := database.QueryContext(ctx, `SELECT invocation_id, root_task_id, COALESCE(parent_invocation_id, ''), trace_id, caller_type, caller_id, workspace_id, target_agent_id, agent_card_version, capability, status, COALESCE(error_code, '') FROM ledger.invocations`)
	if err != nil {
		t.Fatal(err)
	}
	defer projectionRows.Close()
	for projectionRows.Next() {
		var fields [12]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := projectionRows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		serialized := strings.Join(fields[:], "|")
		assertNoForbiddenBody(t, []byte(serialized), env.forbidden, "Ledger projection")
	}
	if err := projectionRows.Err(); err != nil {
		t.Fatal(err)
	}
	cardRows, err := database.QueryContext(ctx, `SELECT agent_id, version, COALESCE(card_name, ''), COALESCE(card_description, ''), card::text FROM catalog.agent_versions`)
	if err != nil {
		t.Fatal(err)
	}
	for cardRows.Next() {
		var fields [5]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := cardRows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		assertNoForbiddenBody(t, []byte(strings.Join(fields[:], "|")), env.forbidden, "persisted Agent Card")
	}
	if err := cardRows.Err(); err != nil {
		t.Fatal(err)
	}
	cardRows.Close()
	bindingRows, err := database.QueryContext(ctx, `SELECT binding_id, provider_id, agent_id, agent_card_version, endpoint, endpoint_origin, endpoint_path, verification_method, verification_status FROM catalog.endpoint_bindings`)
	if err != nil {
		t.Fatal(err)
	}
	for bindingRows.Next() {
		var fields [9]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := bindingRows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		assertNoForbiddenBody(t, []byte(strings.Join(fields[:], "|")), env.forbidden, "persisted endpoint binding")
	}
	if err := bindingRows.Err(); err != nil {
		t.Fatal(err)
	}
	bindingRows.Close()
	releaseRows, err := database.QueryContext(ctx, `SELECT release_id, provider_id, agent_id, agent_card_version, endpoint_origin, endpoint_path, verification_method, state FROM catalog.agent_releases`)
	if err != nil {
		t.Fatal(err)
	}
	for releaseRows.Next() {
		var fields [8]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := releaseRows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		assertNoForbiddenBody(t, []byte(strings.Join(fields[:], "|")), env.forbidden, "persisted Agent Release")
	}
	if err := releaseRows.Err(); err != nil {
		t.Fatal(err)
	}
	releaseRows.Close()
	workspaceRows, err := database.QueryContext(ctx, `SELECT workspace_id, owner_id FROM workspace.workspaces`)
	if err != nil {
		t.Fatal(err)
	}
	for workspaceRows.Next() {
		var workspaceID, ownerID string
		if err := workspaceRows.Scan(&workspaceID, &ownerID); err != nil {
			t.Fatal(err)
		}
		assertNoForbiddenBody(t, []byte(strings.Join([]string{workspaceID, ownerID}, "|")), env.forbidden, "persisted Workspace")
	}
	if err := workspaceRows.Err(); err != nil {
		t.Fatal(err)
	}
	workspaceRows.Close()
	installationRows, err := database.QueryContext(ctx, `SELECT installation_id, workspace_id, agent_id, version_constraint, installed_version, COALESCE(installed_release_id, ''), array_to_string(accepted_permissions, '|'), status FROM workspace.installations`)
	if err != nil {
		t.Fatal(err)
	}
	for installationRows.Next() {
		var fields [8]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := installationRows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		assertNoForbiddenBody(t, []byte(strings.Join(fields[:], "|")), env.forbidden, "persisted Installation")
	}
	if err := installationRows.Err(); err != nil {
		t.Fatal(err)
	}
	installationRows.Close()
	logs := exec.CommandContext(ctx, "docker", "compose", "--file", env.composeFile, "logs", "--no-color")
	output, err := logs.Output()
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenBody(t, output, env.forbidden, "process logs")
}

func restartRouter(t *testing.T, composeFile string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "docker", "compose", "--file", composeFile, "restart", "a2a-router")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("restart Router: %v output=%s", err, output)
	}
}

func waitForReady(t *testing.T, client *http.Client, endpoint string, wantStatus int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == wantStatus {
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("endpoint %s did not become ready", endpoint)
}

func doRequest(t *testing.T, client *http.Client, endpoint, method, token, contentType string, payload any) httpResult {
	t.Helper()
	result, err := doRequestRaw(t.Context(), client, endpoint, method, token, contentType, payload)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func doRequestRaw(ctx context.Context, client *http.Client, endpoint, method, token, contentType string, payload any) (httpResult, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return httpResult{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return httpResult{}, err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodPost && strings.Contains(endpoint, "/invocations") {
		request.Header.Set("Accept", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return httpResult{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return httpResult{}, err
	}
	return httpResult{status: response.StatusCode, header: response.Header, body: data}, nil
}
