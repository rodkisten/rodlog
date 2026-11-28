package rodlog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	pnpmProgress = regexp.MustCompile(`^Progress:\s+resolved\s+(\d+),\s+reused\s+(\d+),\s+downloaded\s+(\d+),\s+added\s+(\d+)(?:,\s+done)?$`)
	pnpmPackages = regexp.MustCompile(`^Packages:\s+([+-]\d+).*$`)
	pnpmDone = regexp.MustCompile(`^Done in\s+([^\s]+)(?:\s+using\s+pnpm)?$`)
	pnpmScope = regexp.MustCompile(`^Scope:\s+(.+)$`)
)

func isPNPMOutput(input string) bool {
	line := strings.TrimSpace(ansiPattern.ReplaceAllString(input, ""))
	if line == "" {
		return false
	}
	if pnpmDiagnostic.MatchString(line) || pnpmProgress.MatchString(line) || pnpmPackages.MatchString(line) || pnpmDone.MatchString(line) || pnpmScope.MatchString(line) {
		return true
	}
	for _, prefix := range []string{
		"Lockfile is up to date",
		"Packages are copied from the content-addressable store",
		"Content-addressable store is at:",
		"Virtual store is at:",
		"Issues with peer dependencies found",
		"Skipped build scripts:",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func highlightPNPM(input string, opts Options) (string, bool) {
	return highlightPNPMMode(input, opts, true)
}

func highlightPNPMMode(input string, opts Options, includeTool bool) (string, bool) {
	colors := opts.SemanticColors
	icons := resolveDiagnosticIcons(opts.DiagnosticIcons, !opts.ASCIISymbols)
	trimmedRight := strings.TrimRight(input, "\r\n")
	ending := input[len(trimmedRight):]
	line := strings.TrimSpace(ansiPattern.ReplaceAllString(trimmedRight, ""))
	if !isPNPMOutput(line) {
		return "", false
	}

	info := semanticColor(colors.Info, "36")
	success := semanticColor(colors.Success, "32")
	warning := semanticColor(colors.Warning, "33")
	muted := semanticColor(colors.Muted, "2")

	if m := pnpmProgress.FindStringSubmatch(line); m != nil {
		parts := []string{
			wrapANSI(info, toolLabel(PNPM, opts)),
			wrapANSI(muted, "resolved")+" "+wrapANSI(info, m[1]),
			wrapANSI(muted, "reused")+" "+wrapANSI(success, m[2]),
			wrapANSI(muted, "downloaded")+" "+wrapANSI(info, m[3]),
			wrapANSI(muted, "added")+" "+wrapANSI(success, m[4]),
		}
		return strings.Join(parts, "  ") + ending, true
	}
	if m := pnpmPackages.FindStringSubmatch(line); m != nil {
		return wrapANSI(info, "◆ pnpm") + "  " + wrapANSI(success, "packages "+m[1]) + ending, true
	}
	if m := pnpmDone.FindStringSubmatch(line); m != nil {
		prefix := ""
		if includeTool { prefix = wrapANSI(success, icons.Success+" pnpm") + "  " }
		return prefix + wrapANSI(success, icons.Success+" done") + " " + dimDockerDuration(m[1]) + ending, true
	}
	if m := pnpmScope.FindStringSubmatch(line); m != nil {
		return wrapANSI(info, "◆ pnpm") + "  " + wrapANSI(muted, "scope") + " " + m[1] + ending, true
	}
	if strings.HasPrefix(line, "Lockfile is up to date") {
		return wrapANSI(success, "✓ pnpm") + "  " + line + ending, true
	}
	if strings.Contains(line, "peer dependencies") {
		prefix := ""
		if includeTool { prefix = wrapANSI(warning, icons.Warning+" pnpm") + "  " }
		return prefix + wrapANSI(warning, icons.Warning+" "+line) + ending, true
	}
	if m := pnpmDiagnostic.FindStringSubmatch(line); m != nil {
		level := warning
		symbol := icons.Warning
		if strings.HasPrefix(m[1], "ERR_") {
			level = semanticColor(colors.Error, "31")
			symbol = icons.Error
		}
		prefix := ""
		if includeTool { prefix = wrapANSI(level, symbol+" pnpm") + "  " }
		return prefix + wrapANSI(level, symbol+" "+m[1]) + "  " + m[2] + ending, true
	}
	// Store and deployment chatter is useful context but should stay quiet.
	return wrapANSI(info, "◆ pnpm") + "  " + wrapANSI(muted, line) + ending, true
}

func pnpmPrefix(color string, opts Options, includeTool bool) string {
	if !includeTool {
		return ""
	}
	return wrapANSI(color, toolLabel(PNPM, opts)) + "  "
}

func pnpmProgressCount(input string) int {
	m := pnpmProgress.FindStringSubmatch(strings.TrimSpace(input))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func toolLabel(tool Tool, opts Options) string {
	icons := resolveDiagnosticIcons(opts.DiagnosticIcons, !opts.ASCIISymbols)
	name := string(tool)
	switch tool {
	case TypeScript:
		name = "tsc"
	case GoCompiler:
		name = "go"
	}
	return fmt.Sprintf("%s %s", icons.Definition, name)
}
