package clientsdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	clientsdk "github.com/Nene7ko/NeKiro/sdks/client-sdk"
)

func ExampleClient() {
	// Installation is a separate, explicit Gateway operation. Once an Agent is
	// installed and enabled, application code binds one Client to that
	// Workspace and never supplies an endpoint, version, Release, or Router.
	client, err := clientsdk.NewClient(clientsdk.Config{
		HTTPClient:            &http.Client{Timeout: 30 * time.Second},
		GatewayOrigin:         "https://api.nekiro.dev",
		WorkspaceID:           "workspace-production",
		ApplicationCredential: os.Getenv("NEKIRO_APPLICATION_CREDENTIAL"),
		RequestLimitBytes:     1 << 20,
		ResponseLimitBytes:    4 << 20,
		StreamEventLimitBytes: 256 << 10,
	})
	if err != nil {
		return
	}

	result, err := client.Invoke(context.Background(), clientsdk.InvokeRequest{
		AgentID:    "summarizer",
		Capability: "document.summarize",
		Input:      json.RawMessage(`{"document":"..."}`),
	})
	if err != nil {
		var platformError *clientsdk.PlatformError
		if errors.As(err, &platformError) && platformError.Code == contracts.ErrorCodeAgentNotInstalled {
			// Ask the Workspace owner to install the Agent; the SDK does not
			// silently install or select another destination.
		}
		return
	}
	_ = result.Output

	stream, err := client.InvokeStream(context.Background(), clientsdk.InvokeRequest{
		AgentID:    "summarizer",
		Capability: "document.summarize",
		Input:      json.RawMessage(`{"document":"..."}`),
	})
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return
		}
		_ = event
	}
}
