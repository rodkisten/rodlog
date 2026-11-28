package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPipe(t *testing.T) {
	var out, errs bytes.Buffer
	status := run([]string{"--color=never"}, strings.NewReader("npm ERR! broken\n"), &out, &errs)
	if status != 0 || out.String() != "npm ERR! broken\n" {
		t.Fatalf("status=%d output=%q error=%q", status, out.String(), errs.String())
	}
}
func TestInvalidMode(t *testing.T) {
	var out, errs bytes.Buffer
	if run([]string{"--mode=invalid"}, strings.NewReader(""), &out, &errs) != 2 {
		t.Fatal("expected invalid mode")
	}
}
func TestExecExitCode(t *testing.T) {
	var out, errs bytes.Buffer
	code := run([]string{"exec", "--color=never", "--", "sh", "-c", "echo 'npm ERR! bad'; exit 7"}, strings.NewReader(""), &out, &errs)
	if code != 7 || !strings.Contains(out.String(), "npm ERR! bad") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errs.String())
	}
}

func TestDockerSeparatorFlag(t *testing.T) {
	input := "#7 [build 1/2] RUN echo hello\n#7 DONE 0.1s\n#8 [build 2/2] RUN echo world\n"
	var out, errs bytes.Buffer
	status := run([]string{"--tool=docker", "--color=always", "--docker-separator=++++"}, strings.NewReader(input), &out, &errs)
	if status != 0 || !strings.Contains(out.String(), "++++") {
		t.Fatalf("status=%d output=%q error=%q", status, out.String(), errs.String())
	}
}

func TestDockerSeparatorEnvironment(t *testing.T) {
	t.Setenv("RODLOG_DOCKER_SEPARATOR", "~~~~")
	input := "#7 [build 1/2] RUN echo hello\n#7 DONE 0.1s\n#8 [build 2/2] RUN echo world\n"
	var out, errs bytes.Buffer
	status := run([]string{"--tool=docker", "--color=always"}, strings.NewReader(input), &out, &errs)
	if status != 0 || !strings.Contains(out.String(), "~~~~") {
		t.Fatalf("status=%d output=%q error=%q", status, out.String(), errs.String())
	}
}

func TestDockerSeparatorFlagOverridesEnvironment(t *testing.T) {
	t.Setenv("RODLOG_DOCKER_SEPARATOR", "ENV-DIVIDER")
	input := "#7 [build 1/2] RUN echo hello\n#7 DONE 0.1s\n#8 [build 2/2] RUN echo world\n"
	var out, errs bytes.Buffer
	status := run([]string{"--tool=docker", "--color=always", "--docker-separator=FLAG-DIVIDER"}, strings.NewReader(input), &out, &errs)
	if status != 0 || !strings.Contains(out.String(), "FLAG-DIVIDER") || strings.Contains(out.String(), "ENV-DIVIDER") {
		t.Fatalf("status=%d output=%q error=%q", status, out.String(), errs.String())
	}
}

func TestThemesAndInitConfigSubcommands(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"themes"}, strings.NewReader(""), &out, &errs); code != 0 || !strings.Contains(out.String(), "tokyo-night") {
		t.Fatalf("themes subcommand failed: code=%d out=%q err=%q", code, out.String(), errs.String())
	}
	out.Reset()
	if code := run([]string{"init-config"}, strings.NewReader(""), &out, &errs); code != 0 || !strings.Contains(out.String(), "[highlight]") {
		t.Fatalf("init-config subcommand failed: code=%d out=%q err=%q", code, out.String(), errs.String())
	}
}


func TestColorAlwaysUsesTokyoNightOnNonTTY(t *testing.T) {
	input := "#7 [build 1/2] RUN pnpm install\n#7 0.1 Progress: resolved 9, reused 8, downloaded 1, added 9\n"
	var out, errs bytes.Buffer
	status := run([]string{
		"--color=always",
		"--mode=transform",
		"--tool=auto",
		"--symbols=unicode",
	}, strings.NewReader(input), &out, &errs)
	if status != 0 {
		t.Fatalf("status=%d err=%q", status, errs.String())
	}
	got := out.String()
	for _, want := range []string{
		"\x1b[38;2;187;154;247m", // Tokyo Night semantic code/step.
		"\x1b[38;2;122;162;247m", // Tokyo Night semantic info.
		"\x1b[2m0.1s\x1b[0m",   // Duration stays faint.
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("forced color missing %q from non-TTY output: %q", want, got)
		}
	}
}
