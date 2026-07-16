#!/usr/bin/env sh
set -eu
file=${1:-}
[ -n "$file" ] && [ -f "$file" ] || { echo "Verwendung: ./scripts/restore.sh <backup.dump>"; exit 2; }
[ "${CONFIRM_RESTORE:-}" = "yes" ] || { echo "Abbruch: CONFIRM_RESTORE=yes ist erforderlich."; exit 2; }
docker compose exec -T werk-postgres pg_restore --clean --if-exists -U "${POSTGRES_USER:-werk}" -d "${POSTGRES_DB:-werk}" < "$file"
