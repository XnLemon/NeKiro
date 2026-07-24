package routerauth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

type verificationError struct {
	status int
	cause  error
}

func (failure *verificationError) Error() string {
	return http.StatusText(failure.status)
}

func (failure *verificationError) Unwrap() error {
	return failure.cause
}

func unauthenticated(cause error) error {
	return &verificationError{status: http.StatusUnauthorized, cause: cause}
}

func forbidden(cause error) error {
	return &verificationError{status: http.StatusForbidden, cause: cause}
}

func writeVerificationError(writer http.ResponseWriter, err error) {
	var failure *verificationError
	if !errors.As(err, &failure) {
		panic("routerauth verifier returned an unclassified error")
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	var code contracts.PlatformErrorCode
	switch failure.status {
	case http.StatusUnauthorized:
		code = contracts.ErrorCodeUnauthenticated
		writer.Header().Set("WWW-Authenticate", "Bearer")
	case http.StatusForbidden:
		code = contracts.ErrorCodeForbidden
	default:
		panic("routerauth verifier returned an unsupported status")
	}
	payload, contractErr := contracts.NewRouterAgentAuthenticationErrorV1(code)
	if contractErr != nil {
		panic(contractErr)
	}
	encoded, encodeErr := json.Marshal(payload)
	if encodeErr != nil {
		panic(encodeErr)
	}
	writer.WriteHeader(failure.status)
	_, _ = writer.Write(encoded)
}
