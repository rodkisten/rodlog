package rodlog

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	npmErrPrefixOld   = "npm ERR!"
	npmErrPrefixNew   = "npm error"
	npmMissingScript  = "Missing script:"
	npmLogLocationMsg = "A complete log of this run can be found in:"
	npmCommandPrefix  = "npm run"
)

var (
	npmCardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("9"))

	npmCardMessageStyle = lipgloss.NewStyle().
				Bold(true)

	npmCardSuggestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10"))

	npmCardLogPathStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14"))

	npmCardBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(0, 1)
)

type NPMDiagnostic struct {
	Message    string
	Suggestion string
	LogPath    string
	Details    []string
}

func ParseNPMDiagnostic(lines []string) NPMDiagnostic {
	var d NPMDiagnostic

	for _, rawLine := range lines {
		content := normalizeNPMLine(rawLine)
		if content == "" {
			continue
		}

		switch {
		case strings.HasPrefix(content, npmMissingScript):
			d.Message = content

		case strings.HasPrefix(content, npmLogLocationMsg):
			if path, ok := strings.CutPrefix(content, npmLogLocationMsg); ok {
				d.LogPath = strings.TrimSpace(path)
			}

		case strings.HasPrefix(content, npmCommandPrefix):
			d.Suggestion = content

		default:
			d.Details = append(d.Details, content)
		}
	}

	return d
}

func RenderNPMCard(lines []string, opts Options) string {
	if len(lines) == 0 {
		return ""
	}

	width := cardWidth(opts)
	diag := ParseNPMDiagnostic(lines)

	body := make([]string, 0, 4+len(diag.Details))

	icon := iconOrDefault(opts.DiagnosticIcons.Error, "✖")
	body = append(body, npmCardTitleStyle.Render(icon+" NPM ERROR"))

	if diag.Message != "" {
		body = appendStyledLines(body, diag.Message, width, npmCardMessageStyle)
	}

	for _, detail := range uniqueCardDetails(diag.Message, diag.Details) {
		body = append(body, cardLines(detail, width)...)
	}

	if diag.Suggestion != "" {
		body = appendStyledLines(body, "$ "+diag.Suggestion, width, npmCardSuggestionStyle)
	}

	if diag.LogPath != "" {
		body = append(body, "Debug log:")
		body = appendStyledLines(body, diag.LogPath, width, npmCardLogPathStyle)
	}

	box := npmCardBoxStyle.
		Width(width - 4).
		MaxWidth(width).
		Render(strings.Join(body, "\n"))

	return colorizeRenderedCard(box, NPM, opts) + "\n"
}

func normalizeNPMLine(line string) string {
	s := strings.TrimSpace(line)
	s, _ = strings.CutPrefix(s, npmErrPrefixOld)
	s, _ = strings.CutPrefix(s, npmErrPrefixNew)
	return strings.TrimSpace(s)
}

func appendStyledLines(dst []string, text string, width int, style lipgloss.Style) []string {
	for _, line := range cardLines(text, width) {
		dst = append(dst, style.Render(line))
	}
	return dst
}
