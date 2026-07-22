#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/dev-env.sh"

NODE="${NODE:-node}"
service="${1:-}"
minimum_go_major=1
minimum_go_minor=26
minimum_go_patch=5
minimum_go_version="${minimum_go_major}.${minimum_go_minor}.${minimum_go_patch}"

mkdir -p "${WERK_DEV_BUILD_DIR}"
cd "${WERK_DEV_ROOT}"

go_is_compatible() {
  local candidate="$1"
  local major
  local minor
  local patch
  local version
  version="$("${candidate}" version 2>/dev/null)" || return 1
  [[ "${version}" =~ go([0-9]+)\.([0-9]+)(\.([0-9]+))? ]] || return 1
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[4]:-0}"
  ((major > minimum_go_major ||
    (major == minimum_go_major && minor > minimum_go_minor) ||
    (major == minimum_go_major && minor == minimum_go_minor && patch >= minimum_go_patch)))
}

resolve_go() {
  local requested="${GO:-go}"
  local candidate
  local index
  local -a module_roots=()
  local -a cached_candidates=()

  if go_is_compatible "${requested}"; then
    GO_RESOLVED="${requested}"
    return
  fi

  [[ -n "${GOMODCACHE:-}" ]] && module_roots+=("${GOMODCACHE}")
  [[ -n "${GOPATH:-}" ]] && module_roots+=("${GOPATH}/pkg/mod")
  [[ -n "${HOME:-}" ]] && module_roots+=("${HOME}/go/pkg/mod")

  shopt -s nullglob
  for candidate in "${module_roots[@]}"; do
    cached_candidates+=("${candidate}"/golang.org/toolchain@v*-*/bin/go)
  done
  shopt -u nullglob

  for ((index = ${#cached_candidates[@]} - 1; index >= 0; index--)); do
    candidate="${cached_candidates[index]}"
    if go_is_compatible "${candidate}"; then
      GO_RESOLVED="${candidate}"
      echo "Hinweis: '${requested}' ist nicht als Go ${minimum_go_version} oder neuer nutzbar; verwende ${candidate}." >&2
      return
    fi
  done

  for candidate in /usr/local/go/bin/go /opt/go/bin/go; do
    if [[ -x "${candidate}" ]] && "${candidate}" version >/dev/null 2>&1; then
      GO_RESOLVED="${candidate}"
      export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
      echo "Hinweis: '${requested}' ist nicht als Go ${minimum_go_version} oder neuer nutzbar; verwende ${candidate} mit Go-Toolchain-Auswahl." >&2
      return
    fi
  done

  echo "Go ${minimum_go_version} oder neuer wurde nicht gefunden. Installiere eine passende Go-Version oder setze GO=/pfad/zu/go." >&2
  exit 1
}

case "${service}" in
  migrate)
    resolve_go
    "${GO_RESOLVED}" build -buildvcs=false -o "${WERK_DEV_BUILD_DIR}/werk-migrate" ./cmd/migrate
    exec env \
      DATABASE_URL="${WERK_DEV_MIGRATOR_DATABASE_URL}" \
      "${WERK_DEV_BUILD_DIR}/werk-migrate"
    ;;
  api)
    resolve_go
    "${GO_RESOLVED}" build -buildvcs=false -o "${WERK_DEV_BUILD_DIR}/werk-api" ./cmd/api
    exec env \
      DATABASE_URL="${WERK_DEV_WORK_DATABASE_URL}" \
      WERK_KAFKA_ENABLED=true \
      WERK_KAFKA_BROKERS="${WERK_DEV_KAFKA_BROKERS}" \
      WERK_KAFKA_CLIENT_ID=platform-api-dev \
      WERK_HTTP_ADDRESS="${WERK_DEV_API_ADDRESS}" \
      "${WERK_DEV_BUILD_DIR}/werk-api"
    ;;
  worker)
    resolve_go
    "${GO_RESOLVED}" build -buildvcs=false -o "${WERK_DEV_BUILD_DIR}/werk-worker" ./cmd/worker
    exec env \
      DATABASE_URL="${WERK_DEV_WORKER_DATABASE_URL}" \
      WERK_KAFKA_ENABLED=true \
      WERK_KAFKA_BROKERS="${WERK_DEV_KAFKA_BROKERS}" \
      WERK_KAFKA_CLIENT_ID=platform-worker-dev \
      "${WERK_DEV_BUILD_DIR}/werk-worker"
    ;;
  dashboard)
    exec env \
      WERK_DASHBOARD_ADDRESS="${WERK_DEV_DASHBOARD_ADDRESS}" \
      WERK_API_URL="${WERK_DEV_API_URL}" \
      "${NODE}" "${WERK_DEV_ROOT}/dashboard/server.mjs"
    ;;
  *)
    echo "Verwendung: $0 {migrate|api|worker|dashboard}" >&2
    exit 2
    ;;
esac
