#!/bin/sh
set -eu

: "${PGHOST:?PGHOST is required}"
: "${PGDATABASE:?PGDATABASE is required}"
: "${PGUSER:?PGUSER is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"
: "${WERK_DB_NAME:?WERK_DB_NAME is required}"
: "${WERK_MIGRATOR_PASSWORD:?WERK_MIGRATOR_PASSWORD is required}"
: "${WERK_WORK_PASSWORD:?WERK_WORK_PASSWORD is required}"
: "${WERK_IDENTITY_PASSWORD:?WERK_IDENTITY_PASSWORD is required}"
: "${WERK_ADMIN_PASSWORD:?WERK_ADMIN_PASSWORD is required}"
: "${WERK_SERVICE_PASSWORD:?WERK_SERVICE_PASSWORD is required}"
: "${WERK_WORKER_PASSWORD:?WERK_WORKER_PASSWORD is required}"
: "${WERK_BACKUP_PASSWORD:?WERK_BACKUP_PASSWORD is required}"

if [ "${WERK_ENV:-development}" = "production" ]; then
  case "${PGPASSWORD}:${WERK_MIGRATOR_PASSWORD}:${WERK_WORK_PASSWORD}:${WERK_IDENTITY_PASSWORD}:${WERK_ADMIN_PASSWORD}:${WERK_SERVICE_PASSWORD}:${WERK_WORKER_PASSWORD}:${WERK_BACKUP_PASSWORD}" in
    *werk-migrator-dev*|*werk-work-dev*|*werk-identity-dev*|*werk-admin-dev*|*werk-service-dev*|*werk-worker-dev*|*werk-backup-dev*|werk:*)
      echo "development database credentials are forbidden in production" >&2
      exit 1
      ;;
  esac
fi

psql \
  --no-psqlrc \
  --set=ON_ERROR_STOP=1 \
  --set=database_name="${WERK_DB_NAME}" \
  --set=migrator_password="${WERK_MIGRATOR_PASSWORD}" \
  --set=work_password="${WERK_WORK_PASSWORD}" \
  --set=identity_password="${WERK_IDENTITY_PASSWORD}" \
  --set=admin_password="${WERK_ADMIN_PASSWORD}" \
  --set=service_password="${WERK_SERVICE_PASSWORD}" \
  --set=worker_password="${WERK_WORKER_PASSWORD}" \
  --set=backup_password="${WERK_BACKUP_PASSWORD}" \
  --file=/bootstrap/bootstrap-roles.sql
