#!/usr/bin/env sh
set -eu
failed=0
for command in git docker make openssl; do
  if command -v "$command" >/dev/null 2>&1; then printf '%-12s ok\n' "$command"; else printf '%-12s fehlt\n' "$command"; failed=1; fi
done
docker compose version >/dev/null 2>&1 || failed=1
[ -f .env ] || echo "Hinweis: .env fehlt; make setup erzeugt sie."
exit "$failed"
