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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	acceptanceWorkspace  = "workspace-acceptance"
	acceptanceProviderID = "provider-acceptance"
)

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
	composeProject      string
	challengeTTL        time.Duration
	credentialIssuer    string
	credentialKeyID     string
	credentialPrivate   string
	releases            map[string]contracts.AgentReleaseResponse
	credentialForbidden []string
	forbidden           []string
}

func (env *acceptanceEnv) forbid(values ...string) {
	for _, value := range values {
		if value != "" {
			env.forbidden = append(env.forbidden, value)
		}
	}
}

func (env *acceptanceEnv) forbidCredential(values ...string) {
	for _, value := range values {
		if value != "" {
			env.credentialForbidden = append(env.credentialForbidden, value)
			env.forbidden = append(env.forbidden, value)
		}
	}
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
		"NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL",
	} {
		env.credentialForbidden = append(env.credentialForbidden, requiredEnv(t, name))
	}
	env.credentialForbidden = append(env.credentialForbidden, env.credentialKeyID, env.credentialPrivate)
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
	runtimeA := acceptanceCard("runtime-a", "Runtime A", "http://runtime-a:8091", "runtime.cross", nil, false)
	runtimeB := acceptanceCard("runtime-b", "Runtime B", "http://runtime-b:8092", "runtime.echo", []string{"text.read"}, true)
	runtimeProtocol := acceptanceCard("runtime-protocol", "Runtime Protocol Fixture", "http://runtime-b:8092", "runtime.protocol", nil, false)
	runtimeRoute := acceptanceCard("runtime-route", "Runtime Route Fixture", "http://runtime-b:8092/unavailable", "runtime.route", nil, false)
	runtimeTimeout := acceptanceCardWithTimeout("runtime-timeout", "Runtime Timeout Fixture", "http://runtime-b:8092", "runtime.timeout", nil, true, 50)
	runtimeInterrupted := acceptanceCard("runtime-interrupted", "Runtime Interrupted Fixture", "http://runtime-b:8092", "runtime.interrupted", nil, true)
	runtimeLifecycle := acceptanceCard("runtime-lifecycle", "Runtime Lifecycle Fixture", "http://runtime-b:8092", "runtime.echo", nil, false)
	registerAndPublish(t, client, &env, runtimeA)
	runtimeBRelease := registerAndPublish(t, client, &env, runtimeB)
	registerAndPublish(t, client, &env, runtimeProtocol)
	registerAndPublish(t, client, &env, runtimeRoute)
	registerAndPublish(t, client, &env, runtimeTimeout)
	registerAndPublish(t, client, &env, runtimeInterrupted)
	runtimeLifecycleRelease := registerAndPublish(t, client, &env, runtimeLifecycle)
	assertVerificationFailureMatrix(t, client, &env)

	discovery := doRequest(t, client, env.controlPlane+"/v3/agents?capability=runtime.echo", http.MethodGet, env.userToken, "", nil)
	if discovery.status != http.StatusOK || !bytes.Contains(discovery.body, []byte(`"agentId":"runtime-b"`)) {
		t.Fatalf("discovery status=%d body=%s", discovery.status, discovery.body)
	}
	assertNoForbiddenBody(t, discovery.body, env.forbidden, "Discovery Card response")
	createWorkspace(t, client, env, acceptanceWorkspace, env.ownerToken)
	assertDirectAgentRequestIsRejected(t, client, &env, runtimeBRelease)
	install(t, client, env, acceptanceWorkspace, "runtime-a", []string{})
	runtimeBInstallation := install(t, client, env, acceptanceWorkspace, "runtime-b", []string{"text.read"})
	install(t, client, env, acceptanceWorkspace, "runtime-protocol", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-route", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-timeout", []string{})
	install(t, client, env, acceptanceWorkspace, "runtime-interrupted", []string{})
	runtimeLifecycleInstallation := install(t, client, env, acceptanceWorkspace, "runtime-lifecycle", []string{})
	assertUnpublishedReleaseRejected(t, client, &env)
	assertRouterCredentialFailureMatrix(t, client, &env, runtimeBRelease)

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
	assertTraceRecords(t, client, env, trace)
	assertQueryableRelease(t, client, env, env.releases["runtime-a"])
	assertQueryableRelease(t, client, env, runtimeBRelease)
	assertInstallationAndReleaseGates(t, client, &env, runtimeBInstallation, runtimeLifecycleInstallation, runtimeLifecycleRelease)

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

	restartRouter(t, env)
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
	ttlSeconds, err := strconv.ParseInt(requiredEnv(t, "NEKIRO_ENDPOINT_CHALLENGE_TTL_SECONDS"), 10, 64)
	if err != nil || ttlSeconds < 2 || ttlSeconds > 15 {
		t.Fatalf("NEKIRO_ENDPOINT_CHALLENGE_TTL_SECONDS must be an acceptance value from 2 through 15 seconds")
	}
	return acceptanceEnv{
		controlPlane:      requiredEnv(t, "NEKIRO_E2E_CONTROL_PLANE_URL"),
		routerURL:         requiredEnv(t, "NEKIRO_E2E_ROUTER_URL"),
		routerToken:       requiredEnv(t, "NEKIRO_E2E_ROUTER_TOKEN"),
		ownerToken:        requiredEnv(t, "NEKIRO_E2E_OWNER_TOKEN"),
		userToken:         requiredEnv(t, "NEKIRO_E2E_USER_TOKEN"),
		otherToken:        requiredEnv(t, "NEKIRO_E2E_OTHER_TOKEN"),
		databaseURL:       requiredEnv(t, "NEKIRO_E2E_DATABASE_URL"),
		composeFile:       composeFile,
		composeProject:    requiredEnv(t, "NEKIRO_E2E_COMPOSE_PROJECT"),
		challengeTTL:      time.Duration(ttlSeconds) * time.Second,
		credentialIssuer:  requiredEnv(t, "NEKIRO_ROUTER_AGENT_CREDENTIAL_ISSUER"),
		credentialKeyID:   requiredEnv(t, "NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID"),
		credentialPrivate: requiredEnv(t, "NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL"),
		releases:          make(map[string]contracts.AgentReleaseResponse),
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

func composeCommand(ctx context.Context, env acceptanceEnv, args ...string) *exec.Cmd {
	base := []string{"compose", "--project-name", env.composeProject, "--file", env.composeFile}
	return exec.CommandContext(ctx, "docker", append(base, args...)...)
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

func assertDirectAgentRequestIsRejected(t *testing.T, client *http.Client, env *acceptanceEnv, release contracts.AgentReleaseResponse) {
	t.Helper()
	const content = "direct-agent-must-not-execute"
	const invocationID = "inv_direct_unauthenticated"
	env.forbid(content)
	contextHeaders := contracts.RouterAgentContextHeadersV1(contracts.RouterInvocationCredentialContextV1{
		WorkspaceID: acceptanceWorkspace, AgentID: release.AgentID, AgentVersion: release.AgentCardVersion,
		ReleaseID: release.ReleaseID, CardDigest: release.CardDigest, Capability: "runtime.echo",
		InvocationID: invocationID, RootTaskID: "task_direct_unauthenticated", TraceID: "trace-direct-unauthenticated",
	})
	output, err := agentRequestInContainer(t, *env, "", contextHeaders, "direct-unauthenticated", content)
	assertNoForbiddenBody(t, output, env.forbidden, "direct Agent rejection")
	if err != nil || !bytes.Contains(output, []byte("401 Unauthorized")) || !bytes.Contains(output, []byte(`"code":"UNAUTHENTICATED"`)) {
		t.Fatalf("direct unauthenticated Agent request was not rejected exactly: err=%v", err)
	}
	assertInvocationAbsent(t, client, *env, invocationID)
}

func registerCard(t *testing.T, client *http.Client, env acceptanceEnv, card []byte) contracts.AgentCard {
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
	return value
}

func createEndpointBinding(t *testing.T, client *http.Client, env acceptanceEnv, card contracts.AgentCard) contracts.EndpointBindingResponse {
	t.Helper()
	bindingResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/agents/%s/endpoint-bindings", acceptanceProviderID, card.AgentID), http.MethodPost, env.ownerToken, "application/json", contracts.CreateEndpointBindingRequest{Endpoint: card.Protocol.Endpoint, Method: "http_well_known", Version: card.Version})
	if bindingResult.status != http.StatusCreated {
		t.Fatalf("create binding %s status=%d body=%s", card.AgentID, bindingResult.status, bindingResult.body)
	}
	assertNoForbiddenBody(t, bindingResult.body, env.forbidden, "endpoint binding response")
	var binding contracts.EndpointBindingResponse
	if err := json.Unmarshal(bindingResult.body, &binding); err != nil || binding.BindingID == "" || binding.VerificationStatus != "pending" {
		t.Fatalf("decode pending binding %s: value=%#v error=%v", card.AgentID, binding, err)
	}
	return binding
}

func createVerificationChallenge(t *testing.T, client *http.Client, env *acceptanceEnv, bindingID string) contracts.VerificationChallengeResponse {
	t.Helper()
	challengeResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/endpoint-bindings/%s/challenges", acceptanceProviderID, bindingID), http.MethodPost, env.ownerToken, "", nil)
	if challengeResult.status != http.StatusCreated {
		t.Fatalf("create challenge for binding %s status=%d", bindingID, challengeResult.status)
	}
	// The one-time authenticated issuance response is the only contract surface
	// allowed to contain the raw proof. Add it to the forbidden set immediately
	// after decoding so every later response, store row, and log is scanned.
	assertNoForbiddenBody(t, challengeResult.body, env.forbidden, "verification challenge response")
	var challenge contracts.VerificationChallengeResponse
	if err := json.Unmarshal(challengeResult.body, &challenge); err != nil || challenge.ChallengeID == "" || challenge.Proof == "" {
		t.Fatalf("decode challenge for binding %s: error=%v", bindingID, err)
	}
	env.forbid(challenge.Proof)
	return challenge
}

func completeVerificationChallenge(t *testing.T, client *http.Client, env acceptanceEnv, bindingID, challengeID string) httpResult {
	t.Helper()
	return doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/endpoint-bindings/%s/challenges/%s/complete", acceptanceProviderID, bindingID, challengeID), http.MethodPost, env.ownerToken, "", nil)
}

func completeVerificationWithProof(t *testing.T, client *http.Client, env acceptanceEnv, card contracts.AgentCard, binding contracts.EndpointBindingResponse, challenge contracts.VerificationChallengeResponse, proof string) contracts.EndpointBindingResponse {
	t.Helper()
	service := challengeService(t, card.Protocol.Endpoint)
	writeChallengeProof(t, env, service, challenge.ChallengeID, proof)
	defer removeChallengeProof(t, env, service, challenge.ChallengeID)
	completed := completeVerificationChallenge(t, client, env, binding.BindingID, challenge.ChallengeID)
	assertNoForbiddenBody(t, completed.body, env.forbidden, "verified binding response")
	if completed.status != http.StatusOK {
		t.Fatalf("complete challenge %s status=%d", card.AgentID, completed.status)
	}
	var verified contracts.EndpointBindingResponse
	if err := json.Unmarshal(completed.body, &verified); err != nil || verified.VerificationStatus != "verified" || verified.VerificationEvidenceDigest == nil {
		t.Fatalf("decode verified binding %s: value=%#v error=%v", card.AgentID, verified, err)
	}
	return verified
}

func createRelease(t *testing.T, client *http.Client, env acceptanceEnv, card contracts.AgentCard, bindingID string) contracts.AgentReleaseResponse {
	t.Helper()
	releaseResult := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/agents/%s/releases", acceptanceProviderID, card.AgentID), http.MethodPost, env.ownerToken, "application/json", contracts.CreateAgentReleaseRequest{Version: card.Version, EndpointBindingID: bindingID})
	if releaseResult.status != http.StatusCreated {
		t.Fatalf("create release %s status=%d body=%s", card.AgentID, releaseResult.status, releaseResult.body)
	}
	assertNoForbiddenBody(t, releaseResult.body, env.forbidden, "agent release response")
	var release contracts.AgentReleaseResponse
	if err := json.Unmarshal(releaseResult.body, &release); err != nil || release.ReleaseID == "" {
		t.Fatalf("decode release %s: value=%#v error=%v", card.AgentID, release, err)
	}
	return release
}

func transitionRelease(t *testing.T, client *http.Client, env acceptanceEnv, releaseID, action, wantState string) contracts.AgentReleaseResponse {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/releases/%s/%s", releaseID, action), http.MethodPost, env.ownerToken, "", nil)
	if result.status != http.StatusOK {
		t.Fatalf("%s release %s status=%d body=%s", action, releaseID, result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, action+" Release response")
	var release contracts.AgentReleaseResponse
	if err := json.Unmarshal(result.body, &release); err != nil || release.ReleaseID != releaseID || release.State != wantState {
		t.Fatalf("decode %s Release %s: value=%#v error=%v", action, releaseID, release, err)
	}
	return release
}

func registerAndPublish(t *testing.T, client *http.Client, env *acceptanceEnv, cardJSON []byte) contracts.AgentReleaseResponse {
	t.Helper()
	card := registerCard(t, client, *env, cardJSON)
	binding := createEndpointBinding(t, client, *env, card)
	challenge := createVerificationChallenge(t, client, env, binding.BindingID)
	completeVerificationWithProof(t, client, *env, card, binding, challenge, challenge.Proof)
	release := createRelease(t, client, *env, card, binding.BindingID)
	if release.State != contracts.ReleaseStateVerified || release.VerificationEvidenceDigest == nil {
		t.Fatalf("verified Release %s = %#v", card.AgentID, release)
	}
	published := transitionRelease(t, client, *env, release.ReleaseID, "publish", contracts.ReleaseStatePublished)
	if published.PublishedAt == nil || published.VerificationMethod != "http_well_known" || published.VerificationEvidenceDigest == nil {
		t.Fatalf("published Release %s = %#v", card.AgentID, published)
	}
	env.releases[card.AgentID] = published
	return published
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

func writeChallengeProof(t *testing.T, env acceptanceEnv, service, challengeID, proof string) {
	t.Helper()
	command := composeCommand(t.Context(), env, "exec", "-T", service, "sh", "-c", `umask 077; cat > "$NEKIRO_AGENT_CHALLENGE_DIRECTORY/$1"`, "sh", challengeID)
	command.Stdin = strings.NewReader(proof)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("write %s challenge proof: %v output=%s", service, err, output)
	}
}

func removeChallengeProof(t *testing.T, env acceptanceEnv, service, challengeID string) {
	t.Helper()
	command := composeCommand(t.Context(), env, "exec", "-T", service, "sh", "-c", `rm "$NEKIRO_AGENT_CHALLENGE_DIRECTORY/$1"`, "sh", challengeID)
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

func install(t *testing.T, client *http.Client, env acceptanceEnv, workspaceID, agentID string, permissions []string) contracts.Installation {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v3/workspaces/%s/installations", workspaceID), http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": agentID, "versionConstraint": "=1.0.0", "acceptedPermissions": permissions})
	if result.status != http.StatusCreated {
		t.Fatalf("install %s status=%d body=%s", agentID, result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, "Installation response")
	var installation contracts.Installation
	if err := json.Unmarshal(result.body, &installation); err != nil || installation.InstallationID == "" || installation.Status != "enabled" {
		t.Fatalf("decode Installation %s: value=%#v error=%v", agentID, installation, err)
	}
	if release, exists := env.releases[agentID]; exists && (installation.InstalledReleaseID != release.ReleaseID || installation.InstalledVersion != release.AgentCardVersion) {
		t.Fatalf("Installation %s Release pin=%#v want=%#v", agentID, installation, release)
	}
	return installation
}

func updateInstallationStatus(t *testing.T, client *http.Client, env acceptanceEnv, installation contracts.Installation, wantStatus string) contracts.Installation {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v3/workspaces/%s/installations/%s", installation.WorkspaceID, installation.InstallationID), http.MethodPatch, env.ownerToken, "application/json", contracts.UpdateInstallationRequest{Status: wantStatus})
	if result.status != http.StatusOK {
		t.Fatalf("set Installation %s to %s status=%d body=%s", installation.InstallationID, wantStatus, result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, "Installation status response")
	var updated contracts.Installation
	if err := json.Unmarshal(result.body, &updated); err != nil || updated.Status != wantStatus || updated.InstallationID != installation.InstallationID || updated.InstalledReleaseID != installation.InstalledReleaseID {
		t.Fatalf("decode %s Installation: value=%#v error=%v", wantStatus, updated, err)
	}
	return updated
}

func assertTrustedPublicationError(t *testing.T, result httpResult, wantStatus int, wantCode contracts.TrustedPublicationErrorCode, forbidden []string) contracts.TrustedPublicationError {
	t.Helper()
	assertNoForbiddenBody(t, result.body, forbidden, "trusted publication error")
	if result.status != wantStatus {
		t.Fatalf("trusted publication status=%d want=%d", result.status, wantStatus)
	}
	var failure contracts.TrustedPublicationError
	if err := json.Unmarshal(result.body, &failure); err != nil || failure.Code != wantCode || failure.TraceID == "" {
		t.Fatalf("trusted publication error=%#v want=%q body=%s err=%v", failure, wantCode, result.body, err)
	}
	if len(result.header.Values("x-nek-trace-id")) != 1 || result.header.Get("x-nek-trace-id") != string(failure.TraceID) {
		t.Fatalf("trusted publication Trace mismatch header=%q body=%#v", result.header.Values("x-nek-trace-id"), failure)
	}
	return failure
}

func readEndpointBinding(t *testing.T, client *http.Client, env acceptanceEnv, bindingID string) contracts.EndpointBindingResponse {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/providers/%s/endpoint-bindings/%s", acceptanceProviderID, bindingID), http.MethodGet, env.ownerToken, "", nil)
	if result.status != http.StatusOK {
		t.Fatalf("read Endpoint Binding %s status=%d body=%s", bindingID, result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, "Endpoint Binding read response")
	var binding contracts.EndpointBindingResponse
	if err := json.Unmarshal(result.body, &binding); err != nil || binding.BindingID != bindingID {
		t.Fatalf("decode Endpoint Binding %s: value=%#v error=%v", bindingID, binding, err)
	}
	return binding
}

func assertVerificationFailureMatrix(t *testing.T, client *http.Client, env *acceptanceEnv) {
	t.Helper()

	recoveryCard := registerCard(t, client, *env, acceptanceCard("runtime-verification-recovery", "Verification Recovery Fixture", "http://runtime-b:8092", "runtime.echo", nil, false))
	recoveryBinding := createEndpointBinding(t, client, *env, recoveryCard)
	recoveryChallenge := createVerificationChallenge(t, client, env, recoveryBinding.BindingID)
	wrongProof := strings.Repeat("0", 64)
	if wrongProof == recoveryChallenge.Proof {
		wrongProof = strings.Repeat("1", 64)
	}
	env.forbid(wrongProof)
	wrongProofResult := func() httpResult {
		service := challengeService(t, recoveryCard.Protocol.Endpoint)
		writeChallengeProof(t, *env, service, recoveryChallenge.ChallengeID, wrongProof)
		defer removeChallengeProof(t, *env, service, recoveryChallenge.ChallengeID)
		return completeVerificationChallenge(t, client, *env, recoveryBinding.BindingID, recoveryChallenge.ChallengeID)
	}()
	assertTrustedPublicationError(t, wrongProofResult, http.StatusBadRequest, contracts.TrustedErrorWrongProof, env.forbidden)
	failedBinding := readEndpointBinding(t, client, *env, recoveryBinding.BindingID)
	if failedBinding.VerificationStatus != "failed" || failedBinding.VerificationFailureCode == nil || *failedBinding.VerificationFailureCode != "wrong_proof" || failedBinding.VerificationEvidenceDigest != nil {
		t.Fatalf("wrong-proof Binding=%#v", failedBinding)
	}
	reusedResult := completeVerificationChallenge(t, client, *env, recoveryBinding.BindingID, recoveryChallenge.ChallengeID)
	assertTrustedPublicationError(t, reusedResult, http.StatusConflict, contracts.TrustedErrorChallengeReused, env.forbidden)
	recoveryChallenge = createVerificationChallenge(t, client, env, recoveryBinding.BindingID)
	recovered := completeVerificationWithProof(t, client, *env, recoveryCard, recoveryBinding, recoveryChallenge, recoveryChallenge.Proof)
	if recovered.VerificationFailureCode != nil {
		t.Fatalf("recovered Binding retained failure=%#v", recovered)
	}

	expiredCard := registerCard(t, client, *env, acceptanceCard("runtime-verification-expired", "Verification Expiry Fixture", "http://runtime-b:8092", "runtime.echo", nil, false))
	expiredBinding := createEndpointBinding(t, client, *env, expiredCard)
	expiredChallenge := createVerificationChallenge(t, client, env, expiredBinding.BindingID)
	wait := time.Until(expiredChallenge.ExpiresAt) + 250*time.Millisecond
	if wait <= 0 || wait > env.challengeTTL+2*time.Second {
		t.Fatalf("challenge expiry wait=%s configured TTL=%s", wait, env.challengeTTL)
	}
	timer := time.NewTimer(wait)
	select {
	case <-timer.C:
	case <-t.Context().Done():
		timer.Stop()
		t.Fatal(t.Context().Err())
	}
	expiredResult := completeVerificationChallenge(t, client, *env, expiredBinding.BindingID, expiredChallenge.ChallengeID)
	assertTrustedPublicationError(t, expiredResult, http.StatusConflict, contracts.TrustedErrorChallengeExpired, env.forbidden)
	expiredState := readEndpointBinding(t, client, *env, expiredBinding.BindingID)
	if expiredState.VerificationStatus != "failed" || expiredState.VerificationFailureCode == nil || *expiredState.VerificationFailureCode != "challenge_expired" {
		t.Fatalf("expired Binding=%#v", expiredState)
	}

	disallowedCard := registerCard(t, client, *env, acceptanceCard("runtime-verification-disallowed", "Verification Disallowed Fixture", "http://postgres:5432", "runtime.echo", nil, false))
	disallowedBinding := createEndpointBinding(t, client, *env, disallowedCard)
	disallowedChallenge := createVerificationChallenge(t, client, env, disallowedBinding.BindingID)
	disallowedResult := completeVerificationChallenge(t, client, *env, disallowedBinding.BindingID, disallowedChallenge.ChallengeID)
	assertTrustedPublicationError(t, disallowedResult, http.StatusForbidden, contracts.TrustedErrorDisallowedNetwork, env.forbidden)
	disallowedState := readEndpointBinding(t, client, *env, disallowedBinding.BindingID)
	if disallowedState.VerificationStatus != "failed" || disallowedState.VerificationFailureCode == nil || *disallowedState.VerificationFailureCode != "disallowed_network" {
		t.Fatalf("disallowed Binding=%#v", disallowedState)
	}

	unavailableCard := registerCard(t, client, *env, acceptanceCard("runtime-verification-unavailable", "Verification Unavailable Fixture", "http://runtime-b:65535", "runtime.echo", nil, false))
	unavailableBinding := createEndpointBinding(t, client, *env, unavailableCard)
	unavailableChallenge := createVerificationChallenge(t, client, env, unavailableBinding.BindingID)
	unavailableResult := completeVerificationChallenge(t, client, *env, unavailableBinding.BindingID, unavailableChallenge.ChallengeID)
	assertTrustedPublicationError(t, unavailableResult, http.StatusServiceUnavailable, contracts.TrustedErrorEndpointUnavailable, env.forbidden)
	unavailableState := readEndpointBinding(t, client, *env, unavailableBinding.BindingID)
	if unavailableState.VerificationStatus != "failed" || unavailableState.VerificationFailureCode == nil || *unavailableState.VerificationFailureCode != "endpoint_unavailable" {
		t.Fatalf("unavailable Binding=%#v", unavailableState)
	}
}

func assertUnpublishedReleaseRejected(t *testing.T, client *http.Client, env *acceptanceEnv) {
	t.Helper()
	card := registerCard(t, client, *env, acceptanceCard("runtime-unpublished", "Unpublished Release Fixture", "http://runtime-b:8092", "runtime.echo", nil, false))
	binding := createEndpointBinding(t, client, *env, card)
	release := createRelease(t, client, *env, card, binding.BindingID)
	if release.State != contracts.ReleaseStatePendingVerification || release.VerificationEvidenceDigest != nil || release.PublishedAt != nil {
		t.Fatalf("pending Release=%#v", release)
	}
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v3/workspaces/%s/installations", acceptanceWorkspace), http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": card.AgentID, "versionConstraint": "=1.0.0", "acceptedPermissions": []string{}})
	if result.status != http.StatusForbidden {
		t.Fatalf("unpublished Release install status=%d body=%s", result.status, result.body)
	}
	value := assertErrorCode(t, result, contracts.ErrorCodeAgentReleaseUnpublished, env.forbidden)
	if value.InvocationID != "" || value.RootTaskID != "" {
		t.Fatalf("Installation error contains Invocation correlation: %#v", value)
	}
}

func assertInstallationAndReleaseGates(t *testing.T, client *http.Client, env *acceptanceEnv, runtimeBInstallation, lifecycleInstallation contracts.Installation, lifecycleRelease contracts.AgentReleaseResponse) {
	t.Helper()
	const disabledContent = "disabled-installation-must-not-execute"
	const suspendedContent = "suspended-release-must-not-execute"
	const revokedContent = "revoked-release-must-not-execute"
	env.forbid(disabledContent, suspendedContent, revokedContent)

	disabled := updateInstallationStatus(t, client, *env, runtimeBInstallation, "disabled")
	disabledResult := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": "runtime-b", "capability": "runtime.echo", "input": map[string]any{"fixture": "success", "value": disabledContent}, "stream": false})
	if disabledResult.status != http.StatusConflict {
		t.Fatalf("disabled Installation invocation status=%d body=%s", disabledResult.status, disabledResult.body)
	}
	assertPreInvocationError(t, disabledResult, contracts.ErrorCodeInstallationDisabled, env.forbidden)
	updateInstallationStatus(t, client, *env, disabled, "enabled")

	if lifecycleInstallation.InstalledReleaseID != lifecycleRelease.ReleaseID {
		t.Fatalf("lifecycle Installation=%#v Release=%#v", lifecycleInstallation, lifecycleRelease)
	}
	suspended := transitionRelease(t, client, *env, lifecycleRelease.ReleaseID, "suspend", contracts.ReleaseStateSuspended)
	if suspended.SuspendedAt == nil || suspended.RevokedAt != nil {
		t.Fatalf("suspended Release=%#v", suspended)
	}
	suspendedResult := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": lifecycleRelease.AgentID, "capability": "runtime.echo", "input": map[string]any{"fixture": "success", "value": suspendedContent}, "stream": false})
	if suspendedResult.status != http.StatusConflict {
		t.Fatalf("suspended Release invocation status=%d body=%s", suspendedResult.status, suspendedResult.body)
	}
	assertPreInvocationError(t, suspendedResult, contracts.ErrorCodeAgentReleaseSuspended, env.forbidden)

	revoked := transitionRelease(t, client, *env, lifecycleRelease.ReleaseID, "revoke", contracts.ReleaseStateRevoked)
	if revoked.SuspendedAt == nil || revoked.RevokedAt == nil {
		t.Fatalf("revoked Release=%#v", revoked)
	}
	revokedResult := doRequest(t, client, env.controlPlane+"/v4/workspaces/"+acceptanceWorkspace+"/invocations", http.MethodPost, env.ownerToken, "application/json", map[string]any{"agentId": lifecycleRelease.AgentID, "capability": "runtime.echo", "input": map[string]any{"fixture": "success", "value": revokedContent}, "stream": false})
	if revokedResult.status != http.StatusConflict {
		t.Fatalf("revoked Release invocation status=%d body=%s", revokedResult.status, revokedResult.body)
	}
	assertPreInvocationError(t, revokedResult, contracts.ErrorCodeAgentReleaseRevoked, env.forbidden)
}

func configuredCredentialPrivateKey(t *testing.T, encoded string) ed25519.PrivateKey {
	t.Helper()
	value, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(value) != ed25519.PrivateKeySize {
		t.Fatalf("decode E2E Router credential private key: length=%d error=%v", len(value), err)
	}
	return ed25519.PrivateKey(value)
}

func signRouterCredential(t *testing.T, privateKey ed25519.PrivateKey, keyID string, claims contracts.RouterInvocationCredentialClaimsV1) string {
	t.Helper()
	headerJSON, err := json.Marshal(struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid"`
	}{Algorithm: contracts.RouterAgentCredentialAlgorithm, Type: contracts.RouterAgentCredentialType, KeyID: keyID})
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := header + "." + payload
	signature := ed25519.Sign(privateKey, []byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func agentRequestInContainer(t *testing.T, env acceptanceEnv, token string, contextHeaders map[string]string, requestID, content string) ([]byte, error) {
	t.Helper()
	payload := `{"jsonrpc":"2.0","id":"` + requestID + `","method":"message/send","params":{"message":{"kind":"message","messageId":"` + requestID + `","role":"user","parts":[{"kind":"data","data":{"fixture":"success","value":"` + content + `"}}]}}}`
	var request strings.Builder
	fmt.Fprintf(&request, "POST / HTTP/1.1\r\nHost: runtime-b:8092\r\nConnection: close\r\nContent-Type: application/json\r\nContent-Length: %d\r\n", len(payload))
	if token != "" {
		fmt.Fprintf(&request, "Authorization: Bearer %s\r\n", token)
	}
	names := make([]string, 0, len(contextHeaders))
	for name := range contextHeaders {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&request, "%s: %s\r\n", name, contextHeaders[name])
	}
	request.WriteString("\r\n")
	request.WriteString(payload)
	command := composeCommand(t.Context(), env, "exec", "-T", "runtime-b", "nc", "127.0.0.1", "8092")
	command.Stdin = strings.NewReader(request.String())
	return command.CombinedOutput()
}

func assertRouterCredentialFailureMatrix(t *testing.T, client *http.Client, env *acceptanceEnv, release contracts.AgentReleaseResponse) {
	t.Helper()
	configuredKey := configuredCredentialPrivateKey(t, env.credentialPrivate)
	forgedKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x5a}, ed25519.SeedSize))
	now := time.Now().UTC().Unix() - 1
	tests := []struct {
		name       string
		audience   string
		issuedAt   int64
		expiresAt  int64
		privateKey ed25519.PrivateKey
		wantStatus string
		wantCode   string
	}{
		{name: "forged", audience: "http://runtime-b:8092", issuedAt: now, expiresAt: now + 30, privateKey: forgedKey, wantStatus: "401 Unauthorized", wantCode: `"code":"UNAUTHENTICATED"`},
		{name: "expired", audience: "http://runtime-b:8092", issuedAt: now - 60, expiresAt: now - 30, privateKey: configuredKey, wantStatus: "401 Unauthorized", wantCode: `"code":"UNAUTHENTICATED"`},
		{name: "wrong-audience", audience: "http://runtime-a:8091", issuedAt: now, expiresAt: now + 30, privateKey: configuredKey, wantStatus: "403 Forbidden", wantCode: `"code":"FORBIDDEN"`},
	}
	for _, test := range tests {
		t.Run("Router credential "+test.name, func(t *testing.T) {
			content := "credential-" + test.name + "-must-not-execute"
			claims := contracts.RouterInvocationCredentialClaimsV1{
				Issuer: env.credentialIssuer, Audience: []string{test.audience}, ExpiresAt: test.expiresAt, IssuedAt: test.issuedAt,
				JWTID: "rtj_e2e_" + strings.ReplaceAll(test.name, "-", "_"), WorkspaceID: acceptanceWorkspace,
				AgentID: release.AgentID, AgentVersion: release.AgentCardVersion, ReleaseID: release.ReleaseID, CardDigest: release.CardDigest,
				Capability: "runtime.echo", InvocationID: "inv_credential_" + strings.ReplaceAll(test.name, "-", "_"),
				RootTaskID: "task_credential_" + strings.ReplaceAll(test.name, "-", "_"), TraceID: contracts.TraceID("trace-credential-" + test.name),
			}
			token := signRouterCredential(t, test.privateKey, env.credentialKeyID, claims)
			env.forbidCredential(token)
			env.forbid(content)
			contextHeaders := contracts.RouterAgentContextHeadersV1(contracts.RouterInvocationCredentialContextV1{
				WorkspaceID: claims.WorkspaceID, AgentID: claims.AgentID, AgentVersion: claims.AgentVersion, ReleaseID: claims.ReleaseID,
				CardDigest: claims.CardDigest, Capability: claims.Capability, InvocationID: claims.InvocationID,
				RootTaskID: claims.RootTaskID, TraceID: claims.TraceID,
			})
			output, err := agentRequestInContainer(t, *env, token, contextHeaders, "credential-"+test.name, content)
			assertNoForbiddenBody(t, output, env.forbidden, test.name+" Router credential rejection")
			if err != nil || !bytes.Contains(output, []byte(test.wantStatus)) || !bytes.Contains(output, []byte(test.wantCode)) {
				t.Fatalf("%s Router credential was not rejected exactly: err=%v", test.name, err)
			}
			assertInvocationAbsent(t, client, *env, claims.InvocationID)
		})
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
	release, exists := env.releases[agentID]
	if !exists {
		t.Fatalf("accepted Agent %s has no published Release fixture", agentID)
	}
	if err := validateExpectedReleaseProvenance(detail.Invocation, release); err != nil {
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

func assertInvocationAbsent(t *testing.T, client *http.Client, env acceptanceEnv, invocationID string) {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+fmt.Sprintf("/v4/workspaces/%s/invocations/%s", acceptanceWorkspace, invocationID), http.MethodGet, env.ownerToken, "", nil)
	assertNoForbiddenBody(t, result.body, env.forbidden, "absent Invocation response")
	if result.status != http.StatusNotFound {
		t.Fatalf("rejected direct Agent request created Invocation %s: read status=%d", invocationID, result.status)
	}
	var failure platformErrorObservation
	if err := json.Unmarshal(result.body, &failure); err != nil || failure.Code != contracts.ErrorCodeNotFound {
		t.Fatalf("absent Invocation %s did not return NOT_FOUND: code=%q err=%v", invocationID, failure.Code, err)
	}
}

func validateInvocationDetail(detail contracts.InvocationDetailResponseV4, invocationID, workspaceID, agentID, status, errorCode string) error {
	if detail.Invocation.InvocationID != invocationID || detail.Invocation.WorkspaceID != workspaceID || detail.Invocation.TargetAgentID != agentID || detail.Invocation.Status != status || len(detail.Events) == 0 {
		return fmt.Errorf("record projection=%#v events=%#v", detail.Invocation, detail.Events)
	}
	if detail.Invocation.AgentReleaseID == "" || detail.Invocation.AgentCardDigest == "" {
		return fmt.Errorf("trusted Invocation %s has no Release provenance: %#v", invocationID, detail.Invocation)
	}
	if errorCode != "" && string(detail.Invocation.ErrorCode) != errorCode {
		return fmt.Errorf("record error=%q want=%q", detail.Invocation.ErrorCode, errorCode)
	}
	terminalCount := 0
	for index, event := range detail.Events {
		if event.Sequence != int64(index) || event.InvocationID != invocationID || event.RootTaskID != detail.Invocation.RootTaskID || event.ParentInvocationID != detail.Invocation.ParentInvocationID || event.TraceID != detail.Invocation.TraceID || event.Caller != detail.Invocation.Caller || event.WorkspaceID != workspaceID || event.TargetAgentID != detail.Invocation.TargetAgentID || event.AgentCardVersion != detail.Invocation.AgentCardVersion || event.AgentReleaseID != detail.Invocation.AgentReleaseID || event.AgentCardDigest != detail.Invocation.AgentCardDigest || event.Capability != detail.Invocation.Capability {
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

func validateExpectedReleaseProvenance(record contracts.InvocationRecordV4, release contracts.AgentReleaseResponse) error {
	if record.AgentReleaseID != release.ReleaseID || record.AgentCardDigest != release.CardDigest || record.TargetAgentID != release.AgentID || record.AgentCardVersion != release.AgentCardVersion {
		return fmt.Errorf("Invocation Release provenance=%#v want Release=%#v", record, release)
	}
	return nil
}

func assertQueryableRelease(t *testing.T, client *http.Client, env acceptanceEnv, expected contracts.AgentReleaseResponse) {
	t.Helper()
	result := doRequest(t, client, env.controlPlane+"/v4/releases/"+expected.ReleaseID, http.MethodGet, env.ownerToken, "", nil)
	if result.status != http.StatusOK {
		t.Fatalf("read Release %s status=%d body=%s", expected.ReleaseID, result.status, result.body)
	}
	assertNoForbiddenBody(t, result.body, env.forbidden, "Agent Release read response")
	var release contracts.AgentReleaseResponse
	if err := json.Unmarshal(result.body, &release); err != nil {
		t.Fatalf("decode Release %s: %v", expected.ReleaseID, err)
	}
	if release.ReleaseID != expected.ReleaseID || release.ProviderID != acceptanceProviderID || release.AgentID != expected.AgentID || release.AgentCardVersion != expected.AgentCardVersion || release.CardDigest != expected.CardDigest || release.EndpointBindingID != expected.EndpointBindingID || release.EndpointOrigin != expected.EndpointOrigin || release.EndpointPath != expected.EndpointPath || release.VerificationMethod != "http_well_known" || release.VerificationEvidenceDigest == nil || release.State != contracts.ReleaseStatePublished || release.PublishedAt == nil {
		t.Fatalf("queryable Release=%#v want=%#v", release, expected)
	}
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
		release, exists := env.releases[projection.TargetAgentID]
		if !exists {
			t.Fatalf("Trace Agent %s has no published Release fixture", projection.TargetAgentID)
		}
		if err := validateExpectedReleaseProvenance(projection, release); err != nil {
			t.Fatal(err)
		}
		if err := validateExpectedReleaseProvenance(detail.Invocation, release); err != nil {
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
			return fmt.Errorf("forbidden secret material appeared in %s", surface)
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
	err := forbiddenBodyError([]byte("secret-value"), []string{"secret-value"}, "detector fixture")
	if err == nil {
		t.Fatal("forbidden literal detector did not detect secret material")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatal("forbidden literal detector exposed secret material")
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
	routeFailure := assertCorrelatedInvocationError(t, route, contracts.ErrorCodeAgentUnavailable, env.forbidden)
	assertRecord(t, client, env, routeFailure.InvocationID, acceptanceWorkspace, "runtime-route", "failed", string(contracts.ErrorCodeAgentUnavailable))
	timedOut := invokeSSE(t, client, env, "runtime-timeout", "runtime.timeout", map[string]any{"fixture": "hold", "value": "timeout-content-secret"})
	timeoutInvocationID := assertStreamTerminal(t, timedOut, contracts.ResultStreamEventTimedOut, contracts.ErrorCodeTimeout, env.forbidden)
	assertRecord(t, client, env, timeoutInvocationID, acceptanceWorkspace, "runtime-timeout", "timed_out", string(contracts.ErrorCodeTimeout))
	canceledInvocationIDs := make(map[string]struct{}, 5)
	for attempt := 0; attempt < 5; attempt++ {
		canceledInvocationID := invokeCanceledSSE(t, client, env, "runtime-interrupted", "runtime.interrupted")
		if _, exists := canceledInvocationIDs[canceledInvocationID]; exists {
			t.Fatalf("caller cancellation attempt %d reused Invocation %s", attempt+1, canceledInvocationID)
		}
		canceledInvocationIDs[canceledInvocationID] = struct{}{}
		waitForRecord(t, client, env, canceledInvocationID, "canceled", string(contracts.ErrorCodeCanceled))
	}
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
	stop := composeCommand(t.Context(), env, "stop", "control-plane")
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
	start := composeCommand(t.Context(), env, "start", "control-plane")
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

type platformErrorObservation struct {
	Code         contracts.PlatformErrorCode `json:"code"`
	TraceID      contracts.TraceID           `json:"traceId"`
	InvocationID string                      `json:"invocationId"`
	RootTaskID   string                      `json:"rootTaskId"`
}

func assertErrorCode(t *testing.T, result httpResult, want contracts.PlatformErrorCode, forbidden ...[]string) platformErrorObservation {
	t.Helper()
	if len(forbidden) == 1 {
		assertNoForbiddenBody(t, result.body, forbidden[0], "failure response")
	}
	if result.status == http.StatusOK {
		t.Fatal("failure unexpectedly succeeded")
	}
	var value platformErrorObservation
	if err := json.Unmarshal(result.body, &value); err != nil || value.Code != want {
		t.Fatalf("error status=%d code=%q want=%q err=%v", result.status, value.Code, want, err)
	}
	if value.TraceID == "" || len(result.header.Values("x-nek-trace-id")) != 1 || result.header.Get("x-nek-trace-id") != string(value.TraceID) {
		t.Fatalf("error Trace header/body mismatch: header=%q value=%#v", result.header.Values("x-nek-trace-id"), value)
	}
	return value
}

func assertPreInvocationError(t *testing.T, result httpResult, want contracts.PlatformErrorCode, forbidden []string) platformErrorObservation {
	t.Helper()
	value := assertErrorCode(t, result, want, forbidden)
	if value.InvocationID != "" || value.RootTaskID != "" {
		t.Fatalf("pre-acceptance error contains Invocation correlation: %#v", value)
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidatePreCorrelationPlatformErrorV4JSON(result.body); err != nil {
		t.Fatalf("invalid pre-correlation Platform Error v4: %v body=%s", err, result.body)
	}
	return value
}

func assertCorrelatedInvocationError(t *testing.T, result httpResult, want contracts.PlatformErrorCode, forbidden []string) platformErrorObservation {
	t.Helper()
	value := assertErrorCode(t, result, want, forbidden)
	if value.InvocationID == "" || value.RootTaskID == "" {
		t.Fatalf("accepted failure has no Invocation correlation: %#v", value)
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidateCorrelatedPlatformErrorV4JSON(result.body); err != nil {
		t.Fatalf("invalid correlated Platform Error v4: %v body=%s", err, result.body)
	}
	return value
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
	rows, err := database.QueryContext(ctx, `SELECT event_id, event_type, status, invocation_id, root_task_id, COALESCE(parent_invocation_id, ''), trace_id, caller_id, workspace_id, target_agent_id, agent_card_version, COALESCE(agent_release_id, ''), COALESCE(encode(agent_card_digest, 'hex'), ''), capability, COALESCE(error_code, '') FROM ledger.invocation_events`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fields [15]string
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
	projectionRows, err := database.QueryContext(ctx, `SELECT invocation_id, root_task_id, COALESCE(parent_invocation_id, ''), trace_id, caller_type, caller_id, workspace_id, target_agent_id, agent_card_version, COALESCE(agent_release_id, ''), COALESCE(encode(agent_card_digest, 'hex'), ''), capability, status, COALESCE(error_code, '') FROM ledger.invocations`)
	if err != nil {
		t.Fatal(err)
	}
	defer projectionRows.Close()
	for projectionRows.Next() {
		var fields [14]string
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
	bindingRows, err := database.QueryContext(ctx, `SELECT binding_id, provider_id, agent_id, agent_card_version, endpoint, endpoint_origin, endpoint_path, verification_method, verification_status, COALESCE(encode(verification_evidence_digest, 'hex'), ''), COALESCE(verification_failure_code, '') FROM catalog.endpoint_bindings`)
	if err != nil {
		t.Fatal(err)
	}
	for bindingRows.Next() {
		var fields [11]string
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
	challengeRows, err := database.QueryContext(ctx, `SELECT challenge_id, binding_id, encode(proof_digest, 'hex'), expires_at::text, COALESCE(used_at::text, ''), created_at::text FROM catalog.verification_challenges`)
	if err != nil {
		t.Fatal(err)
	}
	for challengeRows.Next() {
		var fields [6]string
		args := make([]any, len(fields))
		for index := range fields {
			args[index] = &fields[index]
		}
		if err := challengeRows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		assertNoForbiddenBody(t, []byte(strings.Join(fields[:], "|")), env.forbidden, "persisted verification challenge")
	}
	if err := challengeRows.Err(); err != nil {
		t.Fatal(err)
	}
	challengeRows.Close()
	releaseRows, err := database.QueryContext(ctx, `SELECT release_id, provider_id, agent_id, agent_card_version, encode(card_digest, 'hex'), endpoint_binding_id, endpoint_origin, endpoint_path, verification_method, COALESCE(encode(verification_evidence_digest, 'hex'), ''), state FROM catalog.agent_releases`)
	if err != nil {
		t.Fatal(err)
	}
	for releaseRows.Next() {
		var fields [11]string
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
	logs := composeCommand(ctx, env, "logs", "--no-color")
	output, err := logs.Output()
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenBody(t, output, env.forbidden, "process logs")
}

func restartRouter(t *testing.T, env acceptanceEnv) {
	t.Helper()
	command := composeCommand(t.Context(), env, "restart", "a2a-router")
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
