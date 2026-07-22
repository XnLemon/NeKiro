package challengeproof

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	DirectoryEnvironment   = "NEKIRO_AGENT_CHALLENGE_DIRECTORY"
	challengePathPrefix    = "/.well-known/nekiro/challenges/"
	maximumProofCharacters = 128
	maximumProofBytes      = utf8.UTFMax * maximumProofCharacters
)

var challengeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type handler struct {
	agent     http.Handler
	directory string
}

// NewHandler adds the provider-owned HTTP ownership proof route to a sample
// Agent. The proof directory is required deployment configuration; it has no
// default because choosing a storage target belongs to the Agent operator.
func NewHandler(agent http.Handler, lookup func(string) (string, bool)) (http.Handler, error) {
	if agent == nil || lookup == nil {
		return nil, errors.New("challenge proof handler dependencies are required")
	}
	directory, exists := lookup(DirectoryEnvironment)
	if !exists {
		return nil, fmt.Errorf("%s is required", DirectoryEnvironment)
	}
	if directory == "" || strings.TrimSpace(directory) != directory {
		return nil, fmt.Errorf("%s must be non-empty and contain no surrounding whitespace", DirectoryEnvironment)
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, fmt.Errorf("%s must be a clean absolute path", DirectoryEnvironment)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create challenge proof directory: %w", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("inspect challenge proof directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("challenge proof path must be a directory")
	}
	return &handler{agent: agent, directory: directory}, nil
}

func (value *handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.URL.Path, challengePathPrefix) {
		value.agent.ServeHTTP(writer, request)
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	challengeID := strings.TrimPrefix(request.URL.Path, challengePathPrefix)
	if !challengeIDPattern.MatchString(challengeID) {
		http.NotFound(writer, request)
		return
	}
	file, err := os.Open(filepath.Join(value.directory, challengeID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(writer, request)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	proof, err := io.ReadAll(io.LimitReader(file, maximumProofBytes+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || !validProof(proof) {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = writer.Write(proof)
}

func validProof(value []byte) bool {
	if len(value) == 0 || len(value) > maximumProofBytes || !utf8.Valid(value) {
		return false
	}
	return utf8.RuneCount(value) <= maximumProofCharacters
}
