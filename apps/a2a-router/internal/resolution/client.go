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
	doer            HTTPDoer
	url             string
	versionURL      string
	token           string
	responseLimit   int64
	validator       *contracts.Validator
	resultValidator *contracts.ResultContractValidator
}

type Failure struct {
	StatusCode int
	Code       contracts.PlatformErrorCode
	TraceID    contracts.TraceID
	Body       []byte
}

func (failure *Failure) Error() string { return string(failure.Code) }

func NewClient(doer HTTPDoer, url, token string, responseLimit int64) (*Client, error) {
	return NewClientWithVersionURL(doer, url, "", token, responseLimit)
}

// NewClientWithVersionURL creates a resolution client with an optional
// Control Plane Internal v3 version resolution endpoint.
func NewClientWithVersionURL(doer HTTPDoer, url, versionURL, token string, responseLimit int64) (*Client, error) {
	if doer == nil || url == "" || token == "" || responseLimit < contracts.RuntimeByteLimitMinimum || responseLimit > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("resolution client dependencies are required")
	}
	if httpClient, ok := doer.(*http.Client); ok {
		if httpClient == nil {
			return nil, errors.New("resolution client dependencies are required")
		}
		client := *httpClient
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		doer = &client
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("initialize Control Plane resolution validator: %w", err)
	}
	resultValidator, err := contracts.NewResultContractValidator()
	if err != nil {
		return nil, fmt.Errorf("initialize Control Plane error validator: %w", err)
	}
	return &Client{
		doer: doer, url: url, versionURL: versionURL, token: token, responseLimit: responseLimit,
		validator: validator, resultValidator: resultValidator,
	}, nil
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
		var platformError contracts.PlatformErrorV3
		if err := json.Unmarshal(data, &platformError); err != nil {
			return contracts.ResolveAgentResponse{}, errors.New("control plane resolution error body is invalid")
		}
		if err := client.resultValidator.ValidateResolveAgentErrorCorrelationV3(requestValue, platformError); err != nil {
			return contracts.ResolveAgentResponse{}, errors.New("control plane resolution error correlation is invalid")
		}
		traceID, err := contracts.ParseTraceID(response.Header.Get("x-nek-trace-id"))
		if err != nil || traceID != platformError.TraceID {
			return contracts.ResolveAgentResponse{}, errors.New("control plane resolution error trace header is invalid")
		}
		return contracts.ResolveAgentResponse{}, &Failure{StatusCode: response.StatusCode, Code: platformError.Code, TraceID: traceID, Body: append([]byte(nil), data...)}
	}
	var resolved contracts.ResolveAgentResponse
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resolved); err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("decode Control Plane resolution response: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("decode Control Plane resolution response: %w", err)
	}
	if err := client.validator.ValidateResolveAgentResponseForRequest(requestValue, resolved); err != nil {
		return contracts.ResolveAgentResponse{}, fmt.Errorf("validate Control Plane resolution response: %w", err)
	}
	return resolved, nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
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

// ResolveInstalledVersion calls the Control Plane Internal v3 endpoint to
// resolve the deterministic installed Agent Card version from the enabled
// Installation. It returns the exact pinned version.
func (client *Client) ResolveInstalledVersion(ctx context.Context, requestValue contracts.ResolveInstalledVersionRequest) (contracts.ResolveInstalledVersionResponse, error) {
	if client.versionURL == "" {
		return contracts.ResolveInstalledVersionResponse{}, errors.New("resolution client version URL is not configured")
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(requestValue); err != nil {
		return contracts.ResolveInstalledVersionResponse{}, fmt.Errorf("encode version resolution request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.versionURL, &body)
	if err != nil {
		return contracts.ResolveInstalledVersionResponse{}, fmt.Errorf("construct version resolution request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.doer.Do(request)
	if err != nil {
		return contracts.ResolveInstalledVersionResponse{}, fmt.Errorf("resolve installed version through Control Plane: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.Header.Get("Content-Type") != "application/json" {
		return contracts.ResolveInstalledVersionResponse{}, errors.New("control plane version resolution response media is invalid")
	}
	data, err := readBounded(response.Body, client.responseLimit)
	if err != nil {
		return contracts.ResolveInstalledVersionResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		var platformError contracts.PlatformErrorV3
		if err := json.Unmarshal(data, &platformError); err != nil {
			return contracts.ResolveInstalledVersionResponse{}, errors.New("control plane version resolution error body is invalid")
		}
		traceID, err := contracts.ParseTraceID(response.Header.Get("x-nek-trace-id"))
		if err != nil || traceID != platformError.TraceID {
			return contracts.ResolveInstalledVersionResponse{}, errors.New("control plane version resolution error trace header is invalid")
		}
		return contracts.ResolveInstalledVersionResponse{}, &Failure{StatusCode: response.StatusCode, Code: platformError.Code, TraceID: traceID, Body: append([]byte(nil), data...)}
	}
	var resolved contracts.ResolveInstalledVersionResponse
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resolved); err != nil {
		return contracts.ResolveInstalledVersionResponse{}, fmt.Errorf("decode version resolution response: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return contracts.ResolveInstalledVersionResponse{}, fmt.Errorf("decode version resolution response: %w", err)
	}
	return resolved, nil
}
