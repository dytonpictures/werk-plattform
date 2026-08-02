#!/bin/sh
set -eu

root=${1:-.}
if [ ! -d "$root" ]; then
  echo "error: not a directory: $root" >&2
  exit 2
fi

cd "$root"
printf 'root: %s\n' "$(pwd)"
for tool in node npm pnpm yarn bun; do
  if command -v "$tool" >/dev/null 2>&1; then
    version=$($tool --version 2>/dev/null || printf 'unknown')
    printf '%s: %s (%s)\n' "$tool" "$(command -v "$tool")" "$version"
  else
    printf '%s: missing\n' "$tool"
  fi
done
printf '\nmanifests and lockfiles:\n'
find . -name .git -prune -o -name node_modules -prune -o \( -name package.json -o -name package-lock.json -o -name npm-shrinkwrap.json -o -name pnpm-lock.yaml -o -name pnpm-workspace.yaml -o -name yarn.lock -o -name bun.lock -o -name bun.lockb \) -type f -print | sort
printf '\nlocal instructions:\n'
find . -name .git -prune -o -name node_modules -prune -o \( -name AGENTS.md -o -name CONTRIBUTING.md \) -type f -print | sort
printf '\nanalysis configuration:\n'
find . -name .git -prune -o -name node_modules -prune -o \( -name 'tsconfig*.json' -o -name 'eslint.config.*' -o -name '.eslintrc*' -o -name 'biome.json' -o -name 'biome.jsonc' -o -name 'knip.json' -o -name 'knip.jsonc' \) -type f -print | sort
