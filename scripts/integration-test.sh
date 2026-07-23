#!/bin/sh
set -eu

project_name="${WERK_TEST_PROJECT_NAME:-werk-integration-$$}"

case "${project_name}" in
  "" | [!a-z0-9]* | *[!a-z0-9_-]*)
    echo "WERK_TEST_PROJECT_NAME must start with a lowercase letter or digit and contain only lowercase letters, digits, underscores, or hyphens" >&2
    exit 2
    ;;
esac

bootstrap_password="${POSTGRES_PASSWORD:-werk}"
migrator_password="${POSTGRES_MIGRATOR_PASSWORD:-werk-migrator-dev}"
work_password="${POSTGRES_WORK_PASSWORD:-werk-work-dev}"
identity_password="${POSTGRES_IDENTITY_PASSWORD:-werk-identity-dev}"
worker_password="${POSTGRES_WORKER_PASSWORD:-werk-worker-dev}"
backup_password="${POSTGRES_BACKUP_PASSWORD:-werk-backup-dev}"

export POSTGRES_PASSWORD="${bootstrap_password}"
export POSTGRES_MIGRATOR_PASSWORD="${migrator_password}"
export POSTGRES_WORK_PASSWORD="${work_password}"
export POSTGRES_IDENTITY_PASSWORD="${identity_password}"
export POSTGRES_WORKER_PASSWORD="${worker_password}"
export POSTGRES_BACKUP_PASSWORD="${backup_password}"

compose() {
  docker compose \
    --project-name "${project_name}" \
    --file compose.yaml \
    --file compose.test.yaml \
    "$@"
}

cleanup() {
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cleanup
compose up --detach --wait postgres kafka
compose run --rm database-roles
compose run --rm migrate
compose run --rm migrate
compose run --rm kafka-init
compose run --rm --build integration
