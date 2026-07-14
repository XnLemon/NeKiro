package workspace

import (
	"encoding/base64"
	"testing"
)

func TestDecodeInstallationCursorRejectsDuplicateMembers(t *testing.T) {
	data := []byte(`{"version":1,"workspaceId":"workspace-a","workspaceId":"workspace-a","limit":1,"installedAt":"2026-07-14T12:00:00Z","installationId":"installation-a"}`)
	cursor := base64.RawURLEncoding.EncodeToString(data)
	if _, err := DecodeInstallationCursor(cursor, "workspace-a", 1); err == nil {
		t.Fatal("cursor with duplicate members was accepted")
	}
}
