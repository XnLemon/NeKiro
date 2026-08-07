package kubernetes

import (
	"context"
	"io"
	"strings"
)

const KubernetesRequestExecutorVersionV1 = "v1"

// KubernetesRequestExecutorGuarantees is the explicit v1 declaration made by
// an injected executor. The Directory validates every field at construction;
// it never creates a default HTTP client or transport.
type KubernetesRequestExecutorGuarantees struct {
	Version                  string
	ExactlyOneAttempt        bool
	NoRedirect               bool
	NoEnvironmentProxy       bool
	NoResponseCache          bool
	NoImplicitLimiter        bool
	NoRetry                  bool
	NoAuthoritySwitch        bool
	NoHiddenReauthentication bool
}

func (g KubernetesRequestExecutorGuarantees) Validate() error {
	if g.Version != KubernetesRequestExecutorVersionV1 || !g.ExactlyOneAttempt ||
		!g.NoRedirect || !g.NoEnvironmentProxy || !g.NoResponseCache ||
		!g.NoImplicitLimiter || !g.NoRetry || !g.NoAuthoritySwitch ||
		!g.NoHiddenReauthentication {
		return invalidInput()
	}
	return nil
}

// KubernetesRequest is a fully specified immutable request passed to a
// KubernetesRequestExecutor. It contains no inferred credentials or transport
// policy.
type KubernetesRequest struct {
	method  string
	url     string
	headers map[string][]string
}

func newKubernetesRequest(method, rawURL string, headers map[string][]string) KubernetesRequest {
	return KubernetesRequest{
		method:  method,
		url:     rawURL,
		headers: cloneHeaders(headers),
	}
}

func (r KubernetesRequest) Method() string { return r.method }
func (r KubernetesRequest) URL() string    { return r.url }

// Headers returns a deep copy. Mutating it cannot alter a request retained by
// the Directory or another executor invocation.
func (r KubernetesRequest) Headers() map[string][]string { return cloneHeaders(r.headers) }

// Header returns a copy of all exact values for name.
func (r KubernetesRequest) Header(name string) []string {
	for key, values := range r.headers {
		if strings.EqualFold(key, name) {
			return append([]string(nil), values...)
		}
	}
	return nil
}

// KubernetesResponse is the raw result returned by an injected executor.
// Bodies are owned by the Directory after a successful Execute call and are
// always closed when their List or observation terminates.
type KubernetesResponse struct {
	StatusCode int
	Header     map[string][]string
	Body       io.ReadCloser
}

// KubernetesRequestExecutor is the sole network boundary accepted by this
// provider. It intentionally is not compatible with http.Client or
// http.RoundTripper.
type KubernetesRequestExecutor interface {
	Guarantees() KubernetesRequestExecutorGuarantees
	Execute(context.Context, KubernetesRequest) (KubernetesResponse, error)
}

func cloneHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return nil
	}
	copyHeaders := make(map[string][]string, len(headers))
	for key, values := range headers {
		copyHeaders[key] = append([]string(nil), values...)
	}
	return copyHeaders
}
