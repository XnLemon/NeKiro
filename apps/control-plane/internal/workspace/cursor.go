package workspace

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const installationCursorVersion = 1

type installationCursor struct {
	Version        int       `json:"version"`
	WorkspaceID    string    `json:"workspaceId"`
	Limit          int       `json:"limit"`
	InstalledAt    time.Time `json:"installedAt"`
	InstallationID string    `json:"installationId"`
}

func EncodeInstallationCursor(workspaceID string, limit int, position InstallationPosition) (string, error) {
	if !ValidIdentifier(workspaceID) || limit < 1 || limit > 100 || !ValidIdentifier(position.InstallationID) || position.InstalledAt.IsZero() {
		return "", ErrInvalid
	}
	payload, err := json.Marshal(installationCursor{
		Version: installationCursorVersion, WorkspaceID: workspaceID, Limit: limit,
		InstalledAt: position.InstalledAt.UTC(), InstallationID: position.InstallationID,
	})
	if err != nil {
		return "", fmt.Errorf("encode installation cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeInstallationCursor(value, workspaceID string, limit int) (InstallationPosition, error) {
	if value == "" || !ValidIdentifier(workspaceID) || limit < 1 || limit > 100 {
		return InstallationPosition{}, ErrInvalid
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return InstallationPosition{}, ErrInvalid
	}
	if err := rejectDuplicateInstallationCursorMembers(data); err != nil {
		return InstallationPosition{}, ErrInvalid
	}
	var payload installationCursor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return InstallationPosition{}, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return InstallationPosition{}, ErrInvalid
	}
	if payload.Version != installationCursorVersion || payload.WorkspaceID != workspaceID || payload.Limit != limit ||
		!ValidIdentifier(payload.InstallationID) || payload.InstalledAt.IsZero() {
		return InstallationPosition{}, ErrInvalid
	}
	return InstallationPosition{InstalledAt: payload.InstalledAt, InstallationID: payload.InstallationID}, nil
}

func rejectDuplicateInstallationCursorMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("installation cursor must be an object")
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok {
			return errors.New("installation cursor member name is invalid")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate installation cursor member %q", name)
		}
		seen[name] = struct{}{}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return errors.New("installation cursor object is incomplete")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("installation cursor has trailing data")
	}
	return nil
}
