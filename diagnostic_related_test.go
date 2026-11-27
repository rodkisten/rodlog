package rodlog

import (
	"strings"
	"testing"
)

func TestTypeScriptRelatedSectionsUseSemanticSymbols(t *testing.T) {
	d := &Diagnostic{
		Tool: TypeScript,
		Level: Error,
		Code: "TS2769",
		File: "testdata/src/call.ts",
		DisplayFile: "src/call.ts",
		Line: 10,
		Column: 3,
		Message: "No overload matches this call.",
		Related: []*Diagnostic{
			{File:"testdata/src/service.ts", DisplayFile:"src/service.ts", Line:1, Column:1, Message:"Symbol declared here", Signature:"format(value: string): string", Documentation:"Formats a value."},
			{File:"testdata/src/service.ts", DisplayFile:"src/service.ts", Line:2, Column:1, Message:"Symbol declared here", Signature:"format(value: number, precision: number): string"},
			{File:"testdata/src/service.ts", DisplayFile:"src/service.ts", Line:3, Column:1, Message:"Symbol declared here", Signature:"format(value: Date, locale?: string): string"},
		},
	}
	plain := ansiPattern.ReplaceAllString(string(renderDiagnosticWithTheme(d, SemanticColors{}, resolveDiagnosticIcons(DiagnosticIcons{}, true))), "")
	for _, want := range []string{"◇ 3 candidate signatures", "ƒ format(value: string): string", "⌖ src/service.ts:1:1", "ⓘ Formats a value."} {
		if !strings.Contains(plain, want) { t.Fatalf("missing %q in %q", want, plain) }
	}
}
