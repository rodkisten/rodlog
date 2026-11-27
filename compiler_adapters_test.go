package rodlog

import (
	"os"
	"testing"
)

func TestCompilerAdapters(t *testing.T) {
	raw, err := os.ReadFile("testdata/rust.jsonl")
	if err != nil { t.Fatal(err) }
	d := ParseDiagnostic(Auto, string(raw))
	if d == nil || d.Code != "E0308" || len(d.Spans) != 1 || len(d.Help) != 1 || len(d.Notes) != 1 { t.Fatalf("invalid Rust diagnostic: %#v", d) }
	for _, tc := range []struct{ input string; tool Tool }{
		{"./main.go:12:8: undefined: name", GoCompiler},
		{"ERR_PNPM_OUTDATED_LOCKFILE Cannot install", PNPM},
	} {
		if d := ParseDiagnostic(Auto, tc.input); d == nil || d.Tool != tc.tool { t.Errorf("invalid diagnostic for %q: %#v", tc.input, d) }
	}
}
