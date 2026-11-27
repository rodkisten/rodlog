#!/usr/bin/env bash
set -euo pipefail
repo=rodkisten/rodlog
case "$(uname -s)" in Linux) os=linux;; Darwin) os=darwin;; *) echo "Unsupported OS" >&2; exit 1;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo "Unsupported architecture" >&2; exit 1;; esac
tag="$(gh release list -R "$repo" --limit 100 --json tagName,isDraft,isPrerelease --jq '[.[] | select(.isPrerelease and (.isDraft | not) and (.tagName | startswith("preview-")))][0].tagName // empty')"
test -n "$tag" || { echo "No validated preview available" >&2; exit 1; }
dest="$HOME/.local/bin"
if [[ -x "$dest/rodlog" && -f "$dest/.rodlog-preview" && "$(cat "$dest/.rodlog-preview")" == "$tag" ]]; then
  printf "Already up to date: %s\n" "$tag"
  exit 0
fi
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
asset="rodlog-$os-$arch"
gh release download "$tag" -R "$repo" -p "$asset" -p SHA256SUMS -D "$tmp"
(cd "$tmp"; grep "  $asset$" SHA256SUMS > checksum; if command -v sha256sum >/dev/null; then sha256sum -c checksum; else shasum -a 256 -c checksum; fi)
dest="$HOME/.local/bin"
mkdir -p "$dest"
/usr/bin/install -m 755 "$tmp/$asset" "$dest/.rodlog-new"
mv -f "$dest/.rodlog-new" "$dest/rodlog"
printf "%s\n" "$tag" > "$dest/.rodlog-preview"
printf 'Installed %s at %s/rodlog\n' "$tag" "$dest"
