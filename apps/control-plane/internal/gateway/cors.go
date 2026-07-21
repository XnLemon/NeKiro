package gateway

import (
	"net/http"
	"strings"
)

const (
	CORSAllowedMethods = "GET, POST, PATCH, DELETE, OPTIONS"
	CORSAllowedHeaders = "Authorization, Content-Type, Accept"
	CORSExposeHeaders  = TraceHeader
)

// CORS wraps only public Gateway routes. Internal Router-facing paths never
// receive a browser grant, even when the Origin is otherwise allowlisted.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		_, originAllowed := allowed[origin]
		publicRoute := strings.HasPrefix(request.URL.Path, "/v3/") || strings.HasPrefix(request.URL.Path, "/v4/")
		if origin != "" && originAllowed && publicRoute {
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Access-Control-Allow-Methods", CORSAllowedMethods)
			writer.Header().Set("Access-Control-Allow-Headers", CORSAllowedHeaders)
			writer.Header().Set("Access-Control-Expose-Headers", CORSExposeHeaders)
			if request.Method == http.MethodOptions {
				writer.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(writer, request)
	})
}
