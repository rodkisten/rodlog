package rodlog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	maxDiagnosticLineNumber = 100_000
	defaultDiagBufferBytes  = 512
)

// Package-level compiled regexes for extracting structured facts from known compiler messages.
// Compiled once at startup rather than on every diagnostic render.
var (
	// Matches TS2322 / TS2345 type mismatch pairs:
	// e.g. "Type 'string' is not assignable to type 'number'."
	tsTypePair = regexp.MustCompile(`(?i)(?:argument of )?type '([^']+)' is not assignable to (?:type|parameter of type) '([^']+)'`)

	// Matches TS2339 / TS2551 missing property errors (with optional "Did you mean" hint):
	// e.g. "Property 'foo' does not exist on type 'Bar'. Did you mean 'for'?"
	tsMissingProp = regexp.MustCompile(`(?i)Property '([^']+)' does not exist on type '([^']+)'(?:\.\s*Did you mean '([^']+)')?`)

	// Matches TS2741 / TS2739 missing required object properties:
	// e.g. "Property 'id' is missing in type '{ name: string; }' but required in type 'User'."
	tsMissingRequired = regexp.MustCompile(`(?i)Property '([^']+)' is missing in type '([^']+)' but required in type '([^']+)'`)

	// Matches TS2554 / TS2555 argument count mismatches:
	// e.g. "Expected 2 arguments, but got 1."
	tsArgCount = regexp.MustCompile(`(?i)Expected (\d+(?:-\d+)?) arguments?, but got (\d+)`)
)

// diagnosticExplanation returns a short, deterministic interpretation for well-known
// diagnostic codes without inventing speculative fixes. The tool's original message
// remains visible and authoritative.
func diagnosticExplanation(d *Diagnostic) string {
	// Guard against nil pointer dereference; in Go, pointer receivers/arguments can be nil.
	if d == nil {
		return ""
	}

	if d.Tool == TypeScript {
		return explainTypeScript(d)
	}

	// Fallback explanations for common Node/POSIX system error codes when present.
	switch d.Code {
	case "ENOENT":
		return "File or directory does not exist at the target path."
	case "EADDRINUSE":
		return "Port is already bound by another running process."
	case "ECONNREFUSED":
		return "Target service refused the connection; verify it is running and listening."
	}

	return ""
}

// explainTypeScript maps high-frequency TypeScript compiler codes (TSxxxx) to concise
// domain-specific explanations.
func explainTypeScript(d *Diagnostic) string {
	switch d.Code {
	case "TS2322", "TS2345":
		if m := tsTypePair.FindStringSubmatch(d.Message); m != nil {
			return fmt.Sprintf("Expected: %s  |  Received: %s", m[2], m[1])
		}

	case "TS2339", "TS2551":
		if m := tsMissingProp.FindStringSubmatch(d.Message); m != nil {
			if m[3] != "" {
				return fmt.Sprintf("'%s' does not exist on '%s' (did you mean '%s'?)", m[1], m[2], m[3])
			}
			return fmt.Sprintf("Property '%s' is not declared on type '%s'.", m[1], m[2])
		}

	case "TS2741", "TS2739":
		if m := tsMissingRequired.FindStringSubmatch(d.Message); m != nil {
			return fmt.Sprintf("Missing required property '%s' from '%s' on '%s'.", m[1], m[3], m[2])
		}

	case "TS2554", "TS2555":
		if m := tsArgCount.FindStringSubmatch(d.Message); m != nil {
			return fmt.Sprintf("Expected %s argument(s), received %s.", m[1], m[2])
		}

	case "TS2304":
		return "Symbol is not in scope. Check spelling, imports, or ambient @types packages."

	case "TS2307":
		return "Module could not be resolved. Check the import path, installed dependencies, and tsconfig moduleResolution."

	case "TS2349":
		return "Expression is not callable. Check if the value is a function or needs a property access."

	case "TS7006":
		return "Parameter has an implicit 'any' type. Add an explicit type annotation or improve inference."

	case "TS18046":
		return "Value is of type 'unknown' (common in catch blocks). Narrow it with instanceof or a type guard first."

	case "TS18048":
		return "Value is possibly 'undefined'. Narrow the type or use optional chaining before accessing it."

	case "TS18047":
		return "Value is possibly 'null'. Guard against null before accessing properties."
	}

	return ""
}

// renderDiagnostic formats a structured error with a clear severity, location,
// diagnostic code, and message using default theme colors and icons.
// Only transform mode uses this presentation.
func renderDiagnostic(d *Diagnostic) []byte {
	return renderDiagnosticWithTheme(d, SemanticColors{}, DiagnosticIcons{})
}

// renderDiagnosticWithTheme renders a Diagnostic into a styled byte slice using
// caller-supplied semantic colors and icons.
func renderDiagnosticWithTheme(d *Diagnostic, colors SemanticColors, icons DiagnosticIcons) []byte {
	if d == nil {
		return nil
	}

	heading, levelColor := resolveDiagnosticHeader(d.Level, colors, icons)
	location := formatDiagnosticLocation(d)

	codeBadge := ""
	if d.Code != "" {
		codeBadge = " [" + d.Code + "]"
	}

	var out strings.Builder
	// Pre-allocate buffer capacity to minimize slice growth reallocations.
	out.Grow(defaultDiagBufferBytes + len(d.Message))

	// Header line: e.g. "✖ ERROR  src/index.ts:12:4 [TS2322]"
	fmt.Fprintf(&out, "%s  %s%s\n",
		wrapANSI("1;"+levelColor, heading),
		wrapANSI(semanticColor(colors.Location, "2"), location),
		codeBadge,
	)

	// Indented primary diagnostic message (preserving multi-line compiler output).
	message := strings.TrimSpace(d.Message)
	if message != "" {
		for _, line := range strings.Split(message, "\n") {
			out.WriteString("  ")
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}

	// Source code snippet with caret pointer when valid local file coordinates exist.
	if d.File != "" && d.Line > 0 && d.Line <= maxDiagnosticLineNumber {
		if frame := diagnosticFrame(d); frame != "" {
			out.WriteString(frame)
		}
	}

	// Explicit Expected/Received diff for test runner assertion failures.
	if d.Tool == Vitest || d.Tool == NodeTest {
		if m := runnerAssertion.FindStringSubmatch(message); m != nil {
			// Keep both sides explicit instead of relying on terminal color alone.
			successColor := semanticColor(colors.Success, "32")
			errorColor := semanticColor(colors.Error, "31")
			fmt.Fprintf(&out, "  %s\n  %s\n",
				wrapANSI(successColor, "Expected: "+m[2]),
				wrapANSI(errorColor, "Received: "+m[1]),
			)
		}
	}

	infoColor := semanticColor(colors.Info, "36")

	// Domain-specific explanation (e.g. decoded TypeScript type mismatch).
	if explanation := diagnosticExplanation(d); explanation != "" {
		prefix := iconOrDefault(icons.Explanation, "↳")
		fmt.Fprintf(&out, "  %s\n", wrapANSI(infoColor, prefix+" "+explanation))
	}

	// Actionable next-step suggestion if available.
	if suggestion := diagnosticSuggestion(d); suggestion != "" {
		prefix := iconOrDefault(icons.Suggestion, "💡")
		fmt.Fprintf(&out, "  %s\n", wrapANSI(infoColor, prefix+" "+suggestion))
	}

	// Convert builder contents to []byte. In Go, converting string -> []byte copies
	// the underlying bytes once so the caller owns an independent mutable slice.
	return []byte(out.String())
}

// resolveDiagnosticHeader maps a severity Level to its display label, icon, and SGR color.
func resolveDiagnosticHeader(level Level, colors SemanticColors, icons DiagnosticIcons) (heading, color string) {
	switch level {
	case Warning:
		return iconOrDefault(icons.Warning, "⚠") + " WARNING", semanticColor(colors.Warning, "33")
	case Info:
		return iconOrDefault(icons.Info, "ℹ") + " INFO", semanticColor(colors.Info, "36")
	default:
		return iconOrDefault(icons.Error, "✖") + " ERROR", semanticColor(colors.Error, "31")
	}
}

// formatDiagnosticLocation formats "file:line:column", falling back to the tool name
// when no file path is associated with the diagnostic.
func formatDiagnosticLocation(d *Diagnostic) string {
	if d.File == "" {
		return string(d.Tool)
	}

	var b strings.Builder
	b.WriteString(d.File)
	if d.Line > 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(d.Line))
		if d.Column > 0 {
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(d.Column))
		}
	}
	return b.String()
}

// wrapANSI applies an SGR sequence when terminal color output is enabled.
func wrapANSI(sgrCode, text string) string {
	if sgrCode == "" || lipgloss.ColorProfile() == 0 {
		return text
	}
	return "\x1b[" + sgrCode + "m" + text + "\x1b[0m"
}
