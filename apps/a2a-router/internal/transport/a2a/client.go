package a2a

import (
	"context"
	"errors"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
	a2ago "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
)

const (
	HeaderTraceID            = "x-nek-trace-id"
	HeaderInvocationID       = "x-nek-invocation-id"
	HeaderRootTaskID         = "x-nek-root-task-id"
	HeaderParentInvocationID = "x-nek-parent-invocation-id"
	HeaderWorkspaceID        = "x-nek-workspace-id"
)

type Client struct {
	httpClient         *http.Client
	inputLimitBytes    int64
	responseLimitBytes int64
}

type ContextHeaders struct {
	TraceID            contracts.TraceID
	InvocationID       string
	RootTaskID         string
	ParentInvocationID string
	WorkspaceID        string
}

func NewClient(httpClient *http.Client, inputLimitBytes, responseLimitBytes int64) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("A2A transport HTTP client is required")
	}
	if inputLimitBytes < contracts.RuntimeByteLimitMinimum || inputLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("A2A Agent input limit is invalid")
	}
	if responseLimitBytes < contracts.RuntimeByteLimitMinimum || responseLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("A2A Agent response limit is invalid")
	}
	client := *httpClient
	return &Client{httpClient: &client, inputLimitBytes: inputLimitBytes, responseLimitBytes: responseLimitBytes}, nil
}

func (client *Client) SendMessage(ctx context.Context, target Target, headers ContextHeaders, params *a2ago.MessageSendParams) (a2ago.SendMessageResult, error) {
	if target.Endpoint == "" {
		return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A target endpoint is required"))
	}
	if params == nil || params.Message == nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("A2A message/send params are required"))
	}
	if headers.TraceID == "" || headers.InvocationID == "" || headers.RootTaskID == "" || headers.WorkspaceID == "" {
		return nil, classify(contracts.ErrorCodeA2AProtocol, errors.New("platform context headers are required"))
	}
	httpClient := *client.httpClient
	base := httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	responseLimit := client.responseLimitBytes
	if target.MaxOutputBytes < responseLimit {
		responseLimit = target.MaxOutputBytes
	}
	httpClient.Transport = envelopeValidatingRoundTripper{base: base, maxResponseBytes: responseLimit}
	a2aClient, err := a2aclient.NewFromEndpoints(ctx, []a2ago.AgentInterface{{Transport: a2ago.TransportProtocolJSONRPC, URL: target.Endpoint}}, a2aclient.WithJSONRPCTransport(&httpClient))
	if err != nil {
		return nil, classify(contracts.ErrorCodeA2AProtocol, err)
	}
	a2aClient.AddCallInterceptor(a2aclient.NewStaticCallMetaInjector(a2aclient.CallMeta{
		HeaderTraceID:            []string{string(headers.TraceID)},
		HeaderInvocationID:       []string{headers.InvocationID},
		HeaderRootTaskID:         []string{headers.RootTaskID},
		HeaderParentInvocationID: []string{headers.ParentInvocationID},
		HeaderWorkspaceID:        []string{headers.WorkspaceID},
	}))
	result, err := a2aClient.SendMessage(ctx, params)
	if err != nil {
		return nil, classifyTransportError(err)
	}
	return result, nil
}
