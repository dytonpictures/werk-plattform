#!/usr/bin/env sh
set -eu

script_directory="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
project_root="$(cd -- "${script_directory}/.." && pwd -P)"
cd -- "${project_root}"

if [ ! -f .env ]; then
  echo "Missing .env. Copy .env.example to .env, set mode 0600 and replace every CHANGE_ME value first." >&2
  exit 2
fi

go run ./cmd/werkctl doctor --env .env --config-only
go run ./cmd/werkctl migrate --env .env
echo "WERK starts on the address configured by WERK_HTTP_ADDRESS. Stop with Ctrl+C."
exec go run ./cmd/api
