#!/usr/bin/env bash
set -Eeuo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
base_url="${WERK_REGRESSION_URL:-http://127.0.0.1:3000}"

exec go run "${project_root}/cmd/werkctl" regression \
  --url "${base_url}" \
  "${@}"
