#!/usr/bin/env sh
set -eu
if [ ! -f .env ]; then
  password=$(openssl rand -hex 32)
  sed "s/replace-with-a-long-random-password/$password/g" .env.example > .env
  chmod 600 .env
  echo ".env mit zufälligem Datenbankpasswort erzeugt."
fi
[ "${1:-}" = "--env-only" ] && exit 0
docker compose up -d --build
docker compose run --rm werk-api migrate
./scripts/wait-for-health.sh
