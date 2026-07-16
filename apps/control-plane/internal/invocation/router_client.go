package invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type RouterResponse struct {
	StatusCode  int
	ContentType string
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

func (client *RouterClient) Dispatch(ctx context.Context, value contracts.DispatchInvocationRequestV3, mode contracts.InvocationResultMode) (*RouterResponse, error) {
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
	return &RouterResponse{StatusCode: response.StatusCode, ContentType: contentType, Body: response.Body}, nil
}
