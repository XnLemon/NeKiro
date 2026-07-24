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
	HeaderTraceID            = contracts.RouterAgentTraceHeader
	HeaderInvocationID       = contracts.RouterAgentInvocationHeader
	HeaderRootTaskID         = contracts.RouterAgentRootTaskHeader
	HeaderParentInvocationID = contracts.RouterAgentParentInvocationHeader
	HeaderWorkspaceID        = contracts.RouterAgentWorkspaceHeader
)

type Client struct {
	httpClient         *http.Client
	credentialIssuer   CredentialIssuer
	inputLimitBytes    int64
	responseLimitBytes int64
	a2aEventLimitBytes int64
	sseEventLimitBytes int64
}

type CredentialIssuer interface {
	Issue(contracts.RouterInvocationCredentialContextV1) (string, error)
}

type ContextHeaders struct {
	TraceID            contracts.TraceID
	InvocationID       string
	RootTaskID         string
	ParentInvocationID string
	WorkspaceID        string
}

func NewClient(httpClient *http.Client, credentialIssuer CredentialIssuer, inputLimitBytes, responseLimitBytes, a2aEventLimitBytes, sseEventLimitBytes int64) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("A2A transport HTTP client is required")
	}
	if credentialIssuer == nil {
		return nil, errors.New("a2a transport credential issuer is required")
	}
	if inputLimitBytes < contracts.RuntimeByteLimitMinimum || inputLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("A2A Agent input limit is invalid")
	}
	if responseLimitBytes < contracts.RuntimeByteLimitMinimum || responseLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("A2A Agent response limit is invalid")
	}
	if a2aEventLimitBytes < contracts.RuntimeByteLimitMinimum || a2aEventLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("A2A event limit is invalid")
	}
	if sseEventLimitBytes < contracts.RuntimeByteLimitMinimum || sseEventLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("SSE event limit is invalid")
	}
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{httpClient: &client, credentialIssuer: credentialIssuer, inputLimitBytes: inputLimitBytes, responseLimitBytes: responseLimitBytes, a2aEventLimitBytes: a2aEventLimitBytes, sseEventLimitBytes: sseEventLimitBytes}, nil
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
	a2aClient.AddCallInterceptor(newCredentialInterceptor(client.credentialIssuer, credentialContext(target, headers)))
	result, err := a2aClient.SendMessage(ctx, params)
	if err != nil {
		return nil, classifyTransportError(err)
	}
	return result, nil
}

type credentialInterceptor struct {
	a2aclient.PassthroughInterceptor
	issuer  CredentialIssuer
	context contracts.RouterInvocationCredentialContextV1
}

func newCredentialInterceptor(issuer CredentialIssuer, context contracts.RouterInvocationCredentialContextV1) a2aclient.CallInterceptor {
	return &credentialInterceptor{issuer: issuer, context: context}
}

func (interceptor *credentialInterceptor) Before(ctx context.Context, request *a2aclient.Request) (context.Context, error) {
	token, err := interceptor.issuer.Issue(interceptor.context)
	if err != nil {
		return ctx, classify(contracts.ErrorCodeInternal, err)
	}
	request.Meta.Append(contracts.RouterAgentAuthorizationHeader, "Bearer "+token)
	for name, value := range contracts.RouterAgentContextHeadersV1(interceptor.context) {
		request.Meta.Append(name, value)
	}
	return ctx, nil
}

func credentialContext(target Target, headers ContextHeaders) contracts.RouterInvocationCredentialContextV1 {
	return contracts.RouterInvocationCredentialContextV1{
		Audience: target.Audience, WorkspaceID: headers.WorkspaceID, AgentID: target.AgentID, AgentVersion: target.Version,
		ReleaseID: target.ReleaseID, CardDigest: target.CardDigest, Capability: target.Capability, InvocationID: headers.InvocationID,
		RootTaskID: headers.RootTaskID, ParentInvocationID: headers.ParentInvocationID, TraceID: headers.TraceID,
	}
}
