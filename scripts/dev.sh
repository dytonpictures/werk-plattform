#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/dev-env.sh"

if [[ "${WERK_DEV_SKIP_INFRA:-0}" != "1" ]]; then
  bash "${script_dir}/dev-infra.sh" up
fi

bash "${script_dir}/dev-service.sh" migrate

pids=()

cleanup() {
  local pid
  trap - EXIT INT TERM
  for pid in "${pids[@]:-}"; do
    if kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
    fi
  done
  for pid in "${pids[@]:-}"; do
    wait "${pid}" 2>/dev/null || true
  done
}

trap cleanup EXIT
trap 'exit 130' INT TERM

bash "${script_dir}/dev-service.sh" api &
pids+=("$!")
bash "${script_dir}/dev-service.sh" worker &
pids+=("$!")
bash "${script_dir}/dev-service.sh" dashboard &
pids+=("$!")

echo
echo "WERK laeuft nativ im Entwicklungsmodus:"
echo "  Dashboard: ${WERK_DEV_DASHBOARD_ADDRESS}"
echo "  API:       ${WERK_DEV_API_URL}"
echo "  Beenden:   Ctrl+C (PostgreSQL und Valkey laufen weiter)"
echo

set +e
wait -n "${pids[@]}"
status=$?
set -e

if [[ "${status}" -ne 0 ]]; then
  echo "Ein WERK-Entwicklungsprozess wurde mit Status ${status} beendet." >&2
fi
exit "${status}"
