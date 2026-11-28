package rodlog

import (
	"encoding/json"
	"regexp"
	"strings"
)

var pnpmDiagnostic = regexp.MustCompile(`^(?:\s|\x{2009})*(ERR_PNPM_[A-Z0-9_]+|WARN(?:_PNPM_[A-Z0-9_]+)?)(?:\s|\x{2009})+(.+)$`)
var goDiagnostic = regexp.MustCompile(`^(.+\.go):(\d+):(\d+):\s*(.+)$`)

type rustSpan struct {
	File string `json:"file_name"`
	Line int `json:"line_start"`
	Column int `json:"column_start"`
	EndColumn int `json:"column_end"`
	Primary bool `json:"is_primary"`
	Label string `json:"label"`
	Text []struct {
		Text string `json:"text"`
		Start int `json:"highlight_start"`
		End int `json:"highlight_end"`
	} `json:"text"`
}

type rustMessage struct {
	Message string `json:"message"`
	Level string `json:"level"`
	Code *struct { Code string `json:"code"` } `json:"code"`
	Spans []rustSpan `json:"spans"`
	Children []rustMessage `json:"children"`
}

func parseRustDiagnostic(input string) *Diagnostic {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "{") { return nil }
	var envelope struct {
		Reason string `json:"reason"`
		Type string `json:"$message_type"`
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal([]byte(input), &envelope) != nil { return nil }
	var msg rustMessage
	switch {
	case envelope.Reason == "compiler-message":
		if json.Unmarshal(envelope.Message, &msg) != nil { return nil }
	case envelope.Type == "diagnostic":
		if json.Unmarshal([]byte(input), &msg) != nil { return nil }
	default:
		return nil
	}
	if msg.Message == "" { return nil }
	return normalizeRustMessage(msg)
}

func normalizeRustMessage(msg rustMessage) *Diagnostic {
	d := &Diagnostic{Tool: Rust, Level: Info, Message: msg.Message}
	switch msg.Level {
	case "error", "failure-note", "ice":
		d.Level = Error
	case "warning":
		d.Level = Warning
	}
	if msg.Code != nil { d.Code = msg.Code.Code }
	for _, span := range msg.Spans {
		if span.Primary && d.File == "" {
			d.File, d.Line, d.Column = span.File, span.Line, span.Column
		}
		if len(span.Text) == 0 {
			d.Spans = append(d.Spans, DiagnosticSpan{File:span.File, Line:span.Line, Column:span.Column, EndColumn:span.EndColumn, Primary:span.Primary, Label:span.Label})
		}
		for i, line := range span.Text {
			d.Spans = append(d.Spans, DiagnosticSpan{File:span.File, Line:span.Line+i, Column:line.Start, EndColumn:line.End, Source:line.Text, Primary:span.Primary, Label:span.Label})
		}
	}
	for _, child := range msg.Children {
		switch child.Level {
		case "help":
			d.Help = append(d.Help, child.Message)
		case "note":
			d.Notes = append(d.Notes, child.Message)
		}
		if len(child.Spans) > 0 { d.Spans = append(d.Spans, normalizeRustMessage(child).Spans...) }
	}
	return d
}
