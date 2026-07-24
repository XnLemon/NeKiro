package routerauth

import (
	"context"
	"net/http"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

type claimsContextKey struct{}

type Middleware struct {
	verifier *Verifier
}

func NewMiddleware(config Config, now func() time.Time) (*Middleware, error) {
	verifier, err := NewVerifier(config, now)
	if err != nil {
		return nil, err
	}
	return &Middleware{verifier: verifier}, nil
}

func (middleware *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		claims, err := middleware.verifier.Verify(request)
		if err != nil {
			writeVerificationError(writer, err)
			return
		}
		ctx := context.WithValue(request.Context(), claimsContextKey{}, claims)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func ClaimsFromContext(ctx context.Context) (contracts.RouterInvocationCredentialClaimsV1, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(contracts.RouterInvocationCredentialClaimsV1)
	return claims, ok
}
