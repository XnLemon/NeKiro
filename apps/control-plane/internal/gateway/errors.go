package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

const TraceHeader = "x-nek-trace-id"

func platformErrorStatus(code contracts.PlatformErrorCode) (int, error) {
	switch code {
	case contracts.ErrorCodeValidationError:
		return http.StatusBadRequest, nil
	case contracts.ErrorCodeUnauthenticated:
		return http.StatusUnauthorized, nil
	case contracts.ErrorCodeForbidden:
		return http.StatusForbidden, nil
	case contracts.ErrorCodeNotFound:
		return http.StatusNotFound, nil
	case contracts.ErrorCodeConflict:
		return http.StatusConflict, nil
	case contracts.ErrorCodeDependency:
		return http.StatusServiceUnavailable, nil
	case contracts.ErrorCodeInternal:
		return http.StatusInternalServerError, nil
	default:
		return 0, fmt.Errorf("unsupported Catalog error code %q", code)
	}
}

func writePlatformError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.PlatformErrorCode) error {
	status, err := platformErrorStatus(code)
	if err != nil {
		return err
	}
	payload, err := contracts.NewPlatformError(code, traceID)
	if err != nil {
		return fmt.Errorf("construct Platform Error: %w", err)
	}
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		return fmt.Errorf("encode Platform Error: %w", err)
	}
	return nil
}
