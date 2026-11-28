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
	theme := SyntaxTheme{}
	if len(themes) > 0 { theme = themes[0] }
	return renderDiagnosticWithDetail(d, colors, icons, theme, DetailNormal)
}

func renderDiagnosticWithDetail(d *Diagnostic, colors SemanticColors, icons DiagnosticIcons, theme SyntaxTheme, detail DiagnosticDetail) []byte {
	if d == nil {
		return nil
	}
	if detail == "" {
		detail = DetailNormal
	}
	icons = resolveDiagnosticIcons(icons, true)
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
		lines, hiddenMessageLines := diagnosticMessageLines(d, message, detail)
		for _, line := range lines {
			out.WriteString("  ")
			if d.Tool == TypeScript {
				line = highlightTypeScriptMessage(line, colors)
			}
			out.WriteString(line)
			out.WriteByte('\n')
		}
		if hiddenMessageLines > 0 {
			fmt.Fprintf(&out, "  %s\n",
				wrapANSI("2", diagnosticCount(hiddenMessageLines, "more compiler line")+" · --detail=verbose"),
			)
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

	typePairRendered := false
	if pair := tsTypePair.FindStringSubmatch(message); d.Tool == TypeScript && pair != nil && (d.Code == "TS2322" || d.Code == "TS2345") {
		expected, received := highlightTypeDifference(pair[2], pair[1], colors)
		fmt.Fprintf(&out, "  Expected: %s\n  Received: %s\n", expected, received)
		typePairRendered = true
	} else if explanation := diagnosticExplanation(d); explanation != "" {
		prefix := iconOrDefault(icons.Explanation, "↳")
		fmt.Fprintf(&out, "  %s\n", wrapANSI(infoColor, prefix+" "+explanation))
	}

	if suggestion := diagnosticSuggestion(d); suggestion != "" && !typePairRendered {
		prefix := iconOrDefault(icons.Suggestion, "💡")
		fmt.Fprintf(&out, "  %s\n", wrapANSI(infoColor, prefix+" "+suggestion))
	}

	for _, note := range d.Notes {
		fmt.Fprintf(&out, "  %s %s %s\n", wrapANSI(infoColor, icons.Note), wrapANSI(infoColor, "note:"), note)
	}
	for _, help := range d.Help {
		fmt.Fprintf(&out, "  %s %s %s\n", wrapANSI(semanticColor(colors.Success, "32"), icons.Help), wrapANSI(semanticColor(colors.Success, "32"), "help:"), help)
	}
	renderQuickFixes(&out, d, colors, icons, theme, detail)
	renderRelatedDiagnostics(&out, d, colors, icons, theme, detail)
	return []byte(out.String())
}

func renderQuickFixes(out *strings.Builder, d *Diagnostic, colors SemanticColors, icons DiagnosticIcons, theme SyntaxTheme, detail DiagnosticDetail) {
	if d == nil || len(d.Fixes) == 0 {
		return
	}

	fixes := d.Fixes
	hiddenLowConfidence := 0
	if detail != DetailVerbose {
		filtered := make([]DiagnosticFix, 0, len(fixes))
		for _, fix := range fixes {
			if quickFixLowConfidence(fix) {
				hiddenLowConfidence++
				continue
			}
			filtered = append(filtered, fix)
		}
		fixes = filtered
	}

	limit := len(fixes)
	switch detail {
	case DetailCompact:
		limit = min(limit, 1)
	case DetailNormal:
		limit = min(limit, 2)
	}

	for i, fix := range fixes[:limit] {
		label := "Quick fix"
		if len(fixes) > 1 {
			label = fmt.Sprintf("Quick fix %d/%d", i+1, len(fixes))
		}
		if detail == DetailVerbose && quickFixLowConfidence(fix) {
			label = "Low-confidence fix"
		}
		fmt.Fprintf(out, "  %s %s: %s\n",
			wrapANSI(semanticColor(colors.Warning, "33"), icons.Suggestion),
			wrapANSI("1", label),
			fix.Description,
		)

		changeLimit := len(fix.Changes)
		switch detail {
		case DetailCompact:
			changeLimit = min(changeLimit, 1)
		case DetailNormal:
			changeLimit = min(changeLimit, 2)
		}

		for _, change := range fix.Changes[:changeLimit] {
			location := change.DisplayFile
			if location == "" {
				location = change.File
			}
			if change.Line > 0 {
				location += ":" + strconv.Itoa(change.Line)
				if change.Column > 0 {
					location += ":" + strconv.Itoa(change.Column)
				}
			}
			if location != "" {
				fmt.Fprintf(out, "    %s %s\n",
					wrapANSI(semanticColor(colors.Location, "36"), icons.Location),
					wrapANSI("2", location),
				)
			}
			renderFixTextChange(out, change, colors, theme)
		}

		if changeLimit < len(fix.Changes) {
			fmt.Fprintf(out, "    %s\n", wrapANSI("2", diagnosticCount(len(fix.Changes)-changeLimit, "more edit")))
		}
	}
	if limit < len(fixes) {
		fmt.Fprintf(out, "  %s\n", wrapANSI("2", diagnosticCount(len(fixes)-limit, "more quick fix")+" · --detail=verbose"))
	}
	if hiddenLowConfidence > 0 {
		fmt.Fprintf(out, "  %s\n",
			wrapANSI("2", diagnosticCount(hiddenLowConfidence, "low-confidence quick fix")+" hidden · --detail=verbose"),
		)
	}
}

func quickFixLowConfidence(fix DiagnosticFix) bool {
	description := strings.ToLower(fix.Description)
	if strings.Contains(description, "add index signature") {
		return true
	}
	for _, change := range fix.Changes {
		text := strings.ToLower(change.NewText)
		for _, risky := range []string{
			": any", " as any", "<any>", "[x: string]: any", "[key: string]: any",
		} {
			if strings.Contains(text, risky) {
				return true
			}
		}
	}
	return false
}

func diagnosticMessageLines(d *Diagnostic, message string, detail DiagnosticDetail) ([]string, int) {
	lines := strings.Split(message, "\n")
	if d == nil || d.Tool != TypeScript || detail == DetailVerbose {
		return lines, 0
	}
	limit := len(lines)
	switch detail {
	case DetailCompact:
		limit = min(limit, 4)
	default:
		limit = min(limit, 8)
	}
	if len(lines) <= limit {
		return lines, 0
	}
	return lines[:limit], len(lines) - limit
}

func renderFixTextChange(out *strings.Builder, change DiagnosticTextChange, colors SemanticColors, theme SyntaxTheme) {
	oldLines := compactFixLines(change.OldText, 2)
	newLines := compactFixLines(change.NewText, 2)
	file := change.DisplayFile
	if file == "" {
		file = change.File
	}

	for _, line := range oldLines {
		fmt.Fprintf(out, "    %s %s\n",
			wrapANSI(semanticColor(colors.Error, "31"), "-"),
			highlightSourceLine(line, file, theme),
		)
	}
	for _, line := range newLines {
		fmt.Fprintf(out, "    %s %s\n",
			wrapANSI(semanticColor(colors.Success, "32"), "+"),
			highlightSourceLine(line, file, theme),
		)
	}
}

func compactFixLines(text string, maxLines int) []string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "…")
	}
	for i := range lines {
		if len([]rune(lines[i])) > 96 {
			runes := []rune(lines[i])
			lines[i] = string(runes[:93]) + "…"
		}
	}
	return lines
}

func renderRelatedDiagnostics(out *strings.Builder, parent *Diagnostic, colors SemanticColors, icons DiagnosticIcons, theme SyntaxTheme, detail DiagnosticDetail) {
	if parent == nil || len(parent.Related) == 0 {
		return
	}

	seen := make(map[string]bool)
	related := make([]*Diagnostic, 0, len(parent.Related))
	for _, item := range parent.Related {
		if item == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d:%s:%s:%s", item.File, item.Line, item.Column, item.Message, item.Signature, item.Documentation)
		if seen[key] {
			continue
		}
		seen[key] = true
		related = append(related, item)
	}

	overload := parent.Tool == TypeScript && (parent.Code == "TS2769" || strings.Contains(strings.ToLower(parent.Message), "no overload matches"))
	consumed := make(map[*Diagnostic]bool)
	hidden := 0
	if overload {
		candidates := make([]*Diagnostic, 0, len(related))
		signatures := make(map[string]bool)
		for _, item := range related {
			if item.Signature == "" || !strings.Contains(strings.ToLower(item.Message), "symbol declared here") {
				continue
			}
			if signatures[item.Signature] {
				consumed[item] = true
				continue
			}
			signatures[item.Signature] = true
			candidates = append(candidates, item)
			consumed[item] = true
		}
		if len(candidates) > 1 {
			fmt.Fprintf(out, "  %s %s\n",
				wrapANSI(semanticColor(colors.Warning, "33"), icons.Candidate),
				wrapANSI("1", diagnosticCount(len(candidates), "candidate signature")),
			)
			doc := ""
			for i, item := range candidates {
				fmt.Fprintf(out, "    %d  %s %s\n", i+1,
					wrapANSI(semanticColor(colors.Info, "36"), icons.Signature),
					renderSignatureFocus(item, colors, theme),
				)
				renderSignatureFocusHint(out, item, colors, icons)
				if loc := formatDiagnosticLocation(item); loc != "" {
					fmt.Fprintf(out, "       %s %s\n",
						wrapANSI(semanticColor(colors.Location, "36"), icons.Location),
						wrapANSI("2", loc),
					)
				}
				if doc == "" && item.Documentation != "" {
					doc = item.Documentation
				}
			}
			if doc != "" {
				fmt.Fprintf(out, "    %s %s\n", wrapANSI(semanticColor(colors.Info, "36"), icons.Documentation), wrapANSI("2", doc))
			}
			for _, item := range related {
				if strings.Contains(strings.ToLower(item.Message), "resolved or recovery signature") {
					consumed[item] = true
				}
			}
		}
	}

	remaining := make([]*Diagnostic, 0, len(related))
	for _, item := range related {
		if !consumed[item] {
			remaining = append(remaining, item)
		}
	}

	if detail != DetailVerbose && len(remaining) > 0 {
		bestSignature := -1
		bestCompilerRelated := -1
		bestPriority := 99
		for i, item := range remaining {
			if item.Signature != "" {
				priority := relatedPriority(item)
				if priority < bestPriority {
					bestPriority = priority
					bestSignature = i
				}
			} else if bestCompilerRelated < 0 {
				bestCompilerRelated = i
			}
		}
		keep := make(map[int]bool)
		if bestSignature >= 0 {
			keep[bestSignature] = true
		}
		if detail == DetailNormal && bestCompilerRelated >= 0 && bestCompilerRelated != bestSignature {
			keep[bestCompilerRelated] = true
		}
		filtered := make([]*Diagnostic, 0, len(keep))
		for i, item := range remaining {
			if keep[i] {
				filtered = append(filtered, item)
			} else {
				hidden++
			}
		}
		remaining = filtered
	}

	for _, item := range remaining {
		location := formatDiagnosticLocation(item)
		if item.Signature != "" {
			label := "Defined here"
			lower := strings.ToLower(item.Message)
			switch {
			case strings.Contains(lower, "generic constraint"):
				label = "Generic constraint"
			case strings.Contains(lower, "expected property"):
				label = "Expected property"
			case strings.Contains(lower, "return type"):
				label = "Return type"
			case strings.Contains(lower, "expected type"):
				label = "Expected type"
			case strings.Contains(lower, "type declared"):
				label = "Type declared here"
			}
			fmt.Fprintf(out, "  %s %s", wrapANSI(semanticColor(colors.Info, "36"), icons.Definition), label)
			if location != "" {
				fmt.Fprintf(out, "  %s %s", wrapANSI(semanticColor(colors.Location, "36"), icons.Location), wrapANSI("2", location))
			}
			out.WriteByte('\n')
			fmt.Fprintf(out, "    %s %s\n", wrapANSI(semanticColor(colors.Info, "36"), icons.Signature), renderSignatureFocus(item, colors, theme))
			renderSignatureFocusHint(out, item, colors, icons)
			if item.Documentation != "" && detail != DetailCompact {
				fmt.Fprintf(out, "    %s %s\n", wrapANSI(semanticColor(colors.Info, "36"), icons.Documentation), wrapANSI("2", item.Documentation))
			}
			continue
		}

		fmt.Fprintf(out, "  %s", wrapANSI(semanticColor(colors.Info, "36"), icons.Related))
		if location != "" {
			fmt.Fprintf(out, " %s %s", wrapANSI(semanticColor(colors.Location, "36"), icons.Location), wrapANSI("2", location))
		}
		if item.Message != "" {
			fmt.Fprintf(out, "  %s", item.Message)
		}
		out.WriteByte('\n')
	}
	if hidden > 0 {
		fmt.Fprintf(out, "  %s %s\n", wrapANSI("2", "…"), wrapANSI("2", diagnosticCount(hidden, "more related detail")+" · --detail=verbose"))
	}
}

func renderSignatureFocus(item *Diagnostic, colors SemanticColors, theme SyntaxTheme) string {
	if item == nil || item.Signature == "" {
		return ""
	}
	signature := highlightSourceLine(item.Signature, "signature.ts", theme)
	if item.SignatureFocus == "" {
		return signature
	}

	plain := item.Signature
	start, end := item.SignatureFocusStart, item.SignatureFocusEnd
	if start < 0 || end <= start || end > len(plain) {
		start = strings.Index(plain, item.SignatureFocus)
		if start < 0 {
			return signature
		}
		end = start + len(item.SignatureFocus)
	}
	prefix := highlightSourceLine(plain[:start], "signature.ts", theme)
	focus := highlightSourceLine(plain[start:end], "signature.ts", theme)
	suffix := highlightSourceLine(plain[end:], "signature.ts", theme)
	focus = wrapANSI("1;4;"+semanticColor(colors.Error, "31"), ansiPattern.ReplaceAllString(focus, ""))
	return prefix + focus + suffix
}

func renderSignatureFocusHint(out *strings.Builder, item *Diagnostic, colors SemanticColors, icons DiagnosticIcons) {
	if item == nil || item.SignatureFocus == "" {
		return
	}
	label := item.SignatureFocusLabel
	if label == "" {
		label = "expected here"
	}
	fmt.Fprintf(out, "      %s %s\n",
		wrapANSI(semanticColor(colors.Error, "31"), icons.Explanation),
		wrapANSI("2", label),
	)
}

func relatedPriority(item *Diagnostic) int {
	if item == nil {
		return 99
	}
	lower := strings.ToLower(item.Message)
	switch {
	case strings.Contains(lower, "generic constraint"):
		return 0
	case strings.Contains(lower, "expected property"):
		return 0
	case strings.Contains(lower, "return type"):
		return 0
	case strings.Contains(lower, "expected type"):
		return 1
	case strings.Contains(lower, "type declared"):
		return 2
	case strings.Contains(lower, "symbol declared"):
		return 3
	case strings.Contains(lower, "defined here"):
		return 4
	case strings.Contains(lower, "resolved or recovery"):
		return 5
	default:
		return 5
	}
}

func resolveDiagnosticHeader(level Level, colors SemanticColors, icons DiagnosticIcons) (heading, color string) {
	switch level {
	case Warning:
		return iconOrDefault(icons.Warning, "⚠") + " WARNING", semanticColor(colors.Warning, "33")
	case Success:
		return iconOrDefault(icons.Success, "✓") + " SUCCESS", semanticColor(colors.Success, "32")
	case Info:
		return iconOrDefault(icons.Info, "ℹ") + " INFO", semanticColor(colors.Info, "36")
	default:
		return iconOrDefault(icons.Error, "✖") + " ERROR", semanticColor(colors.Error, "31")
	}
}

func formatDiagnosticLocation(d *Diagnostic) string {
	if d.File == "" && d.DisplayFile == "" {
		return string(d.Tool)
	}

	var b strings.Builder
	file := d.DisplayFile
	if file == "" {
		file = d.File
	}
	b.WriteString(file)
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
