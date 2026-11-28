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

var (
	buildKitHeader         = regexp.MustCompile(`^#(\d+)\s+\[([^\]]+)\]\s+(.+?)(?:\r?\n)?$`)
	buildKitLegacyHeader   = regexp.MustCompile(`^Step\s+(\d+)/(\d+)\s*:\s*(.+?)(?:\r?\n)?$`)
	buildKitStatus         = regexp.MustCompile(`^#(\d+)\s+(DONE|CACHED|ERROR|FINISHED)([^\r\n]*)(?:\r?\n)?$`)
	buildKitChild          = regexp.MustCompile(`^#(\d+)\s+(?:(\d+(?:\.\d+)?)\s+)?(.+?)(?:\r?\n)?$`)
	buildKitFinalError     = regexp.MustCompile(`(?i)^STEP\s+(\d+)\s+ERROR:\s*(.+?)(?:\r?\n)?$`)
	buildKitSolveError     = regexp.MustCompile(`^ERROR:\s+(failed to solve:.+?)(?:\r?\n)?$`)
	buildKitLegacyChild    = regexp.MustCompile(`^\s*--->\s+(.+?)(?:\r?\n)?$`)
	buildKitLegacySuccess  = regexp.MustCompile(`^(Successfully\s+(?:built|tagged))\s+(.+?)(?:\r?\n)?$`)
	buildKitInstruction    = regexp.MustCompile(`(?i)^(?:RUN|COPY|ADD|FROM|WORKDIR|ENV|ARG|CMD|ENTRYPOINT|USER|EXPOSE|LABEL|HEALTHCHECK|SHELL|VOLUME|ONBUILD|STOPSIGNAL|MAINTAINER)\b`)
	buildKitFraction       = regexp.MustCompile(`(?:^|\s)(\d+)/(\d+)(?:\]|\s|$)`)
	dockerComposeLine      = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_.-]*[-_]\d+)\s+[|│]\s?(.*?)(?:\r?\n)?$`)
	dockerComposeLifecycle = regexp.MustCompile(`^\s*([✔⠿⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏+-])\s+(Container|Network|Volume)\s+(\S+)\s+(\S+)(?:\r?\n)?$`)
	dockerComposeExit      = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_.-]*[-_]\d+)\s+exited with code\s+(\d+)(?:\r?\n)?$`)
)

var composePalette = []lipgloss.Color{
	lipgloss.Color("39"),
	lipgloss.Color("42"),
	lipgloss.Color("214"),
	lipgloss.Color("141"),
	lipgloss.Color("45"),
	lipgloss.Color("204"),
	lipgloss.Color("114"),
	lipgloss.Color("220"),
}

var (
	dockerFinalErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	dockerChildLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	dockerTreeBranchStyle   = lipgloss.NewStyle().Faint(true)
	dockerStatusStepStyle   = lipgloss.NewStyle().Faint(true)
	dockerSuccessBadgeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	composePipeStyle        = lipgloss.NewStyle().Faint(true)
	composeExitOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	composeExitErrStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
)

type dockerBuildSteps struct {
	numbers        map[string]int
	children       map[string]int
	totals         map[string]string
	composeStyles  map[string]lipgloss.Style
	composeMaxName int
	lastKey        string
	next           int
	separator      string
	colors         DockerColors
	opts           Options
	stepTools      map[string]Tool
	activeTools    map[string]Tool
}

func newDockerBuildSteps() *dockerBuildSteps {
	return &dockerBuildSteps{
		numbers:       make(map[string]int),
		children:      make(map[string]int),
		totals:        make(map[string]string),
		composeStyles: make(map[string]lipgloss.Style),
		separator:     strings.Repeat("─", defaultStepSeparatorWidth),
		colors:        DefaultDockerColors(),
		stepTools:     make(map[string]Tool),
		activeTools:   make(map[string]Tool),
	}
}

func (s *dockerBuildSteps) format(text string) (string, bool) {
	// BuildKit reuses step IDs across independent builds.
	if strings.HasPrefix(text, "#0 building with ") {
		clear(s.numbers)
		clear(s.children)
		clear(s.totals)
		clear(s.stepTools)
		clear(s.activeTools)
		s.next = 0
		s.lastKey = ""
		return text, true
	}

	if m := buildKitFinalError.FindStringSubmatch(text); m != nil {
		badge := dockerFinalErrorStyle.Render("✖ STEP " + m[1] + " ERROR")
		return badge + "  " + m[2] + "\n", true
	}

	if m := buildKitSolveError.FindStringSubmatch(text); m != nil {
		badge := dockerFinalErrorStyle.Render("✖ BUILD ERROR")
		return badge + "  " + m[1] + "\n", true
	}

	if m := buildKitStatus.FindStringSubmatch(text); m != nil {
		if number, ok := s.numbers[m[1]]; ok {
			statusText := s.renderStatus(m[2])
			stepLabel := dockerStatusStepStyle.Render(fmt.Sprintf("STEP %d", number))
			return fmt.Sprintf("%s  %s%s\n", stepLabel, statusText, m[3]), true
		}
	}

	if out, ok := s.tryFormatHeader(text); ok {
		return out, true
	}

	if m := buildKitLegacySuccess.FindStringSubmatch(text); m != nil {
		badge := dockerSuccessBadgeStyle.Render("✔ " + m[1])
		return fmt.Sprintf("%s  %s\n", badge, highlightLine(m[2])), true
	}

	if m := buildKitLegacyChild.FindStringSubmatch(text); m != nil && s.lastKey != "" {
		return s.formatChildLine(s.lastKey, "", m[1]), true
	}

	if m := buildKitChild.FindStringSubmatch(text); m != nil {
		if _, ok := s.numbers[m[1]]; ok {
			return s.formatChildLine(m[1], m[2], m[3]), true
		}
	}

	if m := dockerComposeLine.FindStringSubmatch(text); m != nil {
		return s.formatComposeLine(m[1], m[2]), true
	}

	if m := dockerComposeExit.FindStringSubmatch(text); m != nil {
		return s.formatComposeExit(m[1], m[2]), true
	}

	if m := dockerComposeLifecycle.FindStringSubmatch(text); m != nil {
		return s.formatComposeLifecycle(m[1], m[2], m[3], m[4]), true
	}

	return "", false
}

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

	if _, exists := s.numbers[key]; exists {
		// Preserve repeated headers without emitting another card.
		return text, true
	}

	s.next++
	s.numbers[key] = s.next
	s.lastKey = key
	if tool := detectDockerRunTool(command); tool != Generic {
		s.stepTools[key] = tool
		s.activeTools[key] = tool
	}
	if total != "" {
		s.totals[key] = total
	}

	badgeText := fmt.Sprintf(" STEP %d ", s.next)
	if stage != "" && stage != "internal" && stage != "build" && stage != "builder" {
		badgeText = fmt.Sprintf(" STEP %d · %s ", s.next, stage)
	}

	badge := renderSGR(badgeText, s.colors.BadgeBackground, s.colors.BadgeText)
	if tool := s.stepTools[key]; tool != Generic {
		badge += "  " + wrapANSI(semanticColor(s.opts.SemanticColors.Info, "36"), pnpmNestedLabel(tool))
	}
	sep := renderSGR(s.separator, s.colors.Separator)

	coloredCmd := command
	if highlighted, err := SyntaxHighlight(command, "dockerfile"); err == nil {
		coloredCmd = strings.TrimRight(highlighted, "\r\n")
	}

	return fmt.Sprintf("%s  %s\n%s\n", badge, coloredCmd, sep), true
}

func (s *dockerBuildSteps) formatChildLine(key, elapsed, rawBody string) string {
	number := s.numbers[key]
	s.children[key]++

	total := s.totals[key]
	if total == "" {
		total = strconv.Itoa(number)
	}

	label := fmt.Sprintf("STEP %d.%d/%s", number, s.children[key], total)
	body := strings.TrimSpace(rawBody)

	tool := s.activeTools[key]
	if detected := detectNestedDockerTool(body); detected != Generic && detected != Docker {
		tool = detected
		s.activeTools[key] = detected
	}
	if tool == Generic {
		tool = s.stepTools[key]
	}

	branch := dockerTreeBranchStyle.Render("  ╰── ")
	styledLabel := dockerChildLabelStyle.Render(label)
	if tool != Generic {
		styledLabel += "  " + wrapANSI(semanticColor(s.opts.SemanticColors.Info, "36"), pnpmNestedLabel(tool))
	}

	rendered := renderDockerNestedBody(tool, body, s.opts)
	if elapsed != "" {
		rendered = wrapANSI("2", elapsed+"s") + "  " + rendered
	}
	return branch + styledLabel + "\n" + indentDockerNested(rendered) + "\n"
}

func detectDockerRunTool(command string) Tool {
	command = strings.TrimSpace(command)
	if len(command) < 4 || !strings.EqualFold(command[:3], "RUN") || (len(command) > 3 && command[3] != ' ' && command[3] != '\t') {
		return Generic
	}
	source := strings.TrimSpace(command[3:])
	if analysis, err := AnalyzeShell(source, "sh"); err == nil {
		for _, cmd := range analysis.Commands {
			if cmd.Tool != Generic && cmd.Tool != Docker {
				return cmd.Tool
			}
		}
	}
	return Generic
}

func detectNestedDockerTool(body string) Tool {
	clean := strings.TrimSpace(ansiPattern.ReplaceAllString(body, ""))
	if strings.HasPrefix(clean, "> ") {
		command := strings.TrimSpace(strings.TrimPrefix(clean, "> "))
		if analysis, err := AnalyzeShell(command, "sh"); err == nil {
			for _, cmd := range analysis.Commands {
				if cmd.Tool != Generic && cmd.Tool != Docker {
					return cmd.Tool
				}
			}
		}
	}
	if isPNPMOutput(clean) {
		return PNPM
	}
	if detected := Detect(clean); detected != Docker {
		return detected
	}
	return Generic
}

func renderDockerNestedBody(tool Tool, body string, opts Options) string {
	if tool == Generic {
		return strings.TrimRight(highlightLine(body), "\r\n")
	}
	if diag := ParseDiagnostic(tool, body); diag != nil && opts.Mode == Transform {
		return strings.TrimRight(string(renderLinkedDiagnostic(diag, opts)), "\r\n")
	}
	event := Event{Tool: tool, Level: Classify(body), Text: body + "\n"}
	return strings.TrimRight(string(Render(event, opts)), "\r\n")
}

func indentDockerNested(rendered string) string {
	lines := strings.Split(strings.TrimRight(rendered, "\r\n"), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = "     " + lines[i]
		} else {
			lines[i] = "     │ " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func (s *dockerBuildSteps) formatComposeLine(service, payload string) string {
	style := s.composeStyleFor(service)
	padded := s.padComposeService(service)
	pipe := composePipeStyle.Render("│")

	return fmt.Sprintf("%s %s %s\n", style.Render(padded), pipe, highlightLine(payload))
}

func (s *dockerBuildSteps) formatComposeExit(service, code string) string {
	style := s.composeStyleFor(service)
	status := composeExitOKStyle.Render("exited with code " + code)
	if code != "0" {
		status = composeExitErrStyle.Render("exited with code " + code)
	}
	return fmt.Sprintf("  ● %s %s\n", style.Render(service), status)
}

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

func (s *dockerBuildSteps) composeStyleFor(service string) lipgloss.Style {
	if style, ok := s.composeStyles[service]; ok {
		return style
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(service))
	color := composePalette[int(h.Sum32())%len(composePalette)]

	style := lipgloss.NewStyle().Bold(true).Foreground(color)
	s.composeStyles[service] = style
	return style
}

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

func parseBuildKitBracket(meta string) (stage, total string) {
	if fraction := buildKitFraction.FindStringSubmatch(meta); fraction != nil {
		total = fraction[2]
		prefix := strings.TrimSpace(strings.TrimSuffix(meta, strings.TrimSpace(fraction[0])))
		if prefix != "" {
			parts := strings.Fields(prefix)
			stage = parts[len(parts)-1]
		}
	}
	return stage, total
}

func renderSGR(text string, codes ...string) string {
	if lipgloss.ColorProfile() == 0 {
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

	return "\x1b[" + strings.Join(valid, "m\x1b[") + "m" + text + "\x1b[0m"
}
