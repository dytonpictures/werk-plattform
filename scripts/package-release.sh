#!/usr/bin/env sh
set -eu

version="${1:?usage: package-release.sh VERSION [OUTPUT_DIRECTORY]}"
output_directory="${2:-dist}"

if ! printf '%s\n' "${version}" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'; then
  echo "version must be SemVer without a leading v or build metadata" >&2
  exit 2
fi

script_directory="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
project_root="$(cd -- "${script_directory}/.." && pwd -P)"
mkdir -p -- "${output_directory}"
output_directory="$(cd -- "${output_directory}" && pwd -P)"

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf -- "${temporary_directory}"
}
trap cleanup EXIT HUP INT TERM

for architecture in amd64 arm64; do
  archive_name="werk-platform-v${version}-linux-${architecture}"
  package_directory="${temporary_directory}/${archive_name}"
  mkdir -p -- "${package_directory}"

  for service in api worker migrate; do
    CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" \
      go build -trimpath -ldflags="-s -w" \
      -o "${package_directory}/werk-${service}" "${project_root}/cmd/${service}"
  done

  printf '%s\n' "${version}" >"${package_directory}/VERSION"
  cp -- "${project_root}/README.md" "${package_directory}/README.md"
  tar -C "${temporary_directory}" -czf "${output_directory}/${archive_name}.tar.gz" "${archive_name}"
done

(
  cd -- "${output_directory}"
  sha256sum "werk-platform-v${version}-linux-amd64.tar.gz" \
    "werk-platform-v${version}-linux-arm64.tar.gz" >SHA256SUMS
)

echo "release artifacts written to ${output_directory}"
