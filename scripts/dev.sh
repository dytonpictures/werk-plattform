#!/usr/bin/env sh
set -eu
docker compose up -d --build
echo "Web: http://127.0.0.1:${WERK_WEB_PORT:-3000}"
echo "API: http://127.0.0.1:8080"
