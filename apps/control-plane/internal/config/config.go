package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const DevelopmentStaticAuthMode = "development-static"

type StaticPrincipal struct {
	ID          string `json:"id"`
	TokenSHA256 string `json:"tokenSha256"`
}

type Config struct {
	DatabaseURL        string
	ListenAddress      string
	AuthMode           string
	Principals         []StaticPrincipal
	InternalAuthMode   string
	InternalPrincipals []StaticPrincipal
}

type jsonFrame struct {
	object    bool
	expecting bool
	members   map[string]struct{}
}

func Load() (Config, error) {
	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return Config{}, err
	}

	listenAddress, err := requiredEnv("NEKIRO_LISTEN_ADDRESS")
	if err != nil {
		return Config{}, err
	}
	if err := validateListenAddress(listenAddress); err != nil {
		return Config{}, fmt.Errorf("NEKIRO_LISTEN_ADDRESS is invalid: %w", err)
	}

	authMode, err := requiredEnv("NEKIRO_AUTH_MODE")
	if err != nil {
		return Config{}, err
	}
	if authMode != DevelopmentStaticAuthMode {
		return Config{}, fmt.Errorf("NEKIRO_AUTH_MODE %q is unsupported", authMode)
	}

	principalsJSON, err := requiredEnv("NEKIRO_DEV_AUTH_PRINCIPALS_JSON")
	if err != nil {
		return Config{}, err
	}
	principals, err := decodePrincipals([]byte(principalsJSON))
	if err != nil {
		return Config{}, fmt.Errorf("NEKIRO_DEV_AUTH_PRINCIPALS_JSON is invalid: %w", err)
	}
	internalAuthMode, err := requiredEnv("NEKIRO_INTERNAL_AUTH_MODE")
	if err != nil {
		return Config{}, err
	}
	if internalAuthMode != DevelopmentStaticAuthMode {
		return Config{}, fmt.Errorf("NEKIRO_INTERNAL_AUTH_MODE %q is unsupported", internalAuthMode)
	}
	internalPrincipalsJSON, err := requiredEnv("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON")
	if err != nil {
		return Config{}, err
	}
	internalPrincipals, err := decodePrincipals([]byte(internalPrincipalsJSON))
	if err != nil {
		return Config{}, fmt.Errorf("NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON is invalid: %w", err)
	}

	return Config{
		DatabaseURL: databaseURL, ListenAddress: listenAddress, AuthMode: authMode,
		Principals: principals, InternalAuthMode: internalAuthMode, InternalPrincipals: internalPrincipals,
	}, nil
}

func LoadDatabaseURL() (string, error) {
	databaseURL, err := requiredEnv("NEKIRO_DATABASE_URL")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" || parsed.Hostname() == "" || parsed.Port() == "" || parsed.Path == "" || parsed.Path == "/" || parsed.User == nil || parsed.User.Username() == "" {
		return "", errors.New("NEKIRO_DATABASE_URL is invalid")
	}
	password, hasPassword := parsed.User.Password()
	if !hasPassword || password == "" || parsed.Query().Get("sslmode") == "" {
		return "", errors.New("NEKIRO_DATABASE_URL is invalid")
	}
	if _, err := pgxpool.ParseConfig(databaseURL); err != nil {
		return "", errors.New("NEKIRO_DATABASE_URL is invalid")
	}
	return databaseURL, nil
}

func requiredEnv(name string) (string, error) {
	value, exists := os.LookupEnv(name)
	if !exists {
		return "", fmt.Errorf("%s is required", name)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be blank", name)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	return value, nil
}

func validateListenAddress(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return errors.New("must be a host and port")
	}
	if host == "" {
		return errors.New("host is required")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

func decodePrincipals(data []byte) ([]StaticPrincipal, error) {
	if err := rejectDuplicateMembers(data); err != nil {
		return nil, err
	}
	var principals []StaticPrincipal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&principals); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	if len(principals) == 0 {
		return nil, errors.New("at least one principal is required")
	}

	ids := make(map[string]struct{}, len(principals))
	digests := make(map[string]struct{}, len(principals))
	for _, principal := range principals {
		if !validIdentifier(principal.ID) {
			return nil, errors.New("principal id is invalid")
		}
		decoded, err := hex.DecodeString(principal.TokenSHA256)
		if err != nil || len(decoded) != 32 || principal.TokenSHA256 != strings.ToLower(principal.TokenSHA256) {
			return nil, errors.New("principal tokenSha256 must be 64 lowercase hexadecimal characters")
		}
		if _, exists := ids[principal.ID]; exists {
			return nil, errors.New("principal id is duplicated")
		}
		if _, exists := digests[principal.TokenSHA256]; exists {
			return nil, errors.New("principal tokenSha256 is duplicated")
		}
		ids[principal.ID] = struct{}{}
		digests[principal.TokenSHA256] = struct{}{}
	}
	return principals, nil
}

func validIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i, character := range []byte(value) {
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == ':' || character == '-' {
			if i > 0 || character != '.' && character != '_' && character != ':' && character != '-' {
				continue
			}
		}
		return false
	}
	return true
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func rejectDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var stack []jsonFrame
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, jsonFrame{object: true, expecting: true, members: map[string]struct{}{}})
			case '[':
				stack = append(stack, jsonFrame{})
			case '}', ']':
				stack = stack[:len(stack)-1]
				markValueConsumed(stack)
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expecting {
				current := &stack[len(stack)-1]
				if _, exists := current.members[value]; exists {
					return fmt.Errorf("duplicate member %q", value)
				}
				current.members[value] = struct{}{}
				current.expecting = false
			} else {
				markValueConsumed(stack)
			}
		default:
			markValueConsumed(stack)
		}
	}
}

func markValueConsumed(stack []jsonFrame) {
	if len(stack) > 0 && stack[len(stack)-1].object {
		stack[len(stack)-1].expecting = true
	}
}
