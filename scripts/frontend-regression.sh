#!/usr/bin/env bash
set -Eeuo pipefail
exec node "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/frontend-regression.mjs" "$@"
