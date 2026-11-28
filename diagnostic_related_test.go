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
	plain := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(d, SemanticColors{}, resolveDiagnosticIcons(DiagnosticIcons{}, true), SyntaxTheme{}, DetailNormal)), "")
	for _, want := range []string{"◇ 3 candidate signatures", "ƒ format(value: string): string", "⌖ src/service.ts:1:1", "ⓘ Formats a value."} {
		if !strings.Contains(plain, want) { t.Fatalf("missing %q in %q", want, plain) }
	}
}


func TestRelatedDetailModes(t *testing.T) {
	d := &Diagnostic{
		Tool: TypeScript, Level: Error, Code: "TS2322", File: "src/a.ts", Line: 1, Column: 1,
		Message: "Type 'string' is not assignable to type 'UserId'.",
		Related: []*Diagnostic{
			{File:"src/types.ts", Line:3, Column:1, Message:"Expected type declared here", Signature:"type UserId = Brand<string, \"UserId\">", Documentation:"Primary identifier."},
			{File:"src/a.ts", Line:1, Column:7, Message:"Symbol declared here", Signature:"id: string"},
			{File:"src/types.ts", Line:1, Column:1, Message:"Compiler related information"},
		},
	}
	icons := resolveDiagnosticIcons(DiagnosticIcons{}, true)

	compact := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(d, SemanticColors{}, icons, SyntaxTheme{}, DetailCompact)), "")
	if !strings.Contains(compact, "Expected type") || strings.Contains(compact, "Primary identifier.") || !strings.Contains(compact, "--detail=verbose") {
		t.Fatalf("unexpected compact output: %q", compact)
	}

	normal := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(d, SemanticColors{}, icons, SyntaxTheme{}, DetailNormal)), "")
	if !strings.Contains(normal, "Primary identifier.") || !strings.Contains(normal, "Compiler related information") {
		t.Fatalf("unexpected normal output: %q", normal)
	}

	verbose := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(d, SemanticColors{}, icons, SyntaxTheme{}, DetailVerbose)), "")
	if strings.Contains(verbose, "--detail=verbose") || !strings.Contains(verbose, "id: string") {
		t.Fatalf("unexpected verbose output: %q", verbose)
	}
}


func TestSignatureFocusHighlightsExpectedParameter(t *testing.T) {
	d := &Diagnostic{
		Tool: TypeScript, Level: Error, Code: "TS2345", File: "src/call.ts", Line: 4, Column: 9,
		Message: "Argument of type 'number' is not assignable to parameter of type 'UserId'.",
		Related: []*Diagnostic{{
			File:"src/service.ts", Line:10, Column:1, Message:"Defined here (resolved or recovery signature)",
			Signature:"(repository: UserRepository, id: UserId): Promise<User>",
			SignatureFocus:"id: UserId",
			SignatureFocusLabel:"argument 2 expects this parameter",
		}},
	}
	plain := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(
		d, SemanticColors{}, resolveDiagnosticIcons(DiagnosticIcons{}, true), SyntaxTheme{}, DetailNormal,
	)), "")
	for _, want := range []string{"id: UserId", "↳ argument 2 expects this parameter"} {
		if !strings.Contains(plain, want) { t.Fatalf("missing %q in %q", want, plain) }
	}
}


func TestSignatureFocusUsesExactOffsets(t *testing.T) {
	d := &Diagnostic{
		Tool: TypeScript, Level: Error, Code: "TS2322", Message: "bad return",
		Related: []*Diagnostic{{
			File:"src/service.ts", Line:1, Column:1, Message:"Return type declared here",
			Signature:"(value: string): string",
			SignatureFocus:"string", SignatureFocusStart:17, SignatureFocusEnd:23,
			SignatureFocusLabel:"function returns this type",
		}},
	}
	plain := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(
		d, SemanticColors{}, resolveDiagnosticIcons(DiagnosticIcons{}, true), SyntaxTheme{}, DetailNormal,
	)), "")
	if !strings.Contains(plain, "◆ Return type") || !strings.Contains(plain, "↳ function returns this type") {
		t.Fatalf("unexpected focused return output: %q", plain)
	}
}

func TestExpectedPropertyFocusLabel(t *testing.T) {
	d := &Diagnostic{
		Tool: TypeScript, Level: Error, Code: "TS2322", Message:"bad property",
		Related: []*Diagnostic{{
			File:"src/types.ts", Line:5, Column:3, Message:"Expected property declared here",
			Signature:"id: UserId", SignatureFocus:"id: UserId",
			SignatureFocusLabel:"property 'id' expects this type",
		}},
	}
	plain := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(
		d, SemanticColors{}, resolveDiagnosticIcons(DiagnosticIcons{}, true), SyntaxTheme{}, DetailNormal,
	)), "")
	if !strings.Contains(plain, "◆ Expected property") || !strings.Contains(plain, "property 'id' expects this type") {
		t.Fatalf("unexpected property focus output: %q", plain)
	}
}


func TestQuickFixPreviewRendering(t *testing.T) {
	d := &Diagnostic{
		Tool: TypeScript, Level: Error, Code: "TS2551", File: "src/fix.ts", Line: 3, Column: 15,
		Message: "Property 'toUppercase' does not exist on type 'string'. Did you mean 'toUpperCase'?",
		Fixes: []DiagnosticFix{{
			Description: "Change spelling to 'toUpperCase'",
			FixName: "spelling",
			Changes: []DiagnosticTextChange{{
				File:"src/fix.ts", Line:3, Column:15,
				OldText:"toUppercase", NewText:"toUpperCase",
			}},
		}},
	}
	plain := ansiPattern.ReplaceAllString(string(renderDiagnosticWithDetail(
		d, SemanticColors{}, resolveDiagnosticIcons(DiagnosticIcons{}, true), SyntaxTheme{}, DetailNormal,
	)), "")
	for _, want := range []string{
		"✦ Quick fix: Change spelling to 'toUpperCase'",
		"⌖ src/fix.ts:3:15",
		"- toUppercase",
		"+ toUpperCase",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing quick-fix preview %q in %q", want, plain)
		}
	}
}
