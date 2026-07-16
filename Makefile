.DEFAULT_GOAL := help
COMPOSE := docker compose

.PHONY: help doctor setup bootstrap up down restart ps logs build lint format test test-unit test-integration test-e2e migrate migrate-status seed bootstrap-admin set-admin-password health backup restore audit-check clean reset-dev-data

help: ## Verfügbare Befehle anzeigen
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Lokale Voraussetzungen prüfen
	@./scripts/doctor.sh

setup: ## Lokale .env sicher vorbereiten
	@./scripts/bootstrap.sh --env-only

bootstrap: ## Umgebung vorbereiten, bauen und starten
	@./scripts/bootstrap.sh

up: ## Dienste starten
	$(COMPOSE) up -d --build

down: ## Dienste ohne Datenverlust stoppen
	$(COMPOSE) down

restart: ## Dienste neu starten
	$(COMPOSE) restart

ps: ## Dienststatus anzeigen
	$(COMPOSE) ps

logs: ## Dienstlogs verfolgen
	$(COMPOSE) logs -f --tail=200

build: ## Backend und Frontend bauen
	$(COMPOSE) build

lint: ## Statische Prüfungen ausführen
	docker run --rm -v "$(CURDIR)/apps/api:/src" -w /src golang:1.26.1-alpine go vet ./...
	docker run --rm -v "$(CURDIR):/workspace" -w /workspace node:24.18.0-alpine npm run lint
	docker run --rm -v "$(CURDIR):/workspace" -w /workspace node:24.18.0-alpine npm run typecheck

format: ## Go-Code formatieren
	docker run --rm -v "$(CURDIR)/apps/api:/src" -w /src golang:1.26.1-alpine gofmt -w .

test: test-unit ## Alle derzeit vorhandenen Tests ausführen

test-unit: ## Unit-Tests ausführen
	docker run --rm -v "$(CURDIR)/apps/api:/src" -w /src golang:1.26.1-alpine go test ./...
	docker run --rm -v "$(CURDIR):/workspace" -w /workspace node:24.18.0-alpine npm test

test-integration: ## Integrationstests ausführen
	@echo "Noch keine Integrationstests implementiert."

test-e2e: ## E2E-Tests ausführen
	@echo "Noch keine E2E-Tests implementiert."

migrate: ## Datenbankmigrationen anwenden
	$(COMPOSE) run --rm werk-api migrate

migrate-status: ## Migrationsstatus anzeigen
	@echo "Statusprüfung folgt mit dem Migrationsframework."

seed: ## Idempotente Systemdaten erzeugen
	@echo "Seeds folgen mit dem Identity-Modul."

bootstrap-admin: ## Ersten Administrator anlegen
	@test -n "$(WERK_BOOTSTRAP_EMAIL)" -a -n "$(WERK_BOOTSTRAP_PASSWORD)" || (echo "WERK_BOOTSTRAP_EMAIL und WERK_BOOTSTRAP_PASSWORD setzen"; exit 2)
	$(COMPOSE) run --rm -e WERK_BOOTSTRAP_EMAIL="$(WERK_BOOTSTRAP_EMAIL)" -e WERK_BOOTSTRAP_PASSWORD="$(WERK_BOOTSTRAP_PASSWORD)" werk-api bootstrap-admin

set-admin-password: ## Administratorpasswort sicher ändern
	@echo "Passwortverwaltung folgt mit dem Identity-Modul."

health: ## Health- und Readiness-Endpunkte prüfen
	@./scripts/wait-for-health.sh

backup: ## PostgreSQL sichern
	@./scripts/backup.sh

restore: ## PostgreSQL wiederherstellen
	@./scripts/restore.sh

audit-check: ## Audit-Konfiguration prüfen
	@echo "Auditprüfung folgt mit dem Audit-Modul."

clean: ## Flüchtige Build-Ausgaben löschen, keine Geschäftsdaten
	rm -rf apps/web/.next coverage

reset-dev-data: ## Entwicklungsdaten nach expliziter Bestätigung löschen
	@test "$(CONFIRM)" = "yes" || (echo "Abbruch: CONFIRM=yes erforderlich"; exit 1)
	$(COMPOSE) down -v
