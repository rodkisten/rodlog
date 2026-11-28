package rodlog

import "testing"

func TestDetectCommand(t *testing.T) {
	cases := []struct {
		argv []string
		want Tool
	}{
		{[]string{"/usr/bin/gcloud", "run", "services", "list"}, GCloud},
		{[]string{"npx", "--yes", "tsc", "--noEmit"}, TypeScript},
		{[]string{"npx", "wrangler", "deploy"}, Wrangler},
		{[]string{"docker", "build", "."}, Docker},
		{[]string{"pnpm", "--filter", "@rodsandbox/runtime-agent...", "install", "--frozen-lockfile"}, PNPM},
		{[]string{"pnpm", "exec", "tsc", "--noEmit"}, TypeScript},
		{[]string{"pnpm", "--filter", "web", "exec", "tsc", "--noEmit"}, TypeScript},
		{[]string{"corepack", "pnpm", "install"}, PNPM},
		{[]string{"echo", "gcloud error"}, Generic},
		{nil, Generic},
	}
	for _, tc := range cases {
		if got := DetectCommand(tc.argv); got != tc.want {
			t.Errorf("DetectCommand(%q) = %s, want %s", tc.argv, got, tc.want)
		}
	}
}
