#!/usr/bin/env sh
set -eu
make lint
make test
docker compose config --quiet
