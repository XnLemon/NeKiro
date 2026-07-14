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
	t.Setenv("NEKIRO_AUTH_MODE", DevelopmentStaticAuthMode)
	t.Setenv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"owner-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
	t.Setenv("NEKIRO_INTERNAL_AUTH_MODE", DevelopmentStaticAuthMode)
	t.Setenv("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"router-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if loaded.ListenAddress != "127.0.0.1:18080" || len(loaded.Principals) != 1 {
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
	t.Setenv("NEKIRO_AUTH_MODE", DevelopmentStaticAuthMode)
	t.Setenv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON", `[{"id":"owner-a","tokenSha256":"`+hex.EncodeToString(digest[:])+`"}]`)
	t.Setenv("NEKIRO_INTERNAL_AUTH_MODE", "")
	t.Setenv("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing internal authentication configuration was accepted")
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
