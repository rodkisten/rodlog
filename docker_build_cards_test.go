package rodlog

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDockerBuildStepCards(t *testing.T) {
	input := "#9 [builder 1/3] FROM node:22\n#9 DONE 0.1s\n#12 [builder 2/3] RUN npm ci\n#12 [builder 2/3] RUN npm ci\n#12 DONE 2.3s\n"
	var output bytes.Buffer
	if err := New(Options{Mode: Enhance, Tool: Docker, Color: true}).Process(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, expected := range []string{"STEP 1", "STEP 2", "FROM", "RUN", "DONE", "────────────────"} {
		if !strings.Contains(got, expected) {
			t.Errorf("missing %q in %q", expected, got)
		}
	}
	if strings.Contains(got, "#9 [builder") {
		t.Errorf("raw header leaked: %q", got)
	}
	plain := ansiPattern.ReplaceAllString(got, "")
	if strings.Count(plain, "STEP 2") != 2 {
		t.Errorf("expected one STEP 2 header plus one status footer: %q", plain)
	}
	if !strings.Contains(ansiPattern.ReplaceAllString(got, ""), "#12 [builder 2/3] RUN npm ci") {
		t.Errorf("repeated header was silently dropped: %q", got)
	}
}

func TestDockerBuildStepCardsRaw(t *testing.T) {
	input := "#9 [builder 1/3] FROM node:22\n"
	for _, opts := range []Options{{Mode: Raw, Tool: Docker, Color: true}, {Mode: Enhance, Tool: Docker, Color: false}} {
		var output bytes.Buffer
		if err := New(opts).Process(context.Background(), strings.NewReader(input), &output); err != nil {
			t.Fatal(err)
		}
		if output.String() != input {
			t.Errorf("input changed: %q", output.String())
		}
	}
}

func TestDockerLegacyStepCards(t *testing.T) {
	got, ok := newDockerBuildSteps().format("Step 2/5 : RUN echo hello\n")
	if !ok || !strings.Contains(got, "STEP 1") || !strings.Contains(got, "RUN") {
		t.Fatalf("legacy header: %q", got)
	}
}

func TestDockerStepStatusMapping(t *testing.T) {
	steps := newDockerBuildSteps()
	steps.format("#23 [builder 1/2] RUN npm ci\n")
	got, ok := steps.format("#23 DONE 4.2s\n")
	if !ok || !strings.Contains(got, "STEP 1") || !strings.Contains(got, "DONE") || !strings.Contains(got, "4.2s") {
		t.Fatalf("status not mapped to step: %q", got)
	}
	if _, ok := steps.format("#99 DONE 1.0s\n"); ok {
		t.Fatal("unknown step status must preserve original output")
	}
}

func TestDockerBuildStatusError(t *testing.T) {
	steps := newDockerBuildSteps()
	steps.format("#5 [build 2/3] RUN false\n")
	got, ok := steps.format("#5 ERROR 0.5s\n")
	if !ok || !strings.Contains(got, "STEP 1") || !strings.Contains(got, "ERROR") || !strings.Contains(got, "0.5s") {
		t.Fatalf("error status lost: %q", got)
	}
}

func TestDockerCustomSeparator(t *testing.T) {
	input := "#1 [build 1/2] RUN echo hello\n#1 DONE 0.1s\n#2 [build 2/2] RUN echo world\n"
	var output bytes.Buffer
	engine := New(Options{Mode: Enhance, Tool: Docker, Color: true, DockerSeparator: "===="})
	if err := engine.Process(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(output.String(), "")
	if strings.Count(plain, "====") != 1 || strings.Contains(plain, "────") {
		t.Fatalf("custom divider must appear exactly once between top-level steps: %q", plain)
	}
}

func TestDockerBuildKitAutoDetection(t *testing.T) {
	for _, line := range []string{
		"#7 [build 1/2] RUN npm ci\n",
		"#7 DONE 2.1s\n",
		"Step 1/3 : FROM node:22\n",
	} {
		if got := Detect(line); got != Docker {
			t.Errorf("Detect(%q) = %q, want docker", line, got)
		}
	}
	var output bytes.Buffer
	input := "#7 [build 1/2] RUN npm ci\n#7 DONE 2.1s\n"
	if err := New(Options{Mode: Enhance, Tool: Auto, Color: true}).Process(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "STEP 1") || !strings.Contains(output.String(), "DONE") {
		t.Fatalf("automatic BuildKit cards missing: %q", output.String())
	}
}

func TestDockerAutoDetectionDoesNotCaptureOtherLogs(t *testing.T) {
	for _, line := range []string{
		"#7 [build 1/2] unrelated event\n",
		"#7 [build 1/2] completed arbitrary task\n",
		"ordinary application output\n",
	} {
		if got := Detect(line); got == Docker {
			t.Errorf("false Docker detection for %q", line)
		}
	}
}

func TestDockerBuildKitInterleavedStatuses(t *testing.T) {
	steps := newDockerBuildSteps()
	steps.format("#18 [build 1/3] FROM node:22\n")
	steps.format("#4 [build 2/3] RUN npm ci\n")
	first, okFirst := steps.format("#4 CACHED\n")
	second, okSecond := steps.format("#18 DONE 0.1s\n")
	if !okFirst || !okSecond || !strings.Contains(first, "STEP 2") || !strings.Contains(first, "CACHED") || !strings.Contains(second, "STEP 1") {
		t.Fatalf("interleaved status mismatch: first=%q second=%q", first, second)
	}
}

func TestDockerCustomColors(t *testing.T) {
	steps := newDockerBuildSteps()
	steps.colors = mergeDockerColors(steps.colors, DockerColors{
		BadgeBackground: "48;2;12;34;56",
		BadgeText:       "38;2;200;210;220",
		Done:            "38;2;1;2;3",
		Separator:       "38;2;4;5;6",
	})
	header, ok := steps.format("#8 [build 1/2] RUN echo hello\n")
	if !ok || !strings.Contains(ansiPattern.ReplaceAllString(header, ""), "STEP 1") {
		t.Fatalf("timeline header missing: %q", header)
	}
	status, ok := steps.format("#8 DONE 1.2s\n")
	if !ok || !strings.Contains(status, "\x1b[38;2;1;2;3m") {
		t.Fatalf("custom status color missing: %q", status)
	}
	second, ok := steps.format("#9 [build 2/2] RUN echo world\n")
	if !ok || !strings.Contains(second, "\x1b[38;2;4;5;6m") {
		t.Fatalf("custom separator color missing between top-level steps: %q", second)
	}
}

func TestColorSGR(t *testing.T) {
	cases := []struct {
		value      string
		background bool
		want       string
	}{
		{"#A78BFA", false, "38;2;167;139;250"},
		{"#0c2238", true, "48;2;12;34;56"},
		{"32", false, "32"},
	}
	for _, tc := range cases {
		got, err := ColorSGR(tc.value, tc.background)
		if err != nil || got != tc.want {
			t.Errorf("ColorSGR(%q) = %q, %v; want %q", tc.value, got, err, tc.want)
		}
	}
	for _, invalid := range []string{"#fff", "#xyzxyz", "red", "31m\\x1b[2J", ""} {
		if _, err := ColorSGR(invalid, false); err == nil {
			t.Errorf("expected invalid color %q to fail", invalid)
		}
	}
}

func TestParseDockerColors(t *testing.T) {
	got, err := ParseDockerColors(DockerColors{BadgeBackground: "#0C2238", Done: "#4ADE80"})
	if err != nil {
		t.Fatal(err)
	}
	if got.BadgeBackground != "48;2;12;34;56" || got.Done != "38;2;74;222;128" || got.Cached != "2" {
		t.Fatalf("incorrect palette merge: %+v", got)
	}
	if _, err := ParseDockerColors(DockerColors{Error: "not-a-color"}); err == nil {
		t.Fatal("invalid theme color was accepted")
	}
}

func TestEngineDockerHexColors(t *testing.T) {
	input := "#8 [build 1/2] RUN echo hello\n#8 DONE 1.2s\n#9 [build 2/2] RUN echo world\n"
	var output bytes.Buffer
	engine := New(Options{
		Mode:  Enhance,
		Tool:  Docker,
		Color: true,
		DockerColors: DockerColors{
			BadgeBackground: "#0C2238",
			BadgeText:       "#E8EAF0",
			Done:            "#4ADE80",
			Separator:       "#A78BFA",
		},
	})
	if err := engine.Process(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"\x1b[38;2;74;222;128m",
		"\x1b[38;2;167;139;250m",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("missing configured color %q in %q", expected, output.String())
		}
	}
}

func TestDockerNestedBuildKitOutput(t *testing.T) {
	input := "#1 [internal] load build definition from Dockerfile\n#1 transferring dockerfile: 615B done\n#2 [internal] load metadata for docker.io/library/node:22-alpine\n#3 [internal] load .dockerignore\n#3 transferring context: 2B done\n#4 [1/4] FROM docker.io/library/node:22-alpine\n#5 [2/4] WORKDIR /app\n#6 [3/4] COPY package.json package-lock.json ./\n#7 [4/4] RUN npm ci\n#7 0.3 npm ERR! code ERESOLVE\nSTEP 4  ERROR: process \"/bin/sh -c npm ci\" did not complete successfully: exit code: 1\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Enhance, Tool: Auto, Color: true}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	for _, expected := range []string{"STEP 1", "load build definition", "├ 1.1", "transferring dockerfile: 615B done", "load metadata", "load .dockerignore", "├ 3.1", "transferring context: 2B done", "STEP 7", "├ 7.1", "npm ERR! code ERESOLVE", "STEP 4 ERROR", "exit code: 1"} {
		if !strings.Contains(plain, expected) {
			t.Errorf("missing %q in output: %s", expected, plain)
		}
	}
}

func TestDockerInterleavedNestedOutput(t *testing.T) {
	s := newDockerBuildSteps()
	s.format("#8 [build 1/2] RUN echo hello\n")
	s.format("#9 [build 2/2] RUN echo world\n")
	got, ok := s.format("#8 0.2 hello\n")
	if !ok || !strings.Contains(got, "1.1") || !strings.Contains(got, "0.2s") {
		t.Fatalf("nested step association: %q", got)
	}
}


func TestDockerNestedPNPMAndTypeScriptTools(t *testing.T) {
	input := strings.Join([]string{
		"#14 [build 7/9] RUN pnpm --filter @rodsandbox/runtime-agent... install --frozen-lockfile --ignore-scripts",
		"#14 1.512 Lockfile is up to date, resolution step is skipped",
		"#14 2.616 Progress: resolved 946, reused 0, downloaded 70, added 60",
		"#14 6.190 WARN 10 deprecated subdependencies found",
		"#14 12.48 Done in 11.5s",
		"#14 DONE 14.7s",
		"#17 [build 9/9] RUN pnpm --filter @rodsandbox/runtime-agent build",
		"#17 0.503 > @rodsandbox/runtime-agent@0.1.0 build /source/packages/runtime-agent",
		"#17 0.503 > tsc -p tsconfig.json",
		"#17 0.800 packages/runtime-agent/src/index.ts(12,7): error TS2322: Type 'number' is not assignable to type 'string'.",
		"#17 ERROR 1.0s",
	}, "\n") + "\n"
	var out bytes.Buffer
	opts := Options{Mode: Transform, Tool: Auto, Color: true, Detail: DetailCompact}
	if err := New(opts).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	for _, want := range []string{
		"STEP 1", "pnpm", "Lockfile is up to date", "resolved", "946", "done", "11.5s",
		"STEP 2", "◆ tsc", "ERROR", "TS2322", "Expected:", "Received:",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in nested Docker output:\n%s", want, plain)
		}
	}
}

func TestDetectDockerRunToolUsesShellAST(t *testing.T) {
	for _, tc := range []struct{ command string; want Tool }{
		{"RUN pnpm --filter app install --frozen-lockfile", PNPM},
		{"RUN corepack pnpm install", PNPM},
		{"RUN pnpm exec tsc --noEmit", TypeScript},
		{"RUN echo ok && pnpm install", PNPM},
		{"COPY package.json ./", Generic},
	} {
		if got := detectDockerRunTool(tc.command); got != tc.want {
			t.Errorf("detectDockerRunTool(%q)=%s want %s", tc.command, got, tc.want)
		}
	}
}


func TestDockerNestedSymbolsRespectASCII(t *testing.T) {
	input := "#14 [build 1/1] RUN pnpm install\n#14 1.0 Progress: resolved 9, reused 8, downloaded 1, added 9\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true, ASCIISymbols: true}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	for _, forbidden := range []string{"◆", "╰", "│"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("unicode symbol %q leaked in ASCII mode: %q", forbidden, plain)
		}
	}
	for _, want := range []string{"> STEP 1", "pnpm", "+ 1.1", "resolved", "9"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing ASCII nested symbol %q in %q", want, plain)
		}
	}
}


func TestDockerSeparatorsOnlyBetweenTopLevelSteps(t *testing.T) {
	input := strings.Join([]string{
		"#14 [build 1/2] RUN pnpm install",
		"#14 0.1 Progress: resolved 1, reused 0, downloaded 1, added 1",
		"#14 0.2 Done in 0.2s",
		"#14 DONE 0.3s",
		"#17 [build 2/2] RUN pnpm build",
		"#17 0.1 > tsc -p tsconfig.json",
		"#17 DONE 0.4s",
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	separator := strings.Repeat("─", defaultStepSeparatorWidth)
	if got := strings.Count(plain, separator); got != 1 {
		t.Fatalf("separator count=%d want 1:\n%s", got, plain)
	}
	firstDone := strings.Index(plain, "STEP 1  DONE")
	sep := strings.Index(plain, separator)
	second := strings.Index(plain, "STEP 2")
	if firstDone < 0 || sep < firstDone || second < sep {
		t.Fatalf("separator must sit between top-level steps:\n%s", plain)
	}
}

func TestDockerDurationsAreAlwaysDimmed(t *testing.T) {
	input := "#14 [build 1/1] RUN pnpm install\n#14 0.146 Progress: resolved 1, reused 0, downloaded 1, added 1\n#14 DONE 14.7s\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	raw := out.String()
	for _, duration := range []string{"0.146s", "14.7s"} {
		if !strings.Contains(raw, "\x1b[2m"+duration+"\x1b[0m") {
			t.Fatalf("duration %q is not faint:\n%q", duration, raw)
		}
	}
}

func TestDockerNestedLayoutIsCompactAndDepthBounded(t *testing.T) {
	input := strings.Join([]string{
		"#17 [build 1/1] RUN pnpm build",
		"#17 0.503 > tsc -p tsconfig.json",
		"#17 0.800 src/index.ts(12,7): error TS2322: Type 'number' is not assignable to type 'string'.",
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true, Detail: DetailCompact}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	for _, want := range []string{"├ 1.1", "◆ tsc", "├ 1.2", "TS2322"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing compact nested fragment %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "STEP 1.1") || strings.Contains(plain, "│  │") {
		t.Fatalf("nested layout exceeded visual hierarchy:\n%s", plain)
	}
	if strings.Count(plain, "pnpm") > 3 {
		t.Fatalf("pnpm tool label duplicated inside nested body:\n%s", plain)
	}
}


func TestDockerTimelineDoesNotRepeatBaseTool(t *testing.T) {
	input := "#14 [build 1/1] RUN pnpm install\n#14 0.1 Progress: resolved 9, reused 8, downloaded 1, added 9\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	if strings.Count(plain, "pnpm") != 2 {
		t.Fatalf("expected pnpm only in header and command, got:\n%s", plain)
	}
	if strings.Contains(plain, "◆ pnpm  ◆ pnpm") {
		t.Fatalf("duplicated pnpm marker:\n%s", plain)
	}
}

func TestDockerTimelineUsesCanonicalTypeScriptRenderer(t *testing.T) {
	input := "#17 [build 1/1] RUN pnpm build\n#17 0.5 > tsc -p tsconfig.json\n#17 0.8 src/index.ts(12,7): error TS2322: Type 'number' is not assignable to type 'string'.\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true, Detail: DetailCompact}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	raw := out.String()
	plain := ansiPattern.ReplaceAllString(raw, "")
	for _, want := range []string{"◆ tsc", "ERROR", "[TS2322]", "src/index.ts:12:7", "Expected:", "Received:", "Compare the expected and actual types"} {
		if !strings.Contains(plain, want) { t.Fatalf("missing canonical TypeScript fragment %q:\n%s", want, plain) }
	}
	if !strings.Contains(raw, "\x1b[7;") && !strings.Contains(raw, "\x1b[1;7;") {
		t.Fatalf("canonical diagnostic severity badge was not rendered: %q", raw)
	}
}


func TestDockerNestedStructuredTypeScriptDiagnosticUsesCanonicalRenderer(t *testing.T) {
	input := strings.Join([]string{
		"#17 [build 1/1] RUN pnpm build",
		"#17 0.5 > tsc -p tsconfig.json",
		`#17 0.8 {"tool":"typescript","level":"error","code":"TS2322","file":"src/a.ts","line":4,"column":7,"message":"Type 'number' is not assignable to type 'string'.","related":[{"tool":"typescript","level":"info","file":"src/types.ts","line":1,"column":1,"message":"Expected type declared here","signature":"type UserId = string","signature_focus":"string","signature_focus_label":"expected here"}]}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := New(Options{Mode: Transform, Tool: Auto, Color: true, Detail: DetailCompact}).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	plain := ansiPattern.ReplaceAllString(out.String(), "")
	for _, want := range []string{"[TS2322]", "Expected type", "type UserId = string", "expected here"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("structured TypeScript data lost inside Docker, missing %q:\n%s", want, plain)
		}
	}
}


func TestDockerNestedPNPMPreservesSemanticColors(t *testing.T) {
	input := "#14 [build 1/1] RUN pnpm install\n#14 0.1 Progress: resolved 946, reused 12, downloaded 34, added 56\n"
	var out bytes.Buffer
	opts := Options{
		Mode: Transform,
		Tool: Auto,
		Color: true,
		SemanticColors: SemanticColors{
			Info: "38;2;10;20;30",
			Success: "38;2;40;50;60",
			Muted: "38;2;70;80;90",
		},
	}
	if err := New(opts).Process(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	raw := out.String()
	for _, want := range []string{
		"\x1b[38;2;70;80;90mresolved\x1b[0m",
		"\x1b[38;2;10;20;30m946\x1b[0m",
		"\x1b[38;2;70;80;90m· reused\x1b[0m",
		"\x1b[38;2;40;50;60m12\x1b[0m",
		"\x1b[38;2;70;80;90m· downloaded\x1b[0m",
		"\x1b[38;2;10;20;30m34\x1b[0m",
		"\x1b[38;2;70;80;90m· added\x1b[0m",
		"\x1b[38;2;40;50;60m56\x1b[0m",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("nested pnpm color %q was stripped:\n%q", want, raw)
		}
	}
}

func TestDockerNestedANSIWrapPreservesHyperlinksAndColors(t *testing.T) {
	rendered := "\x1b[36mhello\x1b[0m \x1b]8;;https://example.com\x1b\\example-link\x1b]8;;\x1b\\"
	got := renderDockerNestedStep(1, 1, "0.1", PNPM, PNPM, rendered, Options{Color:true})
	if !strings.Contains(got, "\x1b[36mhello\x1b[0m") {
		t.Fatalf("ANSI color was stripped: %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;https://example.com\x1b\\") {
		t.Fatalf("OSC8 hyperlink was stripped: %q", got)
	}
}
