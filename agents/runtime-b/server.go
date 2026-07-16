package runtimeb

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/a2aproject/a2a-go/a2asrv"
)

const ListenAddressEnvironment = "RUNTIME_B_LISTEN_ADDR"

func NewHTTPHandler(handler *Handler) http.Handler {
	jsonRPCHandler := a2asrv.NewJSONRPCHandler(handler)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		jsonRPCHandler.ServeHTTP(writer, request)
	})
}

func ListenAddressFromEnvironment(lookup func(string) (string, bool)) (string, error) {
	address, exists := lookup(ListenAddressEnvironment)
	if !exists {
		return "", fmt.Errorf("%s is required", ListenAddressEnvironment)
	}
	if address == "" || strings.TrimSpace(address) != address {
		return "", fmt.Errorf("%s must be non-empty and contain no surrounding whitespace", ListenAddressEnvironment)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("%s must be a host:port TCP address: %w", ListenAddressEnvironment, err)
	}
	if host == "" {
		return "", fmt.Errorf("%s must declare a host", ListenAddressEnvironment)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%s port must be an integer from 1 through 65535", ListenAddressEnvironment)
	}
	return address, nil
}
