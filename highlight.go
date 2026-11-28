package rodlog

import (
	"regexp"
	"slices"
	"strings"

)

const (
	maxRuleMatchesPerLine   = 64
	ansiEscapeOverheadBytes = 16
)

type highlightRule struct {
	expression *regexp.Regexp
	color      string
	priority   int
}

type highlightSpan struct {
	start, end int
	color      string
	priority   int
}

var diagnosticRules = []highlightRule{
	{
		expression: regexp.MustCompile(`\b(?:ERESOLVE|ENOENT|EACCES|EPERM|EADDRINUSE|ECONNREFUSED|ETIMEDOUT|ENOMEM|SIGKILL|SIGTERM|OOMKilled|PERMISSION_DENIED|UNAUTHENTICATED|DEADLINE_EXCEEDED)\b`),
		color:      "31",
		priority:   12,
	},
	{
		expression: regexp.MustCompile(`\bTS[0-9]{4,5}\b|\berror\[E[0-9]{4}\]\b|\b(?:panic|fatal error):`),
		color:      "35",
		priority:   11,
	},
	{
		expression: regexp.MustCompile(`\bexit(?:ed with)? code[:\s]+[1-9][0-9]*\b|\bHTTP[/\s]+[45][0-9]{2}\b|\bstatus[:\s]+[45][0-9]{2}\b`),
		color:      "31",
		priority:   10,
	},
	{
		expression: regexp.MustCompile(`(?:[[:alnum:]_@./-]+)\.(?:[cm]?[tj]sx?|py|go|rs|json|ya?ml|toml|sh|sql|css|html|md):[0-9]+(?::[0-9]+)?`),
		color:      "36",
		priority:   9,
	},
	{
		expression: regexp.MustCompile(`(?:[[:alnum:]_@./-]+)\.(?:[cm]?[tj]sx?|py|go|rs)\([0-9]+,[0-9]+\)`),
		color:      "4;36",
		priority:   9,
	},
	{
		expression: regexp.MustCompile(`\b(?:ERROR|Error|error|FAILED|Failed|failed|FAILURE|Failure|FATAL|Fatal)\b|ERR!`),
		color:      "31",
		priority:   8,
	},
	{
		expression: regexp.MustCompile(`\b(?:WARN|WARNING|Warning|warning|DEPRECATED|Deprecated|deprecated)\b`),
		color:      "33",
		priority:   7,
	},
	{
		expression: regexp.MustCompile(`https?://[^\s"'<>()\]]+`),
		color:      "4;36",
		priority:   7,
	},
	{
		expression: regexp.MustCompile(`\b(?:SUCCESS|Success|success|PASSED|Passed|passed|DONE|Done|done|CACHED|Healthy)\b`),
		color:      "32",
		priority:   6,
	},
	{
		expression: regexp.MustCompile(`\b(?:FROM|WORKDIR|COPY|RUN|ADD|ARG|ENV|ENTRYPOINT|CMD|EXPOSE|VOLUME|USER|HEALTHCHECK|SHELL)\b`),
		color:      "36",
		priority:   4,
	},
	{
		expression: regexp.MustCompile(`\b[0-9]+(?:\.[0-9]+)?(?:ms|s|m|KB|MB|GB|KiB|MiB|GiB)\b`),
		color:      "2",
		priority:   3,
	},
	{
		expression: regexp.MustCompile(`\b(?:npm|npx|pnpm|yarn|bun|node|tsc|vitest|jest|playwright|vite|esbuild|tsdown|go|cargo|rustc|python|uv|ruff|docker|kubectl|terraform|gcloud|wrangler|git|gh)\b`),
		color:      "2",
		priority:   2,
	},
}

func highlightLine(s string) string {
	return highlightLineWithColors(s, SemanticColors{})
}

func highlightLineWithColors(s string, palette SemanticColors) string {
	return highlightWithRules(s, palette, diagnosticRules)
}

func highlightWithRules(s string, palette SemanticColors, rules []highlightRule) string {
	if s == "" || len(rules) == 0 {
		return s
	}
	if strings.Contains(s, "\x1b[") {
		return s
	}

	spans := make([]highlightSpan, 0, 8)
	for _, rule := range rules {
		if rule.expression == nil {
			continue
		}
		color := semanticRuleColor(rule.color, palette)
		if color == "" {
			continue
		}
		for _, pair := range rule.expression.FindAllStringIndex(s, maxRuleMatchesPerLine) {
			if pair[0] == pair[1] {
				continue
			}
			spans = append(spans, highlightSpan{
				start:    pair[0],
				end:      pair[1],
				color:    color,
				priority: rule.priority,
			})
		}
	}

	if len(spans) == 0 {
		return s
	}

	slices.SortStableFunc(spans, func(a, b highlightSpan) int {
		if a.priority != b.priority {
			if a.priority > b.priority {
				return -1
			}
			return 1
		}
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		if a.end > b.end {
			return -1
		}
		if a.end < b.end {
			return 1
		}
		return 0
	})

	accepted := make([]highlightSpan, 0, len(spans))
	for _, candidate := range spans {
		if !overlapsAny(candidate, accepted) {
			accepted = append(accepted, candidate)
		}
	}

	slices.SortFunc(accepted, func(a, b highlightSpan) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return 0
	})

	var out strings.Builder
	out.Grow(len(s) + len(accepted)*ansiEscapeOverheadBytes)

	cursor := 0
	for _, span := range accepted {
		out.WriteString(s[cursor:span.start])
		out.WriteString("\x1b[")
		out.WriteString(span.color)
		out.WriteByte('m')
		out.WriteString(s[span.start:span.end])
		out.WriteString("\x1b[0m")
		cursor = span.end
	}
	out.WriteString(s[cursor:])

	return out.String()
}

func overlapsAny(candidate highlightSpan, accepted []highlightSpan) bool {
	for _, existing := range accepted {
		if candidate.start < existing.end && existing.start < candidate.end {
			return true
		}
	}
	return false
}
