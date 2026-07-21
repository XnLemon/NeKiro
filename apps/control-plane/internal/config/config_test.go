package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestLoadRequiresExplicitValidConfiguration(t *testing.T) {
	digest := sha256.Sum256([]byte("token"))
	t.Setenv("NEKIRO_DATABASE_URL", "postgresql://user:password@127.0.0.1:5432/catalog_test?sslmode=disable")
	t.Setenv("NEKIRO_LISTEN_ADDRESS", "127.0.0.1:18080")
	t.Setenv("NEKIRO_CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")
	t.Setenv("NEKIRO_AUTH_MODE", DevelopmentStaticAuthMode)
	t.Setenv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"owner-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
	t.Setenv("NEKIRO_INTERNAL_AUTH_MODE", DevelopmentStaticAuthMode)
	t.Setenv("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"router-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if loaded.ListenAddress != "127.0.0.1:18080" || len(loaded.Principals) != 1 || len(loaded.CORSAllowedOrigins) != 2 {
		t.Fatalf("loaded config = %#v", loaded)
	}
	if loaded.InternalAuthMode != DevelopmentStaticAuthMode || len(loaded.InternalPrincipals) != 1 {
		t.Fatalf("loaded internal auth config = %#v", loaded)
	}
}

func TestLoadRejectsMissingBlankAndMalformedConfiguration(t *testing.T) {
	digest := sha256.Sum256([]byte("token"))
	validDigest := hex.EncodeToString(digest[:])
	tests := []struct {
		name       string
		database   string
		listen     string
		mode       string
		principals string
	}{
		{name: "blank database", database: " ", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","tokenSha256":"` + validDigest + `"}]`},
		{name: "invalid listen", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","tokenSha256":"` + validDigest + `"}]`},
		{name: "unsupported auth", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: "anonymous", principals: `[{"id":"owner-a","tokenSha256":"` + validDigest + `"}]`},
		{name: "empty principals", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[]`},
		{name: "duplicate id", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","tokenSha256":"` + validDigest + `"},{"id":"owner-a","tokenSha256":"` + strings64("a") + `"}]`},
		{name: "duplicate digest", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","tokenSha256":"` + validDigest + `"},{"id":"owner-b","tokenSha256":"` + validDigest + `"}]`},
		{name: "short digest", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","tokenSha256":"aa"}]`},
		{name: "duplicate member", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","id":"owner-b","tokenSha256":"` + validDigest + `"}]`},
		{name: "uppercase digest", database: "postgresql://user:password@127.0.0.1:5432/catalog_test", listen: "127.0.0.1:18080", mode: DevelopmentStaticAuthMode, principals: `[{"id":"owner-a","tokenSha256":"` + strings64("A") + `"}]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("NEKIRO_DATABASE_URL", test.database)
			t.Setenv("NEKIRO_LISTEN_ADDRESS", test.listen)
			t.Setenv("NEKIRO_CORS_ALLOWED_ORIGINS", "http://localhost:3000")
			t.Setenv("NEKIRO_AUTH_MODE", test.mode)
			t.Setenv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON", test.principals)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestLoadRejectsMissingRequiredVariable(t *testing.T) {
	name := "NEKIRO_DATABASE_URL"
	previous, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		} else {
			_ = os.Unsetenv(name)
		}
	})
	if _, err := LoadDatabaseURL(); err == nil {
		t.Fatal("missing database URL was accepted")
	}
}

func TestLoadRejectsMissingInternalAuthenticationConfiguration(t *testing.T) {
	digest := sha256.Sum256([]byte("token"))
	t.Setenv("NEKIRO_DATABASE_URL", "postgresql://user:password@127.0.0.1:5432/catalog_test?sslmode=disable")
	t.Setenv("NEKIRO_LISTEN_ADDRESS", "127.0.0.1:18080")
	t.Setenv("NEKIRO_CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	t.Setenv("NEKIRO_AUTH_MODE", DevelopmentStaticAuthMode)
	t.Setenv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"owner-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
	t.Setenv("NEKIRO_INTERNAL_AUTH_MODE", "")
	t.Setenv("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing internal authentication configuration was accepted")
	}
}

func TestLoadRejectsImplicitOrMalformedCORSOrigins(t *testing.T) {
	digest := sha256.Sum256([]byte("token"))
	for _, origins := range []string{"*", " http://localhost:3000", "http://localhost:3000/", "http://localhost:3000?", "http://localhost:3000#", "http://localhost:3000:0", "http://localhost:99999", "http://localhost:3000,http://localhost:3000", "localhost:3000", "http://user:pass@localhost:3000"} {
		t.Run(origins, func(t *testing.T) {
			t.Setenv("NEKIRO_DATABASE_URL", "postgresql://user:password@127.0.0.1:5432/catalog_test?sslmode=disable")
			t.Setenv("NEKIRO_LISTEN_ADDRESS", "127.0.0.1:18080")
			t.Setenv("NEKIRO_CORS_ALLOWED_ORIGINS", origins)
			t.Setenv("NEKIRO_AUTH_MODE", DevelopmentStaticAuthMode)
			t.Setenv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"owner-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
			t.Setenv("NEKIRO_INTERNAL_AUTH_MODE", DevelopmentStaticAuthMode)
			t.Setenv("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"router-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
			if _, err := Load(); err == nil {
				t.Fatal("malformed CORS origins were accepted")
			}
		})
	}
}

func TestLoadDatabaseURLRejectsImplicitLibpqDefaults(t *testing.T) {
	for _, value := range []string{
		"host=127.0.0.1 dbname=catalog_test",
		"postgresql://user:password@127.0.0.1/catalog_test?sslmode=disable",
		"postgresql://user:password@127.0.0.1:5432/?sslmode=disable",
		"postgresql://127.0.0.1:5432/catalog_test?sslmode=disable",
		"postgresql://user@127.0.0.1:5432/catalog_test?sslmode=disable",
		"postgresql://user:password@127.0.0.1:5432/catalog_test",
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("NEKIRO_DATABASE_URL", value)
			if _, err := LoadDatabaseURL(); err == nil {
				t.Fatal("database configuration with implicit values was accepted")
			}
		})
	}
}

func strings64(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result
}

func TestLoadInvocationRuntimeRequiresExactNoDefaultConfiguration(t *testing.T) {
	setValidInvocationRuntime(t)
	loaded, err := LoadInvocationRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RouterInternalURL != "http://router.test:8081/internal/v3/invocations" || loaded.RouterBearerToken != "router-secret" || loaded.InternalRequestLimitBytes != 1048576 || loaded.PublicRequestLimitBytes != 1048576 || loaded.SSEEventLimitBytes != 65536 || loaded.MetadataResponseLimitBytes != 1048576 || loaded.DeadlineMS != 30000 {
		t.Fatalf("loaded invocation config = %#v", loaded)
	}
}

func TestLoadInvocationRuntimeRejectsInvalidDestinationSecretAndNumbers(t *testing.T) {
	tests := []struct{ name, variable, value string }{
		{"relative URL", "NEKIRO_ROUTER_INTERNAL_URL", "/internal/v3/invocations"},
		{"wrong path", "NEKIRO_ROUTER_INTERNAL_URL", "http://router.test:8081/internal/v2/invocations"},
		{"URL credentials", "NEKIRO_ROUTER_INTERNAL_URL", "http://user:secret@router.test:8081/internal/v3/invocations"},
		{"URL query", "NEKIRO_ROUTER_INTERNAL_URL", "http://router.test:8081/internal/v3/invocations?target=other"},
		{"blank token", "NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN", ""},
		{"token whitespace", "NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN", "secret token"},
		{"zero internal body", "NEKIRO_CONTROL_PLANE_INTERNAL_REQUEST_MAX_BYTES", "0"},
		{"zero body", "NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES", "0"},
		{"signed body", "NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES", "+1"},
		{"fraction SSE", "NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES", "1.5"},
		{"zero metadata response", "NEKIRO_GATEWAY_METADATA_RESPONSE_MAX_BYTES", "0"},
		{"exponent deadline", "NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS", "1e3"},
		{"too large deadline", "NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS", "600001"},
		{"overflow", "NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES", "999999999999999999999999"},
		{"surrounding whitespace", "NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES", " 10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidInvocationRuntime(t)
			t.Setenv(test.variable, test.value)
			if _, err := LoadInvocationRuntime(); err == nil {
				t.Fatal("invalid invocation runtime configuration was accepted")
			}
		})
	}
}

func TestLoadInvocationRuntimeRejectsEveryMissingVariable(t *testing.T) {
	for _, variable := range []string{"NEKIRO_ROUTER_INTERNAL_URL", "NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN", "NEKIRO_CONTROL_PLANE_INTERNAL_REQUEST_MAX_BYTES", "NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES", "NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES", "NEKIRO_GATEWAY_METADATA_RESPONSE_MAX_BYTES", "NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS"} {
		t.Run(variable, func(t *testing.T) {
			setValidInvocationRuntime(t)
			if err := os.Unsetenv(variable); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadInvocationRuntime(); err == nil {
				t.Fatal("missing invocation runtime configuration was accepted")
			}
		})
	}
}

func setValidInvocationRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("NEKIRO_ROUTER_INTERNAL_URL", "http://router.test:8081/internal/v3/invocations")
	t.Setenv("NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN", "router-secret")
	t.Setenv("NEKIRO_CONTROL_PLANE_INTERNAL_REQUEST_MAX_BYTES", "1048576")
	t.Setenv("NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES", "1048576")
	t.Setenv("NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES", "65536")
	t.Setenv("NEKIRO_GATEWAY_METADATA_RESPONSE_MAX_BYTES", "1048576")
	t.Setenv("NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS", "30000")
}
