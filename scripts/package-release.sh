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

if ! command -v dpkg-deb >/dev/null 2>&1; then
  echo "dpkg-deb is required to build native Ubuntu packages" >&2
  exit 1
fi

for architecture in amd64; do
  archive_name="werk-platform-v${version}-linux-${architecture}"
  package_directory="${temporary_directory}/${archive_name}"
  mkdir -p -- "${package_directory}/bin" "${package_directory}/config" \
    "${package_directory}/systemd" "${package_directory}/postgres"

  for service in api worker migrate; do
    CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" \
      go build -trimpath -ldflags="-s -w" -o "${package_directory}/bin/werk-${service}" \
      "${project_root}/cmd/${service}"
  done
  CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" \
    go build -trimpath -ldflags="-s -w -X main.version=${version}" \
    -o "${package_directory}/bin/werkctl" "${project_root}/cmd/werkctl"

  printf '%s\n' "${version}" >"${package_directory}/VERSION"
  cp -- "${project_root}/README.md" "${package_directory}/README.md"
  cp -- "${project_root}/packaging/linux/README.md" "${package_directory}/NATIVE-INSTALL.md"
  cp -- "${project_root}/packaging/linux/install.sh" "${package_directory}/install.sh"
  chmod 0755 "${package_directory}/install.sh"
  cp -- "${project_root}"/packaging/linux/systemd/* "${package_directory}/systemd/"
  cp -- "${project_root}"/deploy/postgres/bootstrap-roles.* "${package_directory}/postgres/"
  sed "s/@VERSION@/${version}/g" \
    "${project_root}/packaging/linux/config/werk.env" \
    >"${package_directory}/config/.env"
  chmod 0640 "${package_directory}/config/.env"
  tar -C "${temporary_directory}" -czf "${output_directory}/${archive_name}.tar.gz" "${archive_name}"

  debian_root="${temporary_directory}/deb-${architecture}"
  mkdir -p -- "${debian_root}/DEBIAN" "${debian_root}/usr/bin" \
    "${debian_root}/usr/lib/systemd/system" "${debian_root}/usr/share/doc/werk-platform" \
    "${debian_root}/usr/share/werk/postgres" "${debian_root}/etc/werk"
  install -m 0755 "${package_directory}/bin/werk-api" "${debian_root}/usr/bin/werk-api"
  install -m 0755 "${package_directory}/bin/werk-worker" "${debian_root}/usr/bin/werk-worker"
  install -m 0755 "${package_directory}/bin/werk-migrate" "${debian_root}/usr/bin/werk-migrate"
  install -m 0755 "${package_directory}/bin/werkctl" "${debian_root}/usr/bin/werkctl"
  install -m 0644 "${package_directory}/README.md" "${debian_root}/usr/share/doc/werk-platform/README.md"
  install -m 0644 "${package_directory}/NATIVE-INSTALL.md" "${debian_root}/usr/share/doc/werk-platform/NATIVE-INSTALL.md"
  install -m 0644 "${package_directory}"/systemd/* "${debian_root}/usr/lib/systemd/system/"
  install -m 0644 "${package_directory}"/postgres/* "${debian_root}/usr/share/werk/postgres/"
  install -m 0640 "${package_directory}/config/.env" "${debian_root}/etc/werk/.env"
  sed -e "s/@VERSION@/${version}/g" -e "s/@ARCHITECTURE@/${architecture}/g" \
    "${project_root}/packaging/linux/debian/control.in" >"${debian_root}/DEBIAN/control"
  for maintainer_script in postinst prerm postrm; do
    install -m 0755 "${project_root}/packaging/linux/debian/${maintainer_script}" \
      "${debian_root}/DEBIAN/${maintainer_script}"
  done
  dpkg-deb --build --root-owner-group "${debian_root}" \
    "${output_directory}/werk-platform_${version}_${architecture}.deb"
done

(
  cd -- "${output_directory}"
  find . -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.deb' \) -printf '%f\n' \
    | sort | xargs sha256sum >SHA256SUMS
)

echo "release artifacts written to ${output_directory}"
