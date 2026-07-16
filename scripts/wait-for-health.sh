#!/usr/bin/env sh
set -eu
attempt=0
until curl -fsS "${WERK_API_HEALTH_URL:-http://127.0.0.1:8080}/ready" >/dev/null && curl -fsS "${WERK_WEB_HEALTH_URL:-http://127.0.0.1:3000/api/health}" >/dev/null; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 30 ] || { echo "Health Checks nach 60 Sekunden nicht erfolgreich."; exit 1; }
  sleep 2
done
echo "WERK API und Web sind bereit."
