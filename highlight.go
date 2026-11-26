package rodlog

import (
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	// Cap regex matches per rule on a single line to guard against pathological
	// inputs (e.g. minified bundles or huge JSON dumps) causing CPU spikes.
	maxRuleMatchesPerLine = 64

	// Estimated extra bytes per highlighted span: "\x1b[" + color + "m" + "\x1b[0m"
	ansiEscapeOverheadBytes = 16
)

// highlightRule pairs a compiled regular expression with an SGR color code
// and a priority weight. Higher priority wins when two spans overlap.
type highlightRule struct {
	expression *regexp.Regexp
	color      string
	priority   int
}

// highlightSpan represents a byte-offset interval [start, end) in the input string.
type highlightSpan struct {
	start, end int
	color      string
	priority   int
}

// diagnosticRules defines ordered semantic token matchers for CLI and CI logs.
// Priorities (1-15) resolve overlapping matches deterministically:
//   - 12-15: Critical compiler/runtime error codes & signals
//   - 9-11:  File locations, stack coordinates, exit codes, HTTP 5xx
//   - 6-8:   Severity keywords (ERROR, WARN, SUCCESS) & URLs
//   - 2-5:   Dockerfile instructions, CLI tool names, durations/sizes
var diagnosticRules = []highlightRule{
	// 1. Critical runtime/system error codes & signals (Priority 12)
	{
		expression: regexp.MustCompile(`\b(?:ERESOLVE|ENOENT|EACCES|EPERM|EADDRINUSE|ECONNREFUSED|ETIMEDOUT|ENOMEM|SIGKILL|SIGTERM|OOMKilled|PERMISSION_DENIED|UNAUTHENTICATED|DEADLINE_EXCEEDED)\b`),
		color:      "31", // Red
		priority:   12,
	},
	// 2. Compiler diagnostic codes: TypeScript (TS2322), Rust (E0308), Go panics (Priority 11)
	{
		expression: regexp.MustCompile(`\bTS[0-9]{4,5}\b|\berror\[E[0-9]{4}\]\b|\b(?:panic|fatal error):`),
		color:      "35", // Magenta
		priority:   11,
	},
	// 3. Non-zero exit codes & HTTP 4xx/5xx error statuses (Priority 10)
	{
		expression: regexp.MustCompile(`\bexit(?:ed with)? code[:\s]+[1-9][0-9]*\b|\bHTTP[/\s]+[45][0-9]{2}\b|\bstatus[:\s]+[45][0-9]{2}\b`),
		color:      "31", // Red
		priority:   10,
	},
	// 4. File paths with line[:column] coordinates: src/app.ts:42:10, main.go:18 (Priority 9)
	{
		expression: regexp.MustCompile(`(?:[[:alnum:]_@./-]+)\.(?:[cm]?[tj]sx?|py|go|rs|json|ya?ml|toml|sh|sql|css|html|md):[0-9]+(?::[0-9]+)?`),
		color:      "36", // Cyan
		priority:   9,
	},
	// 5. MSBuild / TypeScript parenthesized coordinates: src/app.ts(42,10) (Priority 9)
	{
		expression: regexp.MustCompile(`(?:[[:alnum:]_@./-]+)\.(?:[cm]?[tj]sx?|py|go|rs)\([0-9]+,[0-9]+\)`),
		color:      "4;36", // Underlined cyan
		priority:   9,
	},
	// 6. Severity keywords: Errors & Failures (Priority 8)
	{
		expression: regexp.MustCompile(`\b(?:ERROR|Error|error|FAILED|Failed|failed|FAILURE|Failure|FATAL|Fatal)\b|ERR!`),
		color:      "31", // Red
		priority:   8,
	},
	// 7. Severity keywords: Warnings & Deprecations (Priority 7)
	{
		expression: regexp.MustCompile(`\b(?:WARN|WARNING|Warning|warning|DEPRECATED|Deprecated|deprecated)\b`),
		color:      "33", // Yellow
		priority:   7,
	},
	// 8. HTTP/HTTPS URLs (Priority 7)
	{
		expression: regexp.MustCompile(`https?://[^\s"'<>)\]]+`),
		color:      "4;36", // Underlined cyan
		priority:   7,
	},
	// 9. Severity keywords: Success & Completion (Priority 6)
	{
		expression: regexp.MustCompile(`\b(?:SUCCESS|Success|success|PASSED|Passed|passed|DONE|Done|done|CACHED|Healthy)\b`),
		color:      "32", // Green
		priority:   6,
	},
	// 10. Dockerfile & CI stage instructions (Priority 4)
	{
		expression: regexp.MustCompile(`\b(?:FROM|WORKDIR|COPY|RUN|ADD|ARG|ENV|ENTRYPOINT|CMD|EXPOSE|VOLUME|USER|HEALTHCHECK|SHELL)\b`),
		color:      "36", // Cyan
		priority:   4,
	},
	// 11. Durations & byte sizes (e.g. 120ms, 3.45s, 42.1 MB) to reduce visual noise (Priority 3)
	{
		expression: regexp.MustCompile(`\b[0-9]+(?:\.[0-9]+)?(?:ms|s|m|KB|MB|GB|KiB|MiB|GiB)\b`),
		color:      "2", // Dim
		priority:   3,
	},
	// 12. Common developer CLI binaries (Priority 2)
	{
		expression: regexp.MustCompile(`\b(?:npm|npx|pnpm|yarn|bun|node|tsc|vitest|jest|playwright|vite|esbuild|tsdown|go|cargo|rustc|python|uv|ruff|docker|kubectl|terraform|gcloud|wrangler|git|gh)\b`),
		color:      "2", // Dim
		priority:   2,
	},
}

// highlightLine first collects matches, resolves overlaps, and only then writes
// escape sequences. Matching before rendering prevents our own ANSI codes from
// being interpreted as user content. A bounded match count limits log overhead.
func highlightLine(s string) string {
	return highlightLineWithColors(s, SemanticColors{})
}

// highlightLineWithColors highlights `s` using the default `diagnosticRules`
// mapped through the caller's `SemanticColors` theme.
func highlightLineWithColors(s string, palette SemanticColors) string {
	return highlightWithRules(s, palette, diagnosticRules)
}

// highlightWithRules executes interval-based syntax/semantic highlighting over `s`.
// Algorithm:
//  1. Collect all regex match byte ranges [start, end) across all rules.
//  2. Sort candidates by descending priority (ties broken by earliest start offset).
//  3. Greedily accept non-overlapping intervals.
//  4. Sort accepted intervals by ascending start offset and stitch the final string in one pass.
func highlightWithRules(s string, palette SemanticColors, rules []highlightRule) string {
	// Fast-path exits: empty input, no rules, or terminal color output disabled.
	if s == "" || len(rules) == 0 || lipgloss.ColorProfile() == 0 {
		return s
	}

	// Avoid injecting ANSI sequences into lines that already contain raw escapes,
	// which could split an existing "\x1b[31m" sequence if a rule matched digits.
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

		// FindAllStringIndex returns UTF-8 byte offsets [start, end], which align
		// directly with Go string slicing `s[start:end]`.
		for _, pair := range rule.expression.FindAllStringIndex(s, maxRuleMatchesPerLine) {
			if pair[0] == pair[1] {
				continue // Skip zero-width matches
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

	// slices.SortStableFunc (Go 1.21+) uses type-safe generics instead of reflection,
	// avoiding interface boxing allocations incurred by sort.SliceStable.
	slices.SortStableFunc(spans, func(a, b highlightSpan) int {
		if a.priority != b.priority {
			// Descending priority: higher priority comes first.
			if a.priority > b.priority {
				return -1
			}
			return 1
		}
		// Ascending start offset: earlier token wins ties.
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		// Longer span wins if both priority and start offset are identical.
		if a.end > b.end {
			return -1
		}
		if a.end < b.end {
			return 1
		}
		return 0
	})

	// Resolve overlapping intervals without allocating an O(len(s)) boolean mask.
	accepted := make([]highlightSpan, 0, len(spans))
	for _, candidate := range spans {
		if !overlapsAny(candidate, accepted) {
			accepted = append(accepted, candidate)
		}
	}

	// Re-sort accepted spans left-to-right so we can assemble the output in a single linear pass.
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

// overlapsAny reports whether `candidate` intersects any half-open interval [start, end)
// already present in `accepted`. Two half-open intervals [a0, a1) and [b0, b1) overlap
// if and only if a0 < b1 && b0 < a1.
func overlapsAny(candidate highlightSpan, accepted []highlightSpan) bool {
	for _, existing := range accepted {
		if candidate.start < existing.end && existing.start < candidate.end {
			return true
		}
	}
	return false
}
