package rodlog

import (
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/lipgloss"
)

var (
	sqlStatementPattern = regexp.MustCompile(`(?i)^(?:SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+(?:TABLE|INDEX|VIEW)|ALTER\s+TABLE|DROP\s+TABLE|EXPLAIN)\b`)
	diffHeaderPattern   = regexp.MustCompile(`^(?:diff --git |@@ -\d+|--- a/|\+\+\+ b/)`)
	tomlSectionPattern  = regexp.MustCompile(`^\[[a-zA-Z0-9_.-]+\]$`)
	pythonTracePattern  = regexp.MustCompile(`^(?:Traceback \(most recent call last\):|  File "[^"]+", line \d+)`)
)

func SyntaxHighlight(text, language string) (string, error) {
	return SyntaxHighlightWithTheme(text, language, SyntaxTheme{})
}

func SyntaxHighlightWithTheme(text, language string, theme SyntaxTheme) (string, error) {
	var out strings.Builder
	out.Grow(len(text) * 2)

	style := theme.Style
	if style == "" {
		style = "monokai"
	}

	formatter := resolveChromaFormatter(theme.Profile)
	if err := quick.Highlight(&out, text, language, formatter, style); err != nil {
		return text, err
	}
	return out.String(), nil
}

func resolveChromaFormatter(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "ansi16":
		return "terminal"
	case "ansi256":
		return "terminal256"
	case "truecolor", "":
		return "terminal16m"
	case "auto":
		switch lipgloss.ColorProfile() {
		case 1:
			return "terminal16m"
		case 2:
			return "terminal256"
		default:
			return "terminal"
		}
	default:
		return "terminal16m"
	}
}

func StyleSeverity(text string, level Level) string {
	var color string
	switch level {
	case Error:
		color = "9"
	case Warning:
		color = "11"
	case Success:
		color = "10"
	default:
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
}

func syntaxLanguage(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		return "json"
	}
	if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "#!/bin/bash") || strings.HasPrefix(trimmed, "#!/bin/sh") || strings.HasPrefix(trimmed, "#!/usr/bin/env ") {
		return "bash"
	}
	if dockerfileInstruction.MatchString(trimmed) {
		return "dockerfile"
	}
	if diffHeaderPattern.MatchString(trimmed) {
		return "diff"
	}
	if pythonTracePattern.MatchString(text) {
		return "python"
	}
	if sqlStatementPattern.MatchString(trimmed) {
		return "sql"
	}
	if tomlSectionPattern.MatchString(trimmed) {
		return "toml"
	}
	if strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "interface ") || strings.HasPrefix(trimmed, "type ") || strings.HasPrefix(trimmed, "export ") || strings.HasPrefix(trimmed, "import ") {
		return "typescript"
	}
	if strings.HasPrefix(trimmed, "---\n") && strings.Contains(trimmed, "\n") {
		return "yaml"
	}
	return ""
}
