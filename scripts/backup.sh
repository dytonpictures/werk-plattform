#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd -- "${script_dir}/.." && pwd)"
: "${WERK_BACKUP_RECIPIENT:?WERK_BACKUP_RECIPIENT is required}"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="${WERK_BACKUP_OUTPUT:-${project_root}/backups/werk-${timestamp}-$$.dump.age}"
if [[ "${output}" != *.age ]]; then
  echo "WERK_BACKUP_OUTPUT must end with .age" >&2
  exit 2
fi

output_directory="$(dirname -- "${output}")"
mkdir -p -- "${output_directory}"
output_directory="$(cd -- "${output_directory}" && pwd -P)"
output="${output_directory}/$(basename -- "${output}")"
checksum_file="${output}.sha256"
partial="${output}.partial.$$"
checksum_partial="${checksum_file}.partial.$$"

if [[ -e "${output}" || -e "${checksum_file}" ]]; then
  echo "backup output already exists" >&2
  exit 2
fi

cleanup() {
  rm -f -- "${partial}" "${checksum_partial}"
}
trap cleanup EXIT INT TERM

compose=(
  docker compose
  --file "${project_root}/compose.yaml"
  --file "${project_root}/compose.ops.yaml"
)

cd "${project_root}"
"${compose[@]}" build backup
"${compose[@]}" run --rm --no-deps -T \
  --env "WERK_BACKUP_RECIPIENT=${WERK_BACKUP_RECIPIENT}" \
  backup backup >"${partial}"

if [[ ! -s "${partial}" ]] || [[ "$(head -n 1 "${partial}")" != "age-encryption.org/v1" ]]; then
  echo "backup did not produce a valid age artifact" >&2
  exit 1
fi

checksum="$(sha256sum "${partial}" | awk '{print $1}')"
printf '%s  %s\n' "${checksum}" "$(basename -- "${output}")" >"${checksum_partial}"
mv -- "${partial}" "${output}"
mv -- "${checksum_partial}" "${checksum_file}"
trap - EXIT INT TERM

echo "encrypted backup: ${output}"
echo "ciphertext checksum: ${checksum_file}"
