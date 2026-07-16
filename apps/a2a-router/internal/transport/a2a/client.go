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
	httpClient *http.Client
}

type ContextHeaders struct {
	TraceID            contracts.TraceID
	InvocationID       string
	RootTaskID         string
	ParentInvocationID string
	WorkspaceID        string
}

func NewClient(httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("A2A transport HTTP client is required")
	}
	return &Client{httpClient: httpClient}, nil
}

func (client *Client) SendMessage(ctx context.Context, target Target, headers ContextHeaders, params *a2ago.MessageSendParams) (a2ago.SendMessageResult, error) {
	if target.Endpoint == "" {
		return nil, errors.New("A2A target endpoint is required")
	}
	if params == nil || params.Message == nil {
		return nil, errors.New("A2A message/send params are required")
	}
	if headers.TraceID == "" || headers.InvocationID == "" || headers.RootTaskID == "" || headers.WorkspaceID == "" {
		return nil, errors.New("platform context headers are required")
	}
	a2aClient, err := a2aclient.NewFromEndpoints(ctx, []a2ago.AgentInterface{{Transport: a2ago.TransportProtocolJSONRPC, URL: target.Endpoint}}, a2aclient.WithJSONRPCTransport(client.httpClient))
	if err != nil {
		return nil, err
	}
	a2aClient.AddCallInterceptor(a2aclient.NewStaticCallMetaInjector(a2aclient.CallMeta{
		HeaderTraceID:            []string{string(headers.TraceID)},
		HeaderInvocationID:       []string{headers.InvocationID},
		HeaderRootTaskID:         []string{headers.RootTaskID},
		HeaderParentInvocationID: []string{headers.ParentInvocationID},
		HeaderWorkspaceID:        []string{headers.WorkspaceID},
	}))
	return a2aClient.SendMessage(ctx, params)
}
