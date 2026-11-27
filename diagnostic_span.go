package rodlog

import (
	"fmt"
	"strings"
)

// DiagnosticSpan describes a one-based source range. EndColumn is exclusive.
// Source is optional compiler-provided text, not a request to read a file.
type DiagnosticSpan struct {
	File      string `json:"file,omitempty"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndColumn int    `json:"end_column,omitempty"`
	Source    string `json:"source,omitempty"`
	Label     string `json:"label,omitempty"`
	Primary   bool   `json:"primary,omitempty"`
}

func hasEmbeddedSource(spans []DiagnosticSpan) bool {
	for _, span := range spans {
		if span.Source != "" { return true }
	}
	return false
}

func renderDiagnosticSpans(spans []DiagnosticSpan, levelColor string, colors SemanticColors, themes ...SyntaxTheme) string {
	theme := SyntaxTheme{}
	if len(themes) > 0 { theme = themes[0] }
	var out strings.Builder
	for _, span := range spans {
		if span.Source == "" || span.Line < 1 { continue }
		source := strings.ReplaceAll(strings.ReplaceAll(span.Source, "\r", ""), "\n", " ")
		source = ansiPattern.ReplaceAllString(source, "")
		color := semanticColor(colors.Info, "36")
		if span.Primary { color = levelColor }
		if span.File != "" {
			fmt.Fprintf(&out, "  %s %s:%d:%d\n", wrapANSI(color, "-->"), span.File, span.Line, span.Column)
		}
		gutter := fmt.Sprintf("%4d │", span.Line)
		fmt.Fprintf(&out, "  %s %s\n", wrapANSI(color, gutter), highlightSourceLine(source, span.File, theme))
		if span.Column < 1 || span.Column > 512 { continue }
		start := visualColumn(source, span.Column)
		end := visualColumn(source, min(max(span.EndColumn, span.Column+1), 513))
		pointer := strings.Repeat("^", max(1, end-start))
		fmt.Fprintf(&out, "       %s %s%s %s\n", wrapANSI(color, "│"), strings.Repeat(" ", start), wrapANSI(color, pointer), span.Label)
	}
	return out.String()
}
