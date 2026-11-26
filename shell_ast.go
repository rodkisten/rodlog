package rodlog

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ShellCommand describes a command invocation extracted from a real shell AST.
type ShellCommand struct {
	Name   string   `json:"name"`
	Args   []string `json:"args,omitempty"`
	Line   uint     `json:"line"`
	Column uint     `json:"column"`
	Tool   Tool     `json:"tool"`
}

// ShellAnalysis contains syntax-aware command invocations without executing them.
type ShellAnalysis struct {
	Commands []ShellCommand `json:"commands"`
}

// AnalyzeShell parses POSIX shell, Bash, mksh, or Zsh using mvdan's syntax AST.
// Dynamic expansions ($VAR, $(...), globs) are intentionally not resolved or executed.
func AnalyzeShell(source string, dialect string) (ShellAnalysis, error) {
	variant, err := resolveShellVariant(dialect)
	if err != nil {
		return ShellAnalysis{}, err
	}

	// KeepComments(true) preserves accurate Line/Column positions even when comments
	// precede commands, without affecting command AST nodes.
	parser := syntax.NewParser(syntax.Variant(variant), syntax.KeepComments(true))
	file, err := parser.Parse(strings.NewReader(source), "script")
	if err != nil {
		return ShellAnalysis{}, err
	}

	// Initialize an empty non-nil slice so JSON marshaling produces `[]` instead of `null`.
	result := ShellAnalysis{Commands: []ShellCommand{}}

	// syntax.Walk traverses the entire AST depth-first, including commands nested inside
	// pipelines (a | b), subshells ((...)), if/for/while blocks, and $(...) substitutions.
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		name, ok := literalShellWord(call.Args[0])
		if !ok || name == "" {
			return true
		}

		// Pre-allocate slice capacity based on remaining AST word arguments.
		args := make([]string, 0, len(call.Args)-1)
		for _, arg := range call.Args[1:] {
			if literal, ok := literalShellWord(arg); ok {
				args = append(args, literal)
			}
		}

		// Resolve the underlying tool even when invoked via wrappers like `sudo`, `env`, or `time`.
		effectiveCmd := unwrapCommandWrapper(name, args)

		result.Commands = append(result.Commands, ShellCommand{
			Name:   name,
			Args:   args,
			Line:   call.Pos().Line(),
			Column: call.Pos().Col(),
			Tool:   DetectCommand(effectiveCmd),
		})

		return true
	})

	return result, nil
}

// resolveShellVariant maps user-facing dialect names to mvdan.cc/sh/v3 LangVariant constants.
func resolveShellVariant(dialect string) (syntax.LangVariant, error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "bash":
		return syntax.LangBash, nil
	case "sh", "posix":
		return syntax.LangPOSIX, nil
	case "mksh":
		return syntax.LangMirBSDKorn, nil
	case "zsh":
		// Supported in mvdan.cc/sh/v3 >= v3.9.0.
		return syntax.LangZsh, nil
	default:
		return 0, fmt.Errorf("unsupported shell dialect %q", dialect)
	}
}

// literalShellWord extracts a static string from an AST Word node if and only if
// all of its constituent parts are purely literal (no parameter expansions, command
// substitutions, arithmetic, or unquoted globs).
//
// Examples that succeed: `npm`, `"npm"`, `'vitest'`, `foo"bar"`
// Examples that return ("", false): `$CMD`, `"$HOME/bin"`, `$(which node)`, `*.go`
func literalShellWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}

	// Fast path for the most common case: a single unquoted or single-quoted literal.
	if len(word.Parts) == 1 {
		switch part := word.Parts[0].(type) {
		case *syntax.Lit:
			return part.Value, true
		case *syntax.SglQuoted:
			return part.Value, true
		}
	}

	var b strings.Builder
	for _, part := range word.Parts {
		if !appendLiteralWordPart(&b, part) {
			return "", false
		}
	}
	return b.String(), true
}

// appendLiteralWordPart appends static text from a single syntax.WordPart into `b`.
// Returns false if the part contains any dynamic shell expansion.
func appendLiteralWordPart(b *strings.Builder, part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		b.WriteString(p.Value)
		return true

	case *syntax.SglQuoted:
		// Single-quoted strings in POSIX/Bash never perform expansions.
		b.WriteString(p.Value)
		return true

	case *syntax.DblQuoted:
		// Double-quoted strings are static only if every inner part is static
		// (e.g. `"hello world"` is static, whereas `"hello $USER"` is dynamic).
		for _, inner := range p.Parts {
			if !appendLiteralWordPart(b, inner) {
				return false
			}
		}
		return true

	default:
		// Reject *syntax.ParamExp ($VAR), *syntax.CmdSubst ($(...)),
		// *syntax.ArithmExp ($((...))), *syntax.ProcSubst, *syntax.ExtGlob, etc.
		return false
	}
}

// unwrapCommandWrapper strips transparent command prefixes (`env`, `sudo`, `time`,
// `exec`, `nohup`, `command`) and inline `KEY=VAL` arguments passed to `env` so
// `DetectCommand` can identify the real target tool (e.g. `sudo npm test` -> `["npm", "test"]`).
func unwrapCommandWrapper(name string, args []string) []string {
	cmd := make([]string, 0, 1+len(args))
	cmd = append(cmd, name)
	cmd = append(cmd, args...)

	for len(cmd) > 1 {
		switch cmd[0] {
		case "time", "nohup", "exec":
			cmd = cmd[1:]

		case "sudo", "command", "builtin":
			// Skip wrapper and any leading flags like `sudo -E` or `command -v`.
			next := 1
			for next < len(cmd) && strings.HasPrefix(cmd[next], "-") {
				next++
			}
			if next >= len(cmd) {
				return cmd
			}
			cmd = cmd[next:]

		case "env":
			// Skip `env`, flags (`-i`, `-u`), and `KEY=VALUE` assignments.
			next := 1
			for next < len(cmd) && (strings.HasPrefix(cmd[next], "-") || strings.Contains(cmd[next], "=")) {
				next++
			}
			if next >= len(cmd) {
				return cmd
			}
			cmd = cmd[next:]

		default:
			return cmd
		}
	}

	return cmd
}
