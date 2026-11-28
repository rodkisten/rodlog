#!/usr/bin/env bash
set -euo pipefail

rodlog_bin="${1:-./rodlog}"
project="${2:-testdata/typescript-intelligence/tsconfig.json}"
output="${3:-/tmp/rodlog-docker-typescript-intelligence.log}"

transport="$(mktemp)"
trap 'rm -f "$transport"' EXIT

set +e
"$rodlog_bin" tsc   --transport=jsonl   --project "$project"   --context-before=1   --context-after=1   > "$transport" 2>&1
status=$?
set -e

if [[ "$status" -ne 1 ]]; then
  printf 'expected TypeScript fixture to exit 1, got %s\n' "$status" >&2
  cat "$transport" >&2
  exit 1
fi

if ! head -n 1 "$transport" | grep -q '^{"tool":"typescript"'; then
  printf 'transport is not raw TypeScript Diagnostic JSON\n' >&2
  head -n 3 "$transport" >&2
  exit 1
fi

{
  printf '%s\n' '#88 [build 1/1] RUN rodlog tsc --transport=jsonl -p tsconfig.json'
  awk 'NF { printf "#88 0.%03d %s\n", 200 + NR, $0 }' "$transport"
  printf '%s\n' '#88 ERROR 9.999s'
} > "$output"

printf '%s\n' "$output"
