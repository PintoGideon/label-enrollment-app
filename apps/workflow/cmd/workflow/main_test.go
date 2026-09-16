package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestOperationalCommandsFailSafely(t *testing.T) {
	for _, args := range [][]string{{"migrate"}, {"synthetic-secret-argument"}, {"serve", "extra"}} {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		if run(args, func(string) string { return "" }, logger) == 0 {
			t.Fatal("missing database or invalid command should fail")
		}
		if strings.Contains(logs.String(), "synthetic-secret-argument") {
			t.Fatal("unknown command echoed user input")
		}
	}
}
