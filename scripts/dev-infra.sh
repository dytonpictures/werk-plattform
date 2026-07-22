#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/dev-env.sh"

compose=(
  docker compose
  --project-name "${WERK_DEV_PROJECT}"
  --file "${WERK_DEV_ROOT}/compose.yaml"
  --file "${WERK_DEV_ROOT}/compose.dev.yaml"
)

bootstrap_roles() {
  "${compose[@]}" exec --no-TTY \
    --env PGHOST=127.0.0.1 \
    --env PGPORT=5432 \
    --env PGDATABASE=werk \
    --env PGUSER=werk \
    --env "PGPASSWORD=${POSTGRES_PASSWORD}" \
    --env WERK_ENV=development \
    --env WERK_DB_NAME=werk \
    --env "WERK_MIGRATOR_PASSWORD=${POSTGRES_MIGRATOR_PASSWORD}" \
    --env "WERK_WORK_PASSWORD=${POSTGRES_WORK_PASSWORD}" \
    --env "WERK_IDENTITY_PASSWORD=${POSTGRES_IDENTITY_PASSWORD}" \
    --env "WERK_ADMIN_PASSWORD=${POSTGRES_ADMIN_PASSWORD}" \
    --env "WERK_SERVICE_PASSWORD=${POSTGRES_SERVICE_PASSWORD}" \
    --env "WERK_WORKER_PASSWORD=${POSTGRES_WORKER_PASSWORD}" \
    --env "WERK_BACKUP_PASSWORD=${POSTGRES_BACKUP_PASSWORD}" \
    postgres sh /bootstrap/bootstrap-roles.sh
}

case "${1:-up}" in
  up)
    "${compose[@]}" up --detach --wait postgres valkey
    bootstrap_roles
    echo "WERK Dev-Infrastruktur ist bereit (PostgreSQL :${WERK_DEV_POSTGRES_PORT}, Valkey :${WERK_DEV_VALKEY_PORT})."
    ;;
  roles)
    bootstrap_roles
    ;;
  status)
    "${compose[@]}" ps postgres valkey
    ;;
  logs)
    "${compose[@]}" logs --follow postgres valkey
    ;;
  down)
    "${compose[@]}" down --remove-orphans
    echo "WERK Dev-Infrastruktur wurde beendet; das PostgreSQL-Volume bleibt erhalten."
    ;;
  *)
    echo "Verwendung: $0 {up|roles|status|logs|down}" >&2
    exit 2
    ;;
esac
