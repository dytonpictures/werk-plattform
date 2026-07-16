#!/usr/bin/env sh
set -eu
mkdir -p backups
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
umask 077
docker compose exec -T werk-postgres pg_dump -U "${POSTGRES_USER:-werk}" -d "${POSTGRES_DB:-werk}" -Fc > "backups/werk-$timestamp.dump"
echo "Backup erstellt: backups/werk-$timestamp.dump"
