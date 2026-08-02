# WERK Plattform

WERK ist ein modulares, selbst hostbares Unternehmensbetriebssystem. Der Go-
Core stellt Identität, Organisation, Rechte, Dokumente, Audit, Ereignisse und
Betrieb bereit. PostgreSQL bleibt die fachliche Wahrheit. Die Weboberfläche ist
in der API eingebettet; ein zusätzlicher Webserver oder Frontend-Prozess ist für
den normalen Betrieb nicht erforderlich.

## Voraussetzungen

- Debian oder Ubuntu auf `amd64`
- Go 1.26.5 oder neuer
- PostgreSQL 18
- optional Apache Kafka für Event-, Audit- und Log-Streaming
- für verschlüsselte Sicherungen PostgreSQL-Clientwerkzeuge und `age`

## Erster Start

Die lokale Konfiguration und alle Zugangsdaten liegen in genau einer nicht
versionierten `.env`:

```bash
cp .env.example .env
chmod 600 .env
```

Vor dem ersten Start müssen alle `CHANGE_ME`-Werte ersetzt und die getrennten
PostgreSQL-Rollen aus [`deploy/postgres/bootstrap-roles.sql`](deploy/postgres/bootstrap-roles.sql)
auf einer PostgreSQL-18-Datenbank eingerichtet werden. Die API erhält weiterhin
nur Work-, Identity- und Admin-Runtime-Zugänge; Worker und Migration verwenden
ihre eigenen Rollen.

Unter Debian und Ubuntu startet ein Befehl Konfigurationsprüfung, Migrationen
und die API mit eingebetteter Weboberfläche:

```bash
sh scripts/start.sh
```

Die Adresse kommt aus `WERK_HTTP_ADDRESS`; die Vorlage verwendet
`http://127.0.0.1:3000`. Der initiale Administrator heißt
`admin@werk.local` und muss das in `.env` gesetzte temporäre Passwort beim
ersten Login ändern.

Kafka ist im Minimalbetrieb deaktiviert. PostgreSQL-Outbox und Auditexport-
Queue bleiben dabei dauerhaft erhalten. Wird Kafka später aktiviert, läuft der
Worker getrennt:

```bash
go run ./cmd/worker
```

## Direkte Befehle

```bash
go run ./cmd/werkctl version
go run ./cmd/werkctl doctor --env .env --config-only
go run ./cmd/werkctl doctor --env .env --url http://127.0.0.1:3000
go run ./cmd/werkctl doctor --env .env --json
go run ./cmd/werkctl status --url http://127.0.0.1:3000
go run ./cmd/werkctl migrate --env .env
go run ./cmd/api
```

`doctor` prüft Konfiguration, Listener/TLS, die getrennten Datenbankrollen und
optionale Abhängigkeiten mit stabilen `PASS`-/`WARN`-/`FAIL`-Checks. `status`
liest nur die öffentlichen WERK-Metadaten sowie Liveness und Readiness; es
behauptet keinen Worker-, Kafka-, Migrations-, Host- oder Updatezustand. Beide
Diagnosebefehle können denselben Ergebnisvertrag mit `--json` ausgeben und
führen keine Reparaturen aus.

`migrate` ist bereits beim Aufruf verändernd: Es wendet alle noch offenen,
checksumgebundenen Aufwärtsmigrationen an. Einen `plan`- oder Dry-Run-Modus gibt
es derzeit nicht. Start, Update und Hybrid-Cloud-Pairing sind ebenfalls noch
keine ausführbaren `werkctl`-Befehle.

Bereits gesetzte Betriebssystemvariablen haben Vorrang vor gleichnamigen
Werten aus `.env`. Mit `WERK_ENV_FILE` kann für API, Worker und Migration ein
anderer Pfad ausdrücklich vorgegeben werden.

## Entwicklung und Prüfung

```bash
gofmt -w cmd internal dashboard packaging
go vet ./...
go test ./...
go test -race ./...
for werk_js_file in dashboard/public/*.js dashboard/server.mjs; do
  node --check "$werk_js_file"
done
jq empty api/openapi.json
```

Datenbank-, RLS- und Kafka-Integrationstests benötigen explizite
`WERK_TEST_*`-Verbindungen zu dafür vorgesehenen Wegwerf-Instanzen. Ohne diese
Variablen werden ausschließlich die unabhängigen Tests ausgeführt.

## Native Linux-Pakete

Das `amd64`-Archiv und Debian-Paket entstehen mit:

```bash
sh scripts/package-release.sh 0.1.0-preview.0 dist
```

Die Pakete enthalten API, Worker, Migration, `werkctl`, systemd-Units und
absichtlich nicht vorkonfigurierte Produktionsvorlagen. PostgreSQL, Kafka und
Valkey werden nicht eingebettet.

## Backup und Wiederherstellung

Der native Helfer [`deploy/backup/werk-backup`](deploy/backup/werk-backup)
verwendet `pg_dump`, `pg_restore` und `age`. Sicherungen werden ausschließlich
verschlüsselt geschrieben. Eine Wiederherstellung verlangt eine leere
Zieldatenbank, den passenden privaten Schlüssel und die explizite Bestätigung
`restore:<datenbankname>`.

## Architektur

- [Gesamtprojektziel](docs/WERK_GESAMTPROJEKTZIEL.md)
- [Vision](docs/vision.md)
- [Datenmodell](docs/DATENMODELL.md)
- [Roadmap](docs/ROADMAP.md)
- [Betriebsprofil](docs/BETRIEBSPROFIL.md)
- [Identity und MFA](docs/IDENTITY-MFA.md)
- [Architekturentscheidungen](docs/adr/)

Angewendete Migrationen unter `internal/platform/migrate/migrations` werden
niemals nachträglich verändert.
