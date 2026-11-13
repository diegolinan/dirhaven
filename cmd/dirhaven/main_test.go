package main

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestCliBasics(t *testing.T) {
	tests := []struct {
		args     []string
		ok       bool
		contains string
	}{
		{args: []string{"help"}, ok: true, contains: "Dirhaven"},
		{args: []string{"version"}, ok: true, contains: "dirhaven 0.1.0-dev"},
		{args: nil, ok: false, contains: "subcommand required"},
		{args: []string{"serve"}, ok: false, contains: "at least one --allow"},
		{args: []string{"serve", "--allow", "relative-path-that-does-not-exist"}, ok: false, contains: "relative-path-that-does-not-exist"},
		{args: []string{"serve", "--unknown"}, ok: false, contains: "flag provided but not defined"},
		{args: []string{"serve", "--allow", "x", "unexpected"}, ok: false, contains: "unexpected positional"},
		{args: []string{"totally-unknown"}, ok: false, contains: "unknown subcommand"},
	}
	for _, tc := range tests {
		var out, errs bytes.Buffer
		err := run(context.Background(), tc.args, &out, &errs)
		if (err == nil) != tc.ok {
			t.Errorf("%v: err=%v, expected success=%t", tc.args, err, tc.ok)
		}
		observed := out.String() + errs.String()
		if err != nil {
			observed += err.Error()
		}
		if !strings.Contains(observed, tc.contains) {
			t.Errorf("%v: output %q did not contain %q", tc.args, observed, tc.contains)
		}
	}
}

func TestWildcardNeverAccidentallyRunsOnPosix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports explicit wildcard by design")
	}
	var out, errs bytes.Buffer
	err := run(context.Background(), []string{"serve", "--allow-all-drives"}, &out, &errs)
	if err == nil || !strings.Contains(err.Error(), "supported only on Windows") {
		t.Fatalf("expected rejection for unsupported platform: %v", err)
	}
}
