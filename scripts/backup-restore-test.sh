#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd -- "${script_dir}/.." && pwd)"
project_name="${WERK_BACKUP_TEST_PROJECT_NAME:-werk-backup-restore-$(date -u +%s)-$$}"

if [[ ! "${project_name}" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  echo "WERK_BACKUP_TEST_PROJECT_NAME must start with a lowercase letter or digit and contain only lowercase letters, digits, underscores, or hyphens" >&2
  exit 2
fi

artifacts="$(mktemp -d "${TMPDIR:-/tmp}/werk-backup-restore.XXXXXX")"
archive="${artifacts}/werk.dump.age"
identity="${artifacts}/identity.key"
wrong_identity="${artifacts}/wrong-identity.key"

bootstrap_password="${POSTGRES_PASSWORD:-werk}"
migrator_password="${POSTGRES_MIGRATOR_PASSWORD:-werk-migrator-dev}"
work_password="${POSTGRES_WORK_PASSWORD:-werk-work-dev}"
identity_password="${POSTGRES_IDENTITY_PASSWORD:-werk-identity-dev}"
admin_password="${POSTGRES_ADMIN_PASSWORD:-werk-admin-dev}"
service_password="${POSTGRES_SERVICE_PASSWORD:-werk-service-dev}"
worker_password="${POSTGRES_WORKER_PASSWORD:-werk-worker-dev}"
backup_password="${POSTGRES_BACKUP_PASSWORD:-werk-backup-dev}"

export POSTGRES_PASSWORD="${bootstrap_password}"
export POSTGRES_MIGRATOR_PASSWORD="${migrator_password}"
export POSTGRES_WORK_PASSWORD="${work_password}"
export POSTGRES_IDENTITY_PASSWORD="${identity_password}"
export POSTGRES_ADMIN_PASSWORD="${admin_password}"
export POSTGRES_SERVICE_PASSWORD="${service_password}"
export POSTGRES_WORKER_PASSWORD="${worker_password}"
export POSTGRES_BACKUP_PASSWORD="${backup_password}"

compose=(
  docker compose
  --project-name "${project_name}"
  --file "${project_root}/compose.yaml"
  --file "${project_root}/compose.test.yaml"
  --file "${project_root}/compose.ops.yaml"
  --file "${project_root}/compose.backup-test.yaml"
)

owns_project=false
cleanup() {
  if [[ "${owns_project}" = true ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  rm -rf -- "${artifacts}"
}
trap cleanup EXIT INT TERM

existing_containers="$(docker container ls --all --quiet --filter "label=com.docker.compose.project=${project_name}")"
existing_volumes="$(docker volume ls --quiet --filter "label=com.docker.compose.project=${project_name}")"
existing_networks="$(docker network ls --quiet --filter "label=com.docker.compose.project=${project_name}")"
if [[ -n "${existing_containers}${existing_volumes}${existing_networks}" ]]; then
  echo "refusing to reuse an existing Compose project: ${project_name}" >&2
  exit 2
fi
owns_project=true

cd "${project_root}"

"${compose[@]}" build backup migrate integration-restore
"${compose[@]}" up --detach --wait postgres
"${compose[@]}" run --rm --no-deps database-roles
"${compose[@]}" run --rm --no-deps migrate

"${compose[@]}" exec --no-TTY postgres psql -X --username=werk --dbname=werk --set=ON_ERROR_STOP=1 <<'SQL'
SET ROLE werk_owner;

INSERT INTO werk_core.tenants (
    id, name, status, default_locale, default_timezone
) VALUES
    ('0196f000-0000-7000-8000-000000000001', 'WERK Restore Ä', 'active', 'de-DE', 'Europe/Berlin'),
    ('0196f000-0000-7000-8000-000000000002', 'WERK Restore B', 'active', 'de-DE', 'Europe/Berlin');

INSERT INTO werk_core.organizational_units (
    id, tenant_id, unit_type, name, status
) VALUES
    ('0196f000-0000-7000-8000-000000000011', '0196f000-0000-7000-8000-000000000001', 'company', 'Werk A', 'active'),
    ('0196f000-0000-7000-8000-000000000012', '0196f000-0000-7000-8000-000000000002', 'company', 'Werk B', 'active');

INSERT INTO werk_core.admin_subjects (id, display_name, status)
VALUES ('0196f000-0000-7000-8000-000000000021', 'Restore-Prüfung', 'active');
SQL

source_checksums="$("${compose[@]}" exec --no-TTY postgres psql -XAtq --username=werk --dbname=werk --command="
  SET ROLE werk_owner;
  SELECT name || ':' || checksum FROM werk_core.schema_migrations ORDER BY name;
")"

"${compose[@]}" run --rm --no-deps -T backup keygen >"${identity}"
"${compose[@]}" run --rm --no-deps -T backup keygen >"${wrong_identity}"
chmod 0600 "${identity}" "${wrong_identity}"
recipient="$("${compose[@]}" run --rm --no-deps -T backup recipient <"${identity}")"
if [[ ! "${recipient}" =~ ^age1[0-9a-z]+$ ]]; then
  echo "generated age recipient is invalid" >&2
  exit 1
fi

"${compose[@]}" run --rm --no-deps -T \
  --env "WERK_BACKUP_RECIPIENT=${recipient}" \
  backup backup >"${archive}.partial"
mv -- "${archive}.partial" "${archive}"

if [[ ! -s "${archive}" ]] || [[ "$(head -n 1 "${archive}")" != "age-encryption.org/v1" ]]; then
  echo "encrypted backup artifact is invalid" >&2
  exit 1
fi
if LC_ALL=C grep --text --fixed-strings --quiet 'WERK Restore' "${archive}"; then
  echo "encrypted archive exposes fixture plaintext" >&2
  exit 1
fi
if find "${artifacts}" -maxdepth 1 -type f \( -name '*.sql' -o -name '*.dump' -o -name '*.tar' -o -name '*.backup' \) -print -quit | grep --quiet .; then
  echo "an unencrypted dump artifact was created" >&2
  exit 1
fi

"${compose[@]}" up --detach --wait restore-postgres
"${compose[@]}" run --rm --no-deps restore-database-roles

if "${compose[@]}" run --rm --no-deps -T \
  --user "$(id -u):$(id -g)" \
  --volume "${wrong_identity}:/run/secrets/werk-backup-identity:ro" \
  --env WERK_RESTORE_CONFIRM=restore:werk_restore \
  restore restore <"${archive}"; then
  echo "restore with an unrelated age identity unexpectedly succeeded" >&2
  exit 1
fi

target_has_relations="$("${compose[@]}" exec --no-TTY restore-postgres psql -XAtq --username=werk --dbname=werk_restore --command="
  SELECT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_class AS relation
    JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
    WHERE namespace.nspname = 'werk_core' AND relation.relkind IN ('r', 'p')
  );
")"
if [[ "${target_has_relations}" != "f" ]]; then
  echo "failed restore changed the empty target database" >&2
  exit 1
fi

"${compose[@]}" run --rm --no-deps -T \
  --user "$(id -u):$(id -g)" \
  --volume "${identity}:/run/secrets/werk-backup-identity:ro" \
  --env WERK_RESTORE_CONFIRM=restore:werk_restore \
  restore restore <"${archive}"

"${compose[@]}" run --rm --no-deps \
  --env "DATABASE_URL=postgres://werk_migrator:${migrator_password}@restore-postgres:5432/werk_restore?sslmode=disable" \
  migrate

target_checksums="$("${compose[@]}" exec --no-TTY restore-postgres psql -XAtq --username=werk --dbname=werk_restore --command="
  SET ROLE werk_owner;
  SELECT name || ':' || checksum FROM werk_core.schema_migrations ORDER BY name;
")"
if [[ "${source_checksums}" != "${target_checksums}" ]]; then
  echo "migration checksums differ after restore" >&2
  exit 1
fi

"${compose[@]}" exec --no-TTY restore-postgres psql -X --username=werk --dbname=werk_restore --set=ON_ERROR_STOP=1 <<'SQL'
SET ROLE werk_owner;

DO $verification$
BEGIN
    IF (SELECT count(*) FROM werk_core.tenants) <> 2 THEN
        RAISE EXCEPTION 'restored tenant count is invalid';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM werk_core.tenants
        WHERE id = '0196f000-0000-7000-8000-000000000001'
          AND name = 'WERK Restore Ä'
    ) THEN
        RAISE EXCEPTION 'restored unicode fixture is missing';
    END IF;
    IF (SELECT count(*) FROM werk_core.organizational_units) <> 2 THEN
        RAISE EXCEPTION 'restored organizational unit count is invalid';
    END IF;
    IF (SELECT count(*) FROM werk_core.admin_subjects) <> 1 THEN
        RAISE EXCEPTION 'restored admin subject count is invalid';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_class AS relation
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
        WHERE namespace.nspname IN ('werk_core', 'werk_security')
          AND relation.relkind IN ('r', 'p', 'S', 'v', 'm', 'f')
          AND pg_catalog.pg_get_userbyid(relation.relowner) <> 'werk_owner'
    ) THEN
        RAISE EXCEPTION 'a restored WERK relation has the wrong owner';
    END IF;
    IF (
        SELECT count(*)
        FROM pg_catalog.pg_class AS relation
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
        WHERE namespace.nspname = 'werk_core'
          AND relation.relname IN ('tenants', 'organizational_units')
          AND relation.relrowsecurity
          AND relation.relforcerowsecurity
    ) <> 2 THEN
        RAISE EXCEPTION 'RLS was not restored completely';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_catalog.pg_class AS relation
        JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
        WHERE namespace.nspname IN ('werk_core', 'werk_security')
          AND relation.relkind IN ('r', 'p', 'S', 'v', 'm', 'f')
          AND NOT pg_catalog.has_table_privilege('werk_backup_reader', relation.oid, 'SELECT')
    ) THEN
        RAISE EXCEPTION 'backup grants were not restored completely';
    END IF;
END
$verification$;
SQL

"${compose[@]}" run --rm --no-deps --build integration-restore

source_marker_count="$("${compose[@]}" exec --no-TTY postgres psql -XAtq --username=werk --dbname=werk --command="
  SET ROLE werk_owner;
  SELECT count(*) FROM werk_core.tenants WHERE name = 'WERK Restore Ä';
")"
if [[ "${source_marker_count}" != "1" ]]; then
  echo "source database changed during restore test" >&2
  exit 1
fi

echo "encrypted backup and isolated restore verified"
