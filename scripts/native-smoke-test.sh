#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${WERK_NATIVE_SMOKE_CONFIRM:-}" != "throwaway-postgresql-cluster" ]]; then
  echo "native smoke test changes global WERK roles and requires a disposable PostgreSQL cluster" >&2
  echo "set WERK_NATIVE_SMOKE_CONFIRM=throwaway-postgresql-cluster to continue" >&2
  exit 2
fi

for command in go psql createdb dropdb curl; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "required command not found: ${command}" >&2
    exit 1
  fi
done

script_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_root="$(cd -- "${script_directory}/.." && pwd -P)"
temporary_directory="$(mktemp -d)"
database_name="werk_native_smoke_${$}"
api_port="${WERK_NATIVE_SMOKE_API_PORT:-18081}"
api_pid=""
worker_pid=""

postgres() {
  if [[ "$(id -u)" -eq 0 ]]; then
    runuser -u postgres -- "$@"
  else
    sudo -u postgres -- "$@"
  fi
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  for pid in "${api_pid}" "${worker_pid}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
    fi
  done
  for pid in "${api_pid}" "${worker_pid}"; do
    if [[ -n "${pid}" ]]; then
      wait "${pid}" 2>/dev/null || true
    fi
  done
  if [[ "${status}" -ne 0 ]]; then
    echo "--- native api log ---" >&2
    sed -n '1,240p' "${temporary_directory}/api.log" >&2 2>/dev/null || true
    echo "--- native worker log ---" >&2
    sed -n '1,240p' "${temporary_directory}/worker.log" >&2 2>/dev/null || true
  fi
  postgres dropdb --if-exists "${database_name}" >/dev/null 2>&1 || true
  postgres psql --no-psqlrc --set=ON_ERROR_STOP=1 --dbname=postgres >/dev/null 2>&1 <<'SQL' || true
DROP ROLE IF EXISTS werk_migrator, werk_work_runtime, werk_identity_runtime,
  werk_admin_runtime, werk_service_runtime, werk_worker_runtime, werk_backup;
DROP ROLE IF EXISTS werk_owner, werk_backup_reader;
SQL
  rm -rf -- "${temporary_directory}"
  exit "${status}"
}
trap cleanup EXIT INT TERM

binary_directory="${WERK_NATIVE_BIN_DIR:-${temporary_directory}/bin}"
if [[ -z "${WERK_NATIVE_BIN_DIR:-}" ]]; then
  mkdir -p -- "${binary_directory}"
  for service in api worker migrate; do
    CGO_ENABLED=0 go build -trimpath -o "${binary_directory}/werk-${service}" "${project_root}/cmd/${service}"
  done
  CGO_ENABLED=0 go build -trimpath -o "${binary_directory}/werkctl" "${project_root}/cmd/werkctl"
fi
for binary in werk-api werk-worker werk-migrate werkctl; do
  if [[ ! -x "${binary_directory}/${binary}" ]]; then
    echo "native binary is missing or not executable: ${binary_directory}/${binary}" >&2
    exit 1
  fi
done

postgres createdb "${database_name}"
postgres env \
  PGHOST=/var/run/postgresql PGDATABASE="${database_name}" PGUSER=postgres PGPASSWORD=unused \
  WERK_ENV=development WERK_DB_NAME="${database_name}" \
  WERK_MIGRATOR_PASSWORD=werk-migrator-dev WERK_WORK_PASSWORD=werk-work-dev \
  WERK_IDENTITY_PASSWORD=werk-identity-dev WERK_ADMIN_PASSWORD=werk-admin-dev \
  WERK_SERVICE_PASSWORD=werk-service-dev WERK_WORKER_PASSWORD=werk-worker-dev \
  WERK_BACKUP_PASSWORD=werk-backup-dev \
  sh "${project_root}/deploy/postgres/bootstrap-roles.sh"

cat >"${temporary_directory}/.env" <<EOF
WERK_ENV=development
WERK_BUILD_VERSION=native-smoke
WERK_HTTP_ADDRESS=127.0.0.1:${api_port}
WERK_HTTP_TLS_MODE=disabled
WERK_ALLOWED_ORIGINS=http://127.0.0.1:${api_port}
WERK_BOOTSTRAP_ADMIN_PASSWORD=werk-development
WERK_IDENTITY_MFA_ENABLED=false
WERK_KAFKA_ENABLED=false
WORK_DATABASE_URL=postgresql://werk_work_runtime:werk-work-dev@127.0.0.1:5432/${database_name}?sslmode=disable
IDENTITY_DATABASE_URL=postgresql://werk_identity_runtime:werk-identity-dev@127.0.0.1:5432/${database_name}?sslmode=disable
ADMIN_DATABASE_URL=postgresql://werk_admin_runtime:werk-admin-dev@127.0.0.1:5432/${database_name}?sslmode=disable
WORKER_DATABASE_URL=postgresql://werk_worker_runtime:werk-worker-dev@127.0.0.1:5432/${database_name}?sslmode=disable
MIGRATOR_DATABASE_URL=postgresql://werk_migrator:werk-migrator-dev@127.0.0.1:5432/${database_name}?sslmode=disable
EOF
chmod 0600 "${temporary_directory}/.env"

"${binary_directory}/werkctl" doctor --env "${temporary_directory}/.env" --config-only
"${binary_directory}/werkctl" migrate --env "${temporary_directory}/.env"
"${binary_directory}/werkctl" doctor --env "${temporary_directory}/.env"

(
  exec env WERK_ENV_FILE="${temporary_directory}/.env" "${binary_directory}/werk-api"
) >"${temporary_directory}/api.log" 2>&1 &
api_pid=$!
(
  exec env WERK_ENV_FILE="${temporary_directory}/.env" "${binary_directory}/werk-worker"
) >"${temporary_directory}/worker.log" 2>&1 &
worker_pid=$!

ready=0
for _ in $(seq 1 40); do
  if ! kill -0 "${api_pid}" 2>/dev/null || ! kill -0 "${worker_pid}" 2>/dev/null; then
    echo "a native process exited during startup" >&2
    exit 1
  fi
  if curl --fail --silent --show-error "http://127.0.0.1:${api_port}/health/ready" >/dev/null; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "${ready}" -ne 1 ]]; then
  echo "native API did not become ready" >&2
  exit 1
fi
curl --fail --silent --show-error "http://127.0.0.1:${api_port}/" | grep -q '<!doctype html>'
curl --fail --silent --show-error "http://127.0.0.1:${api_port}/meta" | grep -q 'native-smoke'
if curl --fail --silent "http://127.0.0.1:${api_port}/metrics" >/dev/null; then
  echo "public native listener exposed internal metrics" >&2
  exit 1
fi

echo "native smoke test passed"
