package rodlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	styleGutter       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleActiveMarker = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	stylePointer      = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleActiveLine   = lipgloss.NewStyle().Bold(true)
)

// diagnosticFrame reads only local relative source paths, never absolute paths
// or parent traversal. It is terminal-only and never changes structured events.
func diagnosticFrame(d *Diagnostic, themes ...SyntaxTheme) string {
	if filepath.IsAbs(d.File) || filepath.Clean(d.File) != d.File || d.File == ".." || strings.HasPrefix(d.File, ".."+string(filepath.Separator)) {
		return ""
	}
	info, err := os.Stat(d.File)
	if err != nil || info.IsDir() || info.Size() > 1024*1024 {
		return ""
	}
	content, err := os.ReadFile(d.File)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if d.Line < 1 || d.Line > len(lines) {
		return ""
	}

	theme := SyntaxTheme{}
	if len(themes) > 0 { theme = themes[0] }
	return renderSourceContext(d, lines, theme)
}

func renderSourceContext(d *Diagnostic, lines []string, theme SyntaxTheme) string {
 if d.Line < 1 || d.Line > len(lines) { return "" }
 before, after := 2, 2
 if d.ContextBefore != nil { before = min(20,max(0,*d.ContextBefore)) }
 if d.ContextAfter != nil { after = min(20,max(0,*d.ContextAfter)) }
 first := max(1,d.Line-before)
 last := min(len(lines),d.Line+after)

	var b strings.Builder
	for i := first; i <= last; i++ {
		isTarget := i == d.Line

		marker := " "
		lineText := highlightSourceLine(lines[i-1], d.File, theme)
		if isTarget {
			marker = styleActiveMarker.Render(">")
		} else {
			lineText = wrapANSI("2;90", ansiPattern.ReplaceAllString(lineText, ""))
		}

		gutter := styleGutter.Render(fmt.Sprintf("%4d |", i))
		fmt.Fprintf(&b, "  %s %s %s\n", marker, gutter, lineText)

		if isTarget && d.Column > 0 && d.Column < 512 {
			padding := strings.Repeat(" ", visualColumn(lines[i-1], d.Column))
			emptyGutter := styleGutter.Render("         |")
			end := d.Column+1
 for _, span := range d.Spans { if span.Line==i && span.Column==d.Column && span.EndColumn>end { end=min(span.EndColumn,513) } }
 pointer := stylePointer.Render(strings.Repeat("^",max(1,visualColumn(lines[i-1],end)-visualColumn(lines[i-1],d.Column))))
			fmt.Fprintf(&b, "%s %s%s\n", emptyGutter, padding, pointer)
		}
	}
	return b.String()
}

// visualColumn converts a one-based rune column to terminal cells.
// Tabs use 8-column stops; combining marks occupy no extra cells.
func visualColumn(line string, column int) int {
	width := 0
	position := 1
	for _, r := range line {
		if position >= column {
			break
		}
		if r == '\t' {
			width += 8 - width%8
		} else if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			// Combining mark.
		} else {
			width += runewidth.RuneWidth(r)
		}
		position++
	}
	return width
}
