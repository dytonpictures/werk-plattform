#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd -- "${script_dir}/.." && pwd)"
: "${WERK_RESTORE_ARCHIVE:?WERK_RESTORE_ARCHIVE is required}"
: "${WERK_RESTORE_IDENTITY:?WERK_RESTORE_IDENTITY is required}"
: "${WERK_RESTORE_CONFIRM:?WERK_RESTORE_CONFIRM is required}"

if [[ ! -f "${WERK_RESTORE_ARCHIVE}" || ! -r "${WERK_RESTORE_ARCHIVE}" ]]; then
  echo "restore archive is not a readable file" >&2
  exit 2
fi
if [[ ! -f "${WERK_RESTORE_IDENTITY}" || ! -r "${WERK_RESTORE_IDENTITY}" ]]; then
  echo "restore identity is not a readable file" >&2
  exit 2
fi

archive_directory="$(cd -- "$(dirname -- "${WERK_RESTORE_ARCHIVE}")" && pwd -P)"
archive="${archive_directory}/$(basename -- "${WERK_RESTORE_ARCHIVE}")"
identity_directory="$(cd -- "$(dirname -- "${WERK_RESTORE_IDENTITY}")" && pwd -P)"
identity="${identity_directory}/$(basename -- "${WERK_RESTORE_IDENTITY}")"
checksum_file="${archive}.sha256"
if [[ ! -f "${checksum_file}" ]]; then
  echo "ciphertext checksum file is missing: ${checksum_file}" >&2
  exit 2
fi

(
  cd -- "${archive_directory}"
  sha256sum --check --status "$(basename -- "${checksum_file}")"
) || {
  echo "ciphertext checksum verification failed" >&2
  exit 1
}

compose=(
  docker compose
  --file "${project_root}/compose.yaml"
  --file "${project_root}/compose.ops.yaml"
)

cd "${project_root}"
running_services="$("${compose[@]}" ps --services --status running 2>/dev/null || true)"
if grep --line-regexp --quiet -E 'api|worker' <<<"${running_services}"; then
  echo "restore requires API and worker to be stopped" >&2
  exit 2
fi

"${compose[@]}" build backup
"${compose[@]}" run --rm --no-deps -T \
  --user "$(id -u):$(id -g)" \
  --volume "${identity}:/run/secrets/werk-backup-identity:ro" \
  --env "WERK_RESTORE_CONFIRM=${WERK_RESTORE_CONFIRM}" \
  restore restore <"${archive}"

echo "restore completed for the confirmed empty target database"
