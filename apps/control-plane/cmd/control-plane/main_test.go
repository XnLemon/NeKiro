package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestRunAcceptsOnlyMigrateUpDirection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, arguments := range [][]string{
		{"migrate"},
		{"migrate", "down"},
		{"migrate", "sideways"},
		{"migrate", "up", "extra"},
	} {
		err := run(context.Background(), arguments, logger)
		if err == nil || !strings.Contains(err.Error(), "migrate requires exactly one direction: up") {
			t.Fatalf("run(%q) error = %v", arguments, err)
		}
	}
}
