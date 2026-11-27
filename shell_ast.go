package rodlog

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type ShellCommand struct {
	Name   string   `json:"name"`
	Args   []string `json:"args,omitempty"`
	Line   uint     `json:"line"`
	Column uint     `json:"column"`
	Tool   Tool     `json:"tool"`
}

type ShellAnalysis struct {
	Commands []ShellCommand `json:"commands"`
}

func AnalyzeShell(source string, dialect string) (ShellAnalysis, error) {
	variant, err := resolveShellVariant(dialect)
	if err != nil {
		return ShellAnalysis{}, err
	}

	parser := syntax.NewParser(syntax.Variant(variant), syntax.KeepComments(true))
	file, err := parser.Parse(strings.NewReader(source), "script")
	if err != nil {
		return ShellAnalysis{}, err
	}

	result := ShellAnalysis{Commands: []ShellCommand{}}

	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		name, ok := literalShellWord(call.Args[0])
		if !ok || name == "" {
			return true
		}

		args := make([]string, 0, len(call.Args)-1)
		for _, arg := range call.Args[1:] {
			if literal, ok := literalShellWord(arg); ok {
				args = append(args, literal)
			}
		}

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

func resolveShellVariant(dialect string) (syntax.LangVariant, error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "bash":
		return syntax.LangBash, nil
	case "sh", "posix":
		return syntax.LangPOSIX, nil
	case "mksh":
		return syntax.LangMirBSDKorn, nil
	case "zsh":
		return 0, fmt.Errorf("zsh syntax is not supported by the shell AST parser")
	default:
		return 0, fmt.Errorf("unsupported shell dialect %q", dialect)
	}
}

func literalShellWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
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

func appendLiteralWordPart(b *strings.Builder, part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		b.WriteString(p.Value)
		return true
	case *syntax.SglQuoted:
		b.WriteString(p.Value)
		return true
	case *syntax.DblQuoted:
		for _, inner := range p.Parts {
			if !appendLiteralWordPart(b, inner) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func unwrapCommandWrapper(name string, args []string) []string {
	cmd := make([]string, 0, 1+len(args))
	cmd = append(cmd, name)
	cmd = append(cmd, args...)

	for len(cmd) > 1 {
		switch cmd[0] {
		case "time", "nohup", "exec":
			cmd = cmd[1:]
		case "sudo", "command", "builtin":
			next := 1
			for next < len(cmd) && strings.HasPrefix(cmd[next], "-") {
				next++
			}
			if next >= len(cmd) {
				return cmd
			}
			cmd = cmd[next:]
		case "env":
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
