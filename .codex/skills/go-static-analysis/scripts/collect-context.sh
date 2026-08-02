#!/bin/sh
set -eu

root=${1:-.}
if [ ! -d "$root" ]; then
  echo "error: not a directory: $root" >&2
  exit 2
fi

cd "$root"
printf 'root: %s\n' "$(pwd)"
printf 'go: '
if command -v go >/dev/null 2>&1; then
  go_version=$(go version 2>&1 || true)
  if [ -n "$go_version" ]; then printf '%s\n' "$go_version"; else echo 'present but unavailable'; fi
else
  echo 'not found'
fi
printf '\nmodule files:\n'
find . -name .git -prune -o -name vendor -prune -o \( -name go.mod -o -name go.work -o -name go.sum \) -type f -print | sort
printf '\nlocal instructions:\n'
find . -name .git -prune -o -name vendor -prune -o \( -name AGENTS.md -o -name CONTRIBUTING.md \) -type f -print | sort
printf '\nanalysis configuration:\n'
find . -name .git -prune -o -name vendor -prune -o \( -name '.golangci.yml' -o -name '.golangci.yaml' -o -name 'staticcheck.conf' -o -name 'Makefile' \) -type f -print | sort
printf '\navailable tools:\n'
for tool in go staticcheck govulncheck golangci-lint; do
  if command -v "$tool" >/dev/null 2>&1; then printf '%s: %s\n' "$tool" "$(command -v "$tool")"; else printf '%s: missing\n' "$tool"; fi
done
