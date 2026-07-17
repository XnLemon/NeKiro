package resolution

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

type Client struct {
	doer          HTTPDoer
	url           string
	token         string
	responseLimit int64
}

type Failure struct {
	StatusCode int
	Code       contracts.PlatformErrorCode
	TraceID    contracts.TraceID
	Body       []byte
}

func (failure *Failure) Error() string { return string(failure.Code) }

func NewClient(doer HTTPDoer, url, token string, responseLimit int64) (*Client, error) {
	if doer == nil || url == "" || token == "" || responseLimit < contracts.RuntimeByteLimitMinimum || responseLimit > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("resolution client dependencies are required")
	}
	return &Client{doer: doer, url: url, token: token, responseLimit: responseLimit}, nil
}

func (client *Client) Resolve(ctx context.Context, requestValue contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(requestValue); err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("encode Control Plane resolution request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.url, &body)
	if err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("construct Control Plane resolution request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.doer.Do(request)
	if err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("resolve through Control Plane: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.Header.Get("Content-Type") != "application/json" {
		return contracts.ResolveAgentResponse{}, errors.New("control plane resolution response media is invalid")
	}
	data, err := readBounded(response.Body, client.responseLimit)
	if err != nil {
		return contracts.ResolveAgentResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		var platformError struct {
			Code contracts.PlatformErrorCode `json:"code"`
		}
		if err := json.Unmarshal(data, &platformError); err != nil || platformError.Code == "" {
			return contracts.ResolveAgentResponse{}, errors.New("control plane resolution error body is invalid")
		}
		traceID := contracts.TraceID(response.Header.Get("x-nek-trace-id"))
		if traceID == "" {
			return contracts.ResolveAgentResponse{}, errors.New("control plane resolution error trace header is missing")
		}
		return contracts.ResolveAgentResponse{}, &Failure{StatusCode: response.StatusCode, Code: platformError.Code, TraceID: traceID, Body: append([]byte(nil), data...)}
	}
	var resolved contracts.ResolveAgentResponse
	if err := json.Unmarshal(data, &resolved); err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("decode Control Plane resolution response: %w", err)
	}
	return resolved, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("control plane resolution response is too large")
	}
	return data, nil
}
