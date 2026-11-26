package rodlog

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	defaultStepSeparatorWidth = 36
	composeMaxServiceWidth    = 24
)

// Package-level compiled regular expressions.
// In Go, regexp.MustCompile should ALWAYS be called at package initialization time,
// never inside functions/methods, because compiling regexes at runtime is expensive.
var (
	// --- Docker BuildKit & Classic Builder patterns ---

	// Matches modern BuildKit step headers, including multi-stage and cross-platform builds:
	// e.g. "#7 [4/4] RUN npm ci" or "#12 [linux/amd64 builder 2/5] COPY . ."
	buildKitHeader = regexp.MustCompile(`^#(\d+)\s+\[([^\]]+)\]\s+(.+?)(?:\r?\n)?$`)

	// Matches classic Docker builder step headers:
	// e.g. "Step 1/5 : FROM node:22-alpine"
	buildKitLegacyHeader = regexp.MustCompile(`^Step\s+(\d+)/(\d+)\s*:\s*(.+?)(?:\r?\n)?$`)

	// Matches BuildKit lifecycle status lines:
	// e.g. "#7 DONE 1.4s", "#7 CACHED", "#7 ERROR: process ... did not complete"
	buildKitStatus = regexp.MustCompile(`^#(\d+)\s+(DONE|CACHED|ERROR|FINISHED)([^\r\n]*)(?:\r?\n)?$`)

	// Matches BuildKit progress/log lines belonging to an active step ID:
	// e.g. "#7 0.452 added 120 packages in 2s" or "#7 sha256:9f86d08..."
	buildKitChild = regexp.MustCompile(`^#(\d+)\s+(?:(\d+(?:\.\d+)?)\s+)?(.+?)(?:\r?\n)?$`)

	// Matches BuildKit summary error lines at the end of a failed build:
	// e.g. "STEP 3 ERROR: executor failed running [/bin/sh -c npm test]"
	buildKitFinalError = regexp.MustCompile(`(?i)^STEP\s+(\d+)\s+ERROR:\s*(.+?)(?:\r?\n)?$`)

	// Matches top-level BuildKit solver errors (e.g. "ERROR: failed to solve: ...").
	buildKitSolveError = regexp.MustCompile(`^ERROR:\s+(failed to solve:.+?)(?:\r?\n)?$`)

	// Matches legacy Docker builder intermediate markers:
	// e.g. " ---> Using cache", " ---> Running in a1b2c3d4e5f6", " ---> 9f86d081884c"
	buildKitLegacyChild = regexp.MustCompile(`^\s*--->\s+(.+?)(?:\r?\n)?$`)

	// Matches final legacy Docker builder completion messages:
	// e.g. "Successfully built 9f86d081884c" or "Successfully tagged myapp:latest"
	buildKitLegacySuccess = regexp.MustCompile(`^(Successfully\s+(?:built|tagged))\s+(.+?)(?:\r?\n)?$`)

	// Matches valid Dockerfile instructions (case-insensitive) to prevent false positives
	// when parsing legacy "Step X/Y :" lines from unrelated tools.
	buildKitInstruction = regexp.MustCompile(`(?i)^(?:RUN|COPY|ADD|FROM|WORKDIR|ENV|ARG|CMD|ENTRYPOINT|USER|EXPOSE|LABEL|HEALTHCHECK|SHELL|VOLUME|ONBUILD|STOPSIGNAL|MAINTAINER)\b`)

	// Extracts step progress fractions like "2/5" inside bracketed stage descriptors.
	buildKitFraction = regexp.MustCompile(`(?:^|\s)(\d+)/(\d+)(?:\]|\s|$)`)

	// --- Docker Compose multiplexed log patterns ---

	// Matches Docker Compose v1 (`web_1 | msg`) and v2 (`project-web-1  | msg` or `web-1 │ msg`)
	// multiplexed container output. Requiring the replica suffix (`-\d+` or `_\d+`) avoids
	// false positives on arbitrary shell pipelines.
	dockerComposeLine = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_.-]*[-_]\d+)\s+[|│]\s?(.*?)(?:\r?\n)?$`)

	// Matches Docker Compose v2 lifecycle status lines:
	// e.g. " ✔ Container myapp-db-1  Healthy", " ⠿ Network myapp_default  Creating"
	dockerComposeLifecycle = regexp.MustCompile(`^\s*([✔⠿⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏+-])\s+(Container|Network|Volume)\s+(\S+)\s+(\S+)(?:\r?\n)?$`)

	// Matches container exit notices emitted during `docker compose up`:
	// e.g. "myapp-worker-1 exited with code 0" or "web-1 exited with code 137"
	dockerComposeExit = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_.-]*[-_]\d+)\s+exited with code\s+(\d+)(?:\r?\n)?$`)
)

// Curated high-contrast ANSI 256 colors for distinguishing Docker Compose services.
// Using a package-level slice avoids allocating a new slice on every log line.
var composePalette = []lipgloss.Color{
	lipgloss.Color("39"),  // Deep sky blue
	lipgloss.Color("42"),  // Spring green
	lipgloss.Color("214"), // Warm amber
	lipgloss.Color("141"), // Soft violet
	lipgloss.Color("45"),  // Bright cyan
	lipgloss.Color("204"), // Coral pink
	lipgloss.Color("114"), // Sage green
	lipgloss.Color("220"), // Gold
}

// Pre-allocated lipgloss styles for consistent, zero-allocation rendering.
var (
	dockerFinalErrorStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("9")) // Bright red

	dockerChildLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("63")) // Royal blue/purple

	dockerTreeBranchStyle = lipgloss.NewStyle().
				Faint(true)

	dockerStatusStepStyle = lipgloss.NewStyle().
				Faint(true)

	dockerSuccessBadgeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("10")) // Bright green

	composePipeStyle = lipgloss.NewStyle().
				Faint(true)

	composeExitOKStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")) // Bright green

	composeExitErrStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("9")) // Bright red
)

// dockerBuildSteps is a stateful parser/formatter (Imperative Shell state tracker)
// that normalizes BuildKit task IDs, legacy builder outputs, and Docker Compose
// multiplexed container streams into structured terminal output.
type dockerBuildSteps struct {
	numbers        map[string]int            // Maps internal BuildKit ID (or "legacy-N") -> sequential step number
	children       map[string]int            // Tracks count of child output lines per step ID
	totals         map[string]string         // Stores total steps in the current stage (e.g. "4" from "[2/4]")
	composeStyles  map[string]lipgloss.Style // Caches per-service styles in Docker Compose logs
	composeMaxName int                       // Tracks longest seen service name for vertical pipe alignment
	lastKey        string                    // Tracks the most recently opened step (used for legacy " ---> " lines)
	next           int                       // Monotonically increasing step counter
	separator      string                    // Pre-rendered horizontal divider line
	colors         DockerColors              // Configurable SGR color codes
}

// newDockerBuildSteps initializes internal maps so writes never panic on a nil map.
// In Go, reading from a nil map is safe, but writing to a nil map causes a runtime panic.
func newDockerBuildSteps() *dockerBuildSteps {
	return &dockerBuildSteps{
		numbers:       make(map[string]int),
		children:      make(map[string]int),
		totals:        make(map[string]string),
		composeStyles: make(map[string]lipgloss.Style),
		separator:     strings.Repeat("─", defaultStepSeparatorWidth),
		colors:        DefaultDockerColors(),
	}
}

// format inspects a single Docker/Compose log line. If it recognizes a build step,
// status transition, child output, Compose service line, or error, it returns the
// formatted terminal string and `true`. Otherwise, it returns `"", false`.
func (s *dockerBuildSteps) format(text string) (string, bool) {
	// 1. Final step failure summary (e.g. "STEP 3 ERROR: ...")
	if m := buildKitFinalError.FindStringSubmatch(text); m != nil {
		badge := dockerFinalErrorStyle.Render("✖ STEP " + m[1] + " ERROR")
		return badge + "  " + m[2] + "\n", true
	}

	// 2. Top-level BuildKit solver error (e.g. "ERROR: failed to solve: ...")
	if m := buildKitSolveError.FindStringSubmatch(text); m != nil {
		badge := dockerFinalErrorStyle.Render("✖ BUILD ERROR")
		return badge + "  " + m[1] + "\n", true
	}

	// 3. Step completion/cache/error status (e.g. "#7 DONE 0.4s", "#7 CACHED")
	if m := buildKitStatus.FindStringSubmatch(text); m != nil {
		if number, ok := s.numbers[m[1]]; ok {
			statusText := s.renderStatus(m[2])
			stepLabel := dockerStatusStepStyle.Render(fmt.Sprintf("STEP %d", number))
			return fmt.Sprintf("%s  %s%s\n", stepLabel, statusText, m[3]), true
		}
	}

	// 4. Step headers (BuildKit "#N [stage X/Y] CMD" or Legacy "Step X/Y : CMD").
	// Must be evaluated BEFORE buildKitChild so "#7 [4/4] RUN ..." is registered
	// as a new step rather than treated as child output of #7.
	if out, ok := s.tryFormatHeader(text); ok {
		return out, true
	}

	// 5. Legacy builder completion lines ("Successfully built ...", "Successfully tagged ...")
	if m := buildKitLegacySuccess.FindStringSubmatch(text); m != nil {
		badge := dockerSuccessBadgeStyle.Render("✔ " + m[1])
		return fmt.Sprintf("%s  %s\n", badge, highlightLine(m[2])), true
	}

	// 6. Legacy builder child lines (" ---> Using cache", " ---> Running in <id>")
	if m := buildKitLegacyChild.FindStringSubmatch(text); m != nil && s.lastKey != "" {
		return s.formatChildLine(s.lastKey, "", m[1]), true
	}

	// 7. BuildKit child progress/log lines ("#7 0.231 ...", "#7 sha256:...")
	if m := buildKitChild.FindStringSubmatch(text); m != nil {
		if _, ok := s.numbers[m[1]]; ok {
			return s.formatChildLine(m[1], m[2], m[3]), true
		}
	}

	// 8. Docker Compose multiplexed service logs ("web-1  | Listening on :3000")
	if m := dockerComposeLine.FindStringSubmatch(text); m != nil {
		return s.formatComposeLine(m[1], m[2]), true
	}

	// 9. Docker Compose container exit events ("web-1 exited with code 1")
	if m := dockerComposeExit.FindStringSubmatch(text); m != nil {
		return s.formatComposeExit(m[1], m[2]), true
	}

	// 10. Docker Compose lifecycle events (" ✔ Container app-db-1  Healthy")
	if m := dockerComposeLifecycle.FindStringSubmatch(text); m != nil {
		return s.formatComposeLifecycle(m[1], m[2], m[3], m[4]), true
	}

	return "", false
}

// tryFormatHeader detects both modern BuildKit and legacy Docker step declarations.
func (s *dockerBuildSteps) tryFormatHeader(text string) (string, bool) {
	var key, command, stage, total string

	if m := buildKitHeader.FindStringSubmatch(text); m != nil {
		key, command = m[1], m[3]
		stage, total = parseBuildKitBracket(m[2])
	} else if m := buildKitLegacyHeader.FindStringSubmatch(text); m != nil && buildKitInstruction.MatchString(m[3]) {
		key, command, total = "legacy-"+m[1], m[3], m[2]
	}

	if key == "" {
		return "", false
	}

	// Idempotency guard: BuildKit often re-emits the header line when streaming logs.
	// Returning ("", false) prevents duplicate STEP headers if already registered.
	if _, exists := s.numbers[key]; exists {
		return "", false
	}

	s.next++
	s.numbers[key] = s.next
	s.lastKey = key
	if total != "" {
		s.totals[key] = total
	}

	badgeText := fmt.Sprintf(" STEP %d ", s.next)
	if stage != "" {
		badgeText = fmt.Sprintf(" STEP %d · %s ", s.next, stage)
	}

	badge := renderSGR(badgeText, s.colors.BadgeBackground, s.colors.BadgeText)
	sep := renderSGR(s.separator, s.colors.Separator)

	coloredCmd := command
	if highlighted, err := SyntaxHighlight(command, "dockerfile"); err == nil {
		coloredCmd = strings.TrimRight(highlighted, "\r\n")
	}

	return fmt.Sprintf("%s  %s\n%s\n", badge, coloredCmd, sep), true
}

// formatChildLine formats indented sub-steps and command output under a parent STEP.
func (s *dockerBuildSteps) formatChildLine(key, elapsed, rawBody string) string {
	number := s.numbers[key]
	s.children[key]++

	total := s.totals[key]
	if total == "" {
		// strconv.Itoa is faster and allocates less than fmt.Sprint for integers.
		total = strconv.Itoa(number)
	}

	label := fmt.Sprintf("STEP %d.%d/%s", number, s.children[key], total)
	body := strings.TrimSpace(rawBody)
	if elapsed != "" {
		body = elapsed + "s  " + body
	}

	branch := dockerTreeBranchStyle.Render("  ╰── ")
	styledLabel := dockerChildLabelStyle.Render(label)

	return branch + styledLabel + "\n     " + highlightLine(body) + "\n"
}

// formatComposeLine renders a color-coded, column-aligned Docker Compose container log line.
func (s *dockerBuildSteps) formatComposeLine(service, payload string) string {
	style := s.composeStyleFor(service)
	padded := s.padComposeService(service)
	pipe := composePipeStyle.Render("│")

	return fmt.Sprintf("%s %s %s\n", style.Render(padded), pipe, highlightLine(payload))
}

// formatComposeExit formats container termination notices with semantic exit-code coloring.
func (s *dockerBuildSteps) formatComposeExit(service, code string) string {
	style := s.composeStyleFor(service)
	status := composeExitOKStyle.Render("exited with code " + code)
	if code != "0" {
		status = composeExitErrStyle.Render("exited with code " + code)
	}
	return fmt.Sprintf("  ● %s %s\n", style.Render(service), status)
}

// formatComposeLifecycle formats Docker Compose resource state changes (Created, Started, Healthy).
func (s *dockerBuildSteps) formatComposeLifecycle(symbol, kind, name, state string) string {
	style := s.composeStyleFor(name)
	stateStyle := dockerStatusStepStyle
	switch strings.ToLower(state) {
	case "healthy", "started", "running", "created":
		stateStyle = dockerSuccessBadgeStyle
	case "error", "unhealthy", "exited":
		stateStyle = dockerFinalErrorStyle
	}

	return fmt.Sprintf("  %s %s %s  %s\n",
		stateStyle.Render(symbol),
		dockerStatusStepStyle.Render(kind),
		style.Render(name),
		stateStyle.Render(state),
	)
}

// composeStyleFor deterministically assigns a lipgloss.Style to a service name
// using a fast non-cryptographic FNV-1a hash from the standard library (`hash/fnv`).
// Caching the resulting style in `s.composeStyles` avoids recomputing hashes on hot paths.
func (s *dockerBuildSteps) composeStyleFor(service string) lipgloss.Style {
	if style, ok := s.composeStyles[service]; ok {
		return style
	}

	h := fnv.New32a()
	// hash.Hash.Write never returns an error according to the Go standard library contract.
	_, _ = h.Write([]byte(service))
	color := composePalette[int(h.Sum32())%len(composePalette)]

	style := lipgloss.NewStyle().Bold(true).Foreground(color)
	s.composeStyles[service] = style
	return style
}

// padComposeService dynamically tracks the widest service name seen so far (up to
// `composeMaxServiceWidth`) so the vertical separator stays aligned across services.
func (s *dockerBuildSteps) padComposeService(service string) string {
	n := len(service)
	if n > composeMaxServiceWidth {
		return service[:composeMaxServiceWidth-1] + "…"
	}
	if n > s.composeMaxName {
		s.composeMaxName = n
	}
	return service + strings.Repeat(" ", s.composeMaxName-n)
}

// renderStatus applies the configured SGR color from DockerColors to a status keyword.
func (s *dockerBuildSteps) renderStatus(status string) string {
	color := s.colors.Done
	switch status {
	case "CACHED":
		color = s.colors.Cached
	case "ERROR":
		color = s.colors.Error
	}
	return renderSGR(status, color)
}

// parseBuildKitBracket extracts an optional stage/platform name and step total
// from bracket metadata like "4/4", "builder 2/5", or "linux/amd64 stage-1 1/3".
func parseBuildKitBracket(meta string) (stage, total string) {
	if fraction := buildKitFraction.FindStringSubmatch(meta); fraction != nil {
		total = fraction[2]
		// Strip the fraction suffix to isolate any stage or platform label.
		prefix := strings.TrimSpace(strings.TrimSuffix(meta, strings.TrimSpace(fraction[0])))
		if prefix != "" {
			// If both platform and stage are present (e.g. "linux/amd64 builder"),
			// keep the last token ("builder") to keep the badge concise.
			parts := strings.Fields(prefix)
			stage = parts[len(parts)-1]
		}
	}
	return stage, total
}

// renderSGR wraps text in raw ANSI SGR codes from DockerColors while respecting
// lipgloss's terminal color profile detection (disabling escapes when output is not a TTY).
func renderSGR(text string, codes ...string) string {
	// If lipgloss detected no color support (e.g. piped output or NO_COLOR=1),
	// return plain text so raw escape sequences don't pollute logs.
	if lipgloss.ColorProfile() == 0 { // termenv.Ascii == 0
		return text
	}

	var valid []string
	for _, code := range codes {
		if c := strings.TrimSpace(code); c != "" {
			valid = append(valid, c)
		}
	}
	if len(valid) == 0 {
		return text
	}

	return "\x1b[" + strings.Join(valid, ";") + "m" + text + "\x1b[0m"
}
