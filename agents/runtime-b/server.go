package runtimeb

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nene7ko/NeKiro/sdks/agent-sdk/routerauth"
	"github.com/a2aproject/a2a-go/a2asrv"
)

const ListenAddressEnvironment = "RUNTIME_B_LISTEN_ADDR"

func NewHTTPHandlerWithAuth(handler *Handler, authenticationConfig routerauth.Config) (http.Handler, error) {
	if handler == nil {
		return nil, fmt.Errorf("runtime-b handler is required")
	}
	authentication, err := routerauth.NewMiddleware(authenticationConfig, time.Now)
	if err != nil {
		return nil, err
	}
	jsonRPCHandler := a2asrv.NewJSONRPCHandler(handler)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.Handle("/unavailable", authentication.Handler(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	})))
	mux.Handle("/", authentication.Handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		jsonRPCHandler.ServeHTTP(writer, request)
	})))
	return mux, nil
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
