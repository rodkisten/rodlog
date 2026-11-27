package rodlog

import (
	"regexp"
	"strings"
)

var dockerfileInstruction = regexp.MustCompile(`(?i)^(?:FROM|RUN|COPY|ADD|WORKDIR|ENV|ARG|EXPOSE|ENTRYPOINT|CMD|USER|LABEL|VOLUME|HEALTHCHECK|SHELL|STOPSIGNAL|ONBUILD)\s+\S`)
var dockerBuildStep = regexp.MustCompile(`^#\d+\s+(?:\[[^]]+\]\s*)?(?:CACHED|DONE|ERROR|FINISHED|exporting|transferring|loading|resolving|extracting|importing|writing|naming)\b`)
var dockerBuildStatus = regexp.MustCompile(`\b(?:CACHED|DONE|ERROR|FINISHED)\b`)
var dockerBuildActivity = regexp.MustCompile(`\b(?:exporting|transferring|loading|resolving|extracting|importing|writing|naming)\b`)

var dockerEvents = []struct {
	pattern *regexp.Regexp
	color   string
}{
	{regexp.MustCompile(`(?i)\b(?:healthy|health: healthy|started|running|created|pulled|ready|listening)\b`), "32"},
	{regexp.MustCompile(`(?i)\b(?:unhealthy|health: unhealthy|oomkilled|out of memory|permission denied|connection refused|exited \(\d+\)|fatal)\b`), "31"},
	{regexp.MustCompile(`(?i)\b(?:restarting|stopping|waiting|retrying|health: starting)\b`), "33"},
}

func highlightDocker(text string) (string, bool) {
	if dockerBuildStep.MatchString(text) {
		if loc := dockerBuildStatus.FindStringIndex(text); loc != nil {
			color := "36"
			switch text[loc[0]:loc[1]] {
			case "CACHED":
				color = "2"
			case "DONE", "FINISHED":
				color = "32"
			case "ERROR":
				color = "31"
			}
			return colorDockerSpan(text, loc, color), true
		}
		if loc := dockerBuildActivity.FindStringIndex(text); loc != nil {
			return colorDockerSpan(text, loc, "36"), true
		}
	}
	for _, rule := range dockerEvents {
		if loc := rule.pattern.FindStringIndex(text); loc != nil {
			return colorDockerSpan(text, loc, rule.color), true
		}
	}
	return "", false
}

func colorDockerSpan(text string, loc []int, color string) string {
	var b strings.Builder
	b.WriteString(text[:loc[0]])
	b.WriteString("\x1b[")
	b.WriteString(color)
	b.WriteString("m")
	b.WriteString(text[loc[0]:loc[1]])
	b.WriteString("\x1b[0m")
	b.WriteString(text[loc[1]:])
	return b.String()
}
