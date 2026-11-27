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

var (
	tsTypePair        = regexp.MustCompile(`(?i)(?:argument of )?type '([^']+)' is not assignable to (?:type|parameter of type) '([^']+)'`)
	tsMissingProp     = regexp.MustCompile(`(?i)Property '([^']+)' does not exist on type '([^']+)'(?:\.\s*Did you mean '([^']+)')?`)
	tsMissingRequired = regexp.MustCompile(`(?i)Property '([^']+)' is missing in type '([^']+)' but required in type '([^']+)'`)
	tsArgCount        = regexp.MustCompile(`(?i)Expected (\d+(?:-\d+)?) arguments?, but got (\d+)`)
)

// diagnosticExplanation returns a short, deterministic interpretation for well-known
// diagnostic codes without inventing speculative fixes.
func diagnosticExplanation(d *Diagnostic) string {
	if d == nil {
		return ""
	}
	if d.Tool == TypeScript {
		return explainTypeScript(d)
	}
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
		return "Parameter has an implicit any type. Add an explicit type annotation or improve inference."
	case "TS18046":
		return "Value is of type 'unknown' (common in catch blocks). Narrow it with instanceof or a type guard first."
	case "TS18048":
		return "Value is possibly 'undefined'. Narrow the type or use optional chaining before accessing it."
	case "TS18047":
		return "Value is possibly 'null'. Guard against null before accessing properties."
	}
	return ""
}

func renderDiagnostic(d *Diagnostic) []byte {
	return renderDiagnosticWithTheme(d, SemanticColors{}, DiagnosticIcons{})
}

func renderDiagnosticWithTheme(d *Diagnostic, colors SemanticColors, icons DiagnosticIcons, themes ...SyntaxTheme) []byte {
	if d == nil {
		return nil
	}

	theme := SyntaxTheme{}
	if len(themes) > 0 { theme = themes[0] }
	heading, levelColor := resolveDiagnosticHeader(d.Level, colors, icons)
	location := formatDiagnosticLocation(d)
	if hasEmbeddedSource(d.Spans) || len(d.SourceLines)>0 { location = "" }

	codeBadge := ""
	if d.Code != "" {
		codeBadge = " " + wrapANSI(semanticColor(colors.Info, "36"), "[" + d.Code + "]")
	}

	var out strings.Builder
	out.Grow(defaultDiagBufferBytes + len(d.Message))

	fmt.Fprintf(&out, "%s%s%s\n",
		wrapANSI("1;7;"+levelColor, " "+heading+" "),
		wrapANSI(semanticColor(colors.Location, "2"), strings.TrimRight("  "+location, " ")),
		codeBadge,
	)

	message := strings.TrimSpace(d.Message)
	if message != "" {
		for _, line := range strings.Split(message, "\n") {
			out.WriteString("  ")
			if d.Tool == TypeScript {
				line = highlightTypeScriptMessage(line, colors)
			}
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}

	if len(d.SourceLines)>0 {
 fmt.Fprintf(&out,"  %s %s\n",wrapANSI(levelColor,"-->"),formatDiagnosticLocation(d))
 out.WriteString(renderSourceContext(d,d.SourceLines,theme))
 } else if len(d.Spans) > 0 && hasEmbeddedSource(d.Spans) {
		out.WriteString(renderDiagnosticSpans(d.Spans, levelColor, colors, theme))
	} else if d.File != "" && d.Line > 0 && d.Line <= maxDiagnosticLineNumber {
		if frame := diagnosticFrame(d, theme); frame != "" {
			out.WriteString(frame)
		}
	}

	if d.Tool == Vitest || d.Tool == NodeTest {
		if m := runnerAssertion.FindStringSubmatch(message); m != nil {
			successColor := semanticColor(colors.Success, "32")
			errorColor := semanticColor(colors.Error, "31")
			fmt.Fprintf(&out, "  %s\n  %s\n",
				wrapANSI(successColor, "Expected: "+m[2]),
				wrapANSI(errorColor, "Received: "+m[1]),
			)
		}
	}

	infoColor := semanticColor(colors.Info, "36")

	if pair := tsTypePair.FindStringSubmatch(message); d.Tool == TypeScript && pair != nil && (d.Code == "TS2322" || d.Code == "TS2345") {
		expected, received := highlightTypeDifference(pair[2], pair[1], colors)
		fmt.Fprintf(&out, "  Expected: %s\n  Received: %s\n", expected, received)
	} else if explanation := diagnosticExplanation(d); explanation != "" {
		prefix := iconOrDefault(icons.Explanation, "↳")
		fmt.Fprintf(&out, "  %s\n", wrapANSI(infoColor, prefix+" "+explanation))
	}

	if suggestion := diagnosticSuggestion(d); suggestion != "" {
		prefix := iconOrDefault(icons.Suggestion, "💡")
		fmt.Fprintf(&out, "  %s\n", wrapANSI(infoColor, prefix+" "+suggestion))
	}

	for _, note := range d.Notes {
		fmt.Fprintf(&out, "  %s %s\n", wrapANSI(infoColor, "note:"), note)
	}
	for _, help := range d.Help {
		fmt.Fprintf(&out, "  %s %s\n", wrapANSI(semanticColor(colors.Success, "32"), "help:"), help)
	}
	for _, related := range d.Related {
		if related != nil {
			fmt.Fprintf(&out, "  %s %s: %s\n", wrapANSI(infoColor, "related:"), formatDiagnosticLocation(related), related.Message)
		}
	}
	return []byte(out.String())
}

func resolveDiagnosticHeader(level Level, colors SemanticColors, icons DiagnosticIcons) (heading, color string) {
	switch level {
	case Warning:
		return iconOrDefault(icons.Warning, "⚠") + " WARNING", semanticColor(colors.Warning, "33")
	case Success:
		return "✓ SUCCESS", semanticColor(colors.Success, "32")
	case Info:
		return iconOrDefault(icons.Info, "ℹ") + " INFO", semanticColor(colors.Info, "36")
	default:
		return iconOrDefault(icons.Error, "✖") + " ERROR", semanticColor(colors.Error, "31")
	}
}

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

func wrapANSI(sgrCode, text string) string {
	if sgrCode == "" || lipgloss.ColorProfile() == 0 {
		return text
	}
	return "\x1b[" + sgrCode + "m" + text + "\x1b[0m"
}

// Highlight actual and expected types without changing the compiler message.
var tsMessageTypes = regexp.MustCompile(`(?i)(?:argument of )?type '(.+?)' (?:is not assignable to (?:type|parameter of type)|does not satisfy the constraint) '(.+?)'(?:[.!]?$|[.]\s|:\s)`)
var tsPropertyTypes = regexp.MustCompile(`(?i)property '(.+?)' (?:does not exist on type|is missing in type) '(.+?)'(?:[.!]?$|[.]\s| but )`)
var tsOverload = regexp.MustCompile(`(?i)(overload [0-9]+ of [0-9]+|no overload matches this call)`)

func highlightTypeScriptMessage(message string, colors SemanticColors) string {
 for _, pattern := range []*regexp.Regexp{tsMessageTypes, tsPropertyTypes} {
  spans := pattern.FindStringSubmatchIndex(message)
  if len(spans) != 6 { continue }
  return message[:spans[2]] + wrapANSI(semanticColor(colors.Error, "31"), message[spans[2]:spans[3]]) + message[spans[3]:spans[4]] + wrapANSI(semanticColor(colors.Success, "32"), message[spans[4]:spans[5]]) + message[spans[5]:]
 }
 return tsOverload.ReplaceAllStringFunc(message, func(match string) string {
  return wrapANSI(semanticColor(colors.Warning, "33"), match)
 })
}

// highlightTypeDifference preserves shared prefix/suffix and emphasizes changed runes.
func highlightTypeDifference(expected, received string, colors SemanticColors) (string, string) {
 a, b := []rune(expected), []rune(received)
 prefix, suffix := 0, 0
 for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] { prefix++ }
 for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] { suffix++ }
 render := func(value []rune, color string) string {
  return wrapANSI("2", string(value[:prefix])) + wrapANSI("1;"+color, string(value[prefix:len(value)-suffix])) + wrapANSI("2", string(value[len(value)-suffix:]))
 }
 return render(a, semanticColor(colors.Success,"32")), render(b, semanticColor(colors.Error,"31"))
}
