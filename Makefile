GO_PACKAGES := ./...
GO ?= go
GOFMT ?= gofmt
NODE ?= node
TEST_COUNT ?= 20

.PHONY: fmt fmt-check vet test test-fast test-focus test-repeat check-native check build compose-config compose-build up down logs \
	migration-test integration-test backup restore restore-test dev dev-apps dev-infra dev-db-roles \
	dev-infra-status dev-infra-logs dev-migrate dev-api dev-worker \
	dev-dashboard dev-down

fmt:
	$(GOFMT) -w cmd internal

fmt-check:
	@files="$$($(GOFMT) -l cmd internal)"; if [ -n "$$files" ]; then echo "Go files need formatting:"; echo "$$files"; exit 1; fi

vet:
	$(GO) vet $(GO_PACKAGES)

test: test-fast

test-fast:
	$(GO) test $(GO_PACKAGES)

test-focus:
	@if [ -z "$(PKG)" ]; then echo "PKG is required, for example: make test-focus PKG=./internal/platform/sync"; exit 2; fi
	$(GO) test $(PKG) -count=1

test-repeat:
	@if [ -z "$(PKG)" ]; then echo "PKG is required, for example: make test-repeat PKG=./internal/platform/sync TEST_COUNT=20"; exit 2; fi
	$(GO) test $(PKG) -count=$(TEST_COUNT)

check-native: fmt-check vet test-fast build

check: check-native compose-config

build:
	$(GO) build $(GO_PACKAGES)

compose-config:
	docker compose config --quiet
	docker compose -f compose.yaml -f compose.dev.yaml config --quiet
	docker compose -f compose.yaml -f compose.ops.yaml config --quiet
	docker compose -f compose.yaml -f compose.test.yaml -f compose.ops.yaml -f compose.backup-test.yaml config --quiet
	WERK_BUILD_VERSION=0.0.0 docker compose -f compose.yaml -f compose.release.yaml config --quiet
	WERK_BUILD_VERSION=0.0.0 docker compose -f compose.yaml -f compose.ops.yaml -f compose.release.yaml -f compose.release.ops.yaml config --quiet

compose-build:
	docker compose build
	docker compose -f compose.yaml -f compose.ops.yaml build backup

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

migration-test: integration-test

integration-test:
	sh scripts/integration-test.sh

backup:
	bash scripts/backup.sh

restore:
	bash scripts/restore.sh

restore-test:
	bash scripts/backup-restore-test.sh

dev:
	GO="$(GO)" NODE="$(NODE)" bash scripts/dev.sh

dev-apps:
	GO="$(GO)" NODE="$(NODE)" WERK_DEV_SKIP_INFRA=1 bash scripts/dev.sh

dev-infra:
	bash scripts/dev-infra.sh up

dev-db-roles:
	bash scripts/dev-infra.sh roles

dev-infra-status:
	bash scripts/dev-infra.sh status

dev-infra-logs:
	bash scripts/dev-infra.sh logs

dev-migrate:
	GO="$(GO)" bash scripts/dev-service.sh migrate

dev-api:
	GO="$(GO)" bash scripts/dev-service.sh api

dev-worker:
	GO="$(GO)" bash scripts/dev-service.sh worker

dev-dashboard:
	NODE="$(NODE)" bash scripts/dev-service.sh dashboard

dev-down:
	bash scripts/dev-infra.sh down
