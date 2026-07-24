package invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/Nene7ko/NeKiro/contracts"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type RouterResponse struct {
	StatusCode  int
	ContentType string
	Headers     http.Header
	Body        io.ReadCloser
}

type RouterClient struct {
	doer  HTTPDoer
	url   string
	token string
}

func NewRouterClient(doer HTTPDoer, url, token string) (*RouterClient, error) {
	if doer == nil || url == "" || token == "" {
		return nil, errors.New("Router client dependencies are required")
	}
	return &RouterClient{doer: doer, url: url, token: token}, nil
}

func (client *RouterClient) Dispatch(ctx context.Context, value contracts.DispatchInvocationRequestV4, mode contracts.InvocationResultMode) (*RouterResponse, error) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode Router dispatch request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.url, &body)
	if err != nil {
		return nil, fmt.Errorf("construct Router dispatch request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	switch mode {
	case contracts.InvocationResultModeJSON:
		request.Header.Set("Accept", "application/json")
	case contracts.InvocationResultModeSSE:
		request.Header.Set("Accept", "text/event-stream")
	default:
		return nil, errors.New("unsupported invocation result mode")
	}
	response, err := client.doer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("dispatch to Router: %w", err)
	}
	contentType := response.Header.Get("Content-Type")
	want := "application/json"
	if response.StatusCode == http.StatusOK && mode == contracts.InvocationResultModeSSE {
		want = "text/event-stream"
	}
	if contentType != want {
		_ = response.Body.Close()
		return nil, fmt.Errorf("Router response Content-Type %q does not match %q", contentType, want)
	}
	return &RouterResponse{StatusCode: response.StatusCode, ContentType: contentType, Headers: response.Header.Clone(), Body: response.Body}, nil
}

// GetInvocation reads one Workspace-scoped metadata projection from the same
// explicitly configured Router origin as dispatch. The path is fixed by the
// active Router Internal v3 contract and never comes from the caller.
func (client *RouterClient) GetInvocation(ctx context.Context, workspaceID, invocationID string) (*RouterResponse, error) {
	if !validReadIdentifier(workspaceID) || !validReadIdentifier(invocationID) {
		return nil, errors.New("Router Invocation read identifiers are invalid")
	}
	return client.getMetadata(ctx, "/internal/v3/workspaces/"+workspaceID+"/invocations/"+invocationID)
}

// GetTrace reads one Workspace-scoped metadata lineage from the same Router
// origin. Trace ordering and lineage validation remain Router Ledger concerns.
func (client *RouterClient) GetTrace(ctx context.Context, workspaceID string, traceID contracts.TraceID) (*RouterResponse, error) {
	if !validReadIdentifier(workspaceID) {
		return nil, errors.New("Router Trace Workspace identifier is invalid")
	}
	if _, err := contracts.ParseTraceID(string(traceID)); err != nil {
		return nil, err
	}
	return client.getMetadata(ctx, "/internal/v3/workspaces/"+workspaceID+"/traces/"+string(traceID))
}

func (client *RouterClient) getMetadata(ctx context.Context, path string) (*RouterResponse, error) {
	target, err := client.metadataURL(path)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("construct Router metadata request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/json")
	response, err := client.doer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read metadata from Router: %w", err)
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("Router metadata response is empty")
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "application/json" {
		_ = response.Body.Close()
		return nil, fmt.Errorf("Router metadata response Content-Type %q does not match %q", contentType, "application/json")
	}
	return &RouterResponse{StatusCode: response.StatusCode, ContentType: contentType, Headers: response.Header.Clone(), Body: response.Body}, nil
}

func (client *RouterClient) metadataURL(path string) (string, error) {
	parsed, err := url.Parse(client.url)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Router metadata destination is invalid")
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed.String(), nil
}

func validReadIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for index, character := range []byte(value) {
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == ':' || character == '-' {
			if index > 0 || character != '.' && character != '_' && character != ':' && character != '-' {
				continue
			}
		}
		return false
	}
	return true
}
