package rodlog

import (
 "encoding/json"
 "os"
	"regexp"
	"strconv"
	"strings"
)

type Diagnostic struct {
	Tool    Tool   `json:"tool"`
	Level   Level  `json:"level"`
	Code    string `json:"code,omitempty"`
	File        string `json:"file,omitempty"`
	DisplayFile string `json:"display_file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
	SourceLines []string `json:"source_lines,omitempty"`
 ContextBefore *int `json:"context_before,omitempty"`
 ContextAfter *int `json:"context_after,omitempty"`
 Spans []DiagnosticSpan `json:"spans,omitempty"`
	Notes []string `json:"notes,omitempty"`
	Help []string `json:"help,omitempty"`
	Related []*Diagnostic `json:"related,omitempty"`
 Signature string `json:"signature,omitempty"`
 SignatureFocus string `json:"signature_focus,omitempty"`
 SignatureFocusStart int `json:"signature_focus_start,omitempty"`
 SignatureFocusEnd int `json:"signature_focus_end,omitempty"`
 SignatureFocusLabel string `json:"signature_focus_label,omitempty"`
 Documentation string `json:"documentation,omitempty"`
}

var (
	tsLocation      = regexp.MustCompile(`^(.+?)\((\d+),(\d+)\):\s*(error|warning)\s+(TS\d+):\s*(.+)$`)
	tsColon         = regexp.MustCompile(`^(.+?):(\d+):(\d+)\s*[-:]\s*(error|warning)\s+(TS\d+):\s*(.+)$`)
	tsPlain         = regexp.MustCompile(`^(error|warning)\s+(TS\d+):\s*(.+)$`)
	npmPrefix       = regexp.MustCompile(`(?i)^npm\s+(ERR!|error|WARN|warn|notice)\s*(.*)$`)
	gcloudPrefix    = regexp.MustCompile(`^(ERROR|WARNING):\s*(?:\(gcloud(?:\.[^)]+)?\)\s*)?(.+)$`)
	dockerPrefix    = regexp.MustCompile(`(?i)^(?:ERROR:\s*)?(?:failed to solve|failed to compute cache key|dockerfile parse error):\s*(.+)$`)
	dockerLocation  = regexp.MustCompile(`(?i)^Dockerfile:(\d+)(?::(\d+))?\b`)
	wranglerPrefix  = regexp.MustCompile(`^(?:✘\s*)?\[(ERROR|WARNING)\]\s*(.+)$`)
	runnerAssertion = regexp.MustCompile(`(?i)^(?:AssertionError:\s*)?expected\s+(.+?)\s+to\s+(?:deeply\s+)?(?:equal|be)\s+(.+?)\.?$`)
	vitestFailure   = regexp.MustCompile(`^\s*(?:❯|✖|×)\s+(.+)$`)
	nodeTestFailure = regexp.MustCompile(`^\s*(?:✖\s+|not ok\s+\d+\s*-\s*)(.+)$`)
	wranglerError   = regexp.MustCompile(`(?i)^(?:✘\s*)?(?:error|warning):\s*(.+)$`)
)

func ParseDiagnostic(tool Tool, input string) *Diagnostic {
	line := strings.TrimSpace(ansiPattern.ReplaceAllString(input, ""))
	if line == "" {
		return nil
	}
	if tool == Auto || tool == Generic {
		tool = Detect(line)
	}
	d := &Diagnostic{Tool: tool, Level: Error}
 for key, target := range map[string]**int{"RODLOG_CONTEXT_BEFORE": &d.ContextBefore, "RODLOG_CONTEXT_AFTER": &d.ContextAfter} { if n,err:=strconv.Atoi(os.Getenv(key)); err==nil { *target=&n } }
	switch tool {
	case Rust:
		return parseRustDiagnostic(line)
	case PNPM:
		if m := pnpmDiagnostic.FindStringSubmatch(line); m != nil {
			d.Code, d.Message = m[1], m[2]
			if m[1] == "WARN" { d.Level = Warning }
			return d
		}
	case GoCompiler:
		if m := goDiagnostic.FindStringSubmatch(line); m != nil {
			d.File, d.Message = m[1], m[4]
			d.Line, _ = strconv.Atoi(m[2])
			d.Column, _ = strconv.Atoi(m[3])
			d.Spans = []DiagnosticSpan{{File:d.File, Line:d.Line, Column:d.Column, Primary:true}}
			return d
		}
	case TypeScript:
		if strings.HasPrefix(line,"{") { var structured Diagnostic; if json.Unmarshal([]byte(line), &structured)==nil && structured.Tool==TypeScript && structured.Code!="" && structured.Message!="" { if d.ContextBefore!=nil { structured.ContextBefore=d.ContextBefore }; if d.ContextAfter!=nil { structured.ContextAfter=d.ContextAfter }; return &structured } }
		for _, re := range []*regexp.Regexp{tsLocation, tsColon} {
			if m := re.FindStringSubmatch(line); m != nil {
				d.File = m[1]
				d.Line, _ = strconv.Atoi(m[2])
				d.Column, _ = strconv.Atoi(m[3])
				if m[4] == "warning" {
					d.Level = Warning
				}
				d.Code, d.Message = m[5], m[6]
				d.Spans = []DiagnosticSpan{{File: d.File, Line: d.Line, Column: d.Column, Primary: true}}
				return d
			}
		}
		if m := tsPlain.FindStringSubmatch(line); m != nil {
			if m[1] == "warning" {
				d.Level = Warning
			}
			d.Code, d.Message = m[2], m[3]
			return d
		}
	case NPM:
		if m := npmPrefix.FindStringSubmatch(line); m != nil {
			d.Message = strings.TrimSpace(m[2])
			switch strings.ToLower(m[1]) {
			case "warn":
				d.Level = Warning
			case "notice":
				d.Level = Info
			}
			if d.Message != "" {
				return d
			}
		}
	case GCloud:
		if m := gcloudPrefix.FindStringSubmatch(line); m != nil {
			if m[1] == "WARNING" {
				d.Level = Warning
			}
			d.Message = m[2]
			return d
		}
	case Docker:
		if m := dockerPrefix.FindStringSubmatch(line); m != nil {
			d.Message = m[1]
			return d
		}
		if m := dockerLocation.FindStringSubmatch(line); m != nil {
			d.File = "Dockerfile"
			d.Line, _ = strconv.Atoi(m[1])
			if m[2] != "" {
				d.Column, _ = strconv.Atoi(m[2])
			}
			d.Message = line
			return d
		}
	case Vitest, NodeTest:
		if m := runnerAssertion.FindStringSubmatch(line); m != nil {
			d.Message = line
			return d
		}
		if tool == Vitest {
			if m := vitestFailure.FindStringSubmatch(line); m != nil {
				d.Message = m[1]
				return d
			}
		} else if m := nodeTestFailure.FindStringSubmatch(line); m != nil {
			d.Message = m[1]
			return d
		}
	case Wrangler:
		if m := wranglerPrefix.FindStringSubmatch(line); m != nil {
			if m[1] == "WARNING" {
				d.Level = Warning
			}
			d.Message = m[2]
			return d
		}
		if m := wranglerError.FindStringSubmatch(line); m != nil {
			if strings.Contains(strings.ToLower(line), "warning:") {
				d.Level = Warning
			}
			d.Message = m[1]
			return d
		}
	}
	return nil
}
