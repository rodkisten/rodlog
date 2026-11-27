package rodlog

import (
	"strings"
	"testing"
)

func TestStructuredDiagnosticSpans(t *testing.T) {
	d := &Diagnostic{Tool: TypeScript, Level: Error, Code: "TS2322", Message: "mismatched types",
		Spans: []DiagnosticSpan{{File: "src/a.ts", Line: 4, Column: 7, EndColumn: 10, Source: "const abc = 1", Primary: true, Label: "expected string"}},
		Notes: []string{"type annotation"}, Help: []string{"use a string literal"}}
	got := ansiPattern.ReplaceAllString(string(renderDiagnostic(d)), "")
	for _, want := range []string{"src/a.ts:4:7", "const abc = 1", "^^^", "expected string", "note:", "help:"} {
		if !strings.Contains(got, want) { t.Errorf("missing %q in %q", want, got) }
	}
}
