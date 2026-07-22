# WERK

WERK ist ein selbst hostbares, modulares Unternehmensbetriebssystem. Der aktuelle
Stand liefert das fachneutrale Plattformfundament: getrennte Dashboard-, API-,
Worker- und Migrationscontainer, PostgreSQL, Valkey, Caddy und versionierte
Core-Migrationen sowie Apache Kafka im KRaft-Modus. Bereits verfügbar sind
getrennt authentifizierte Self-Service-,
Workspace- und Administrationsverträge für Identität, Mandanten,
Organisationseinheiten und Work-Rollen. Geschützte Änderungen schreiben Audit
und bei fachlicher Relevanz Outbox-Ereignisse atomar. Eine MFA- und
berechtigungsgeschützte Audit-Ansicht macht Sicherheitsereignisse ohne freie
interne Detail- oder Session-Rohdaten nachvollziehbar.
Domain-Events, minimierte Security-Audits und strukturierte Laufzeitlogs werden
über getrennte Kafka-Topics mit einem gemeinsamen versionierten Tagging-
Envelope verteilt; PostgreSQL bleibt die verbindliche Wahrheit. Topic-Layout,
Retention und spätere Clustergröße werden mit der Betriebsreife verfeinert;
Änderungen vorbehalten.

## Native Entwicklung

API, Worker, Migration und Dashboard können direkt als lokale Prozesse laufen.
Nur PostgreSQL, Valkey und Kafka bleiben dabei als Infrastruktur in Containern.
Das getrennte Compose-Projekt `werk-dev` verwendet eigene persistente Volumes
und veröffentlicht die Entwicklungsanschlüsse ausschließlich auf Loopback.

Voraussetzungen sind mindestens Go 1.26.5, Node.js 24 und Docker Compose v2.
Ein gemeinsamer Start inklusive Rolleninitialisierung und Migration genügt:

```bash
make dev
```

Danach ist das Dashboard unter `http://localhost:3000` erreichbar. Es leitet die
öffentlichen API-Pfade im Entwicklungsmodus an die native API unter
`http://localhost:8081` weiter. `Ctrl+C` beendet Dashboard, API und Worker; die
Infrastruktur bleibt für den nächsten Start erhalten.

Wer getrennte Terminals bevorzugt, startet die Prozesse so:

```bash
make dev-infra
make dev-migrate
make dev-api
make dev-worker
make dev-dashboard
```

`make dev-infra` wird benötigt, wenn die Infrastruktur noch nicht läuft;
`make dev-migrate` einmalig und nach neuen Migrationen. Beide Befehle sind
idempotent. Die laufenden Prozesse gehören jeweils in ein eigenes Terminal.
Status und Logs der Infrastruktur sind mit `make dev-infra-status` und
`make dev-infra-logs` sichtbar. Beenden ohne Datenverlust:

```bash
make dev-down
```

Für den kurzen Entwicklungszyklus sind keine Container erforderlich. Ein
einzelnes geändertes Paket, ein wiederholter Stabilitätslauf und der vollständige
native Check laufen getrennt:

```bash
make test-focus PKG=./internal/platform/sync
make test-repeat PKG=./internal/platform/sync TEST_COUNT=20
make check-native
```

Ohne installiertes `make`, etwa direkt aus PowerShell, sind die entsprechenden
Go-Befehle:

```powershell
go test ./internal/platform/sync -count=1
go test ./internal/platform/sync -count=20
go test ./...
go vet ./...
go build ./...
```

`make test-fast` beziehungsweise `make test` verwendet den Go-Testcache und
prüft alle Pakete. Datenbank- und Kafka-Integrationstests werden dabei ohne die
expliziten `WERK_TEST_*`-Umgebungsvariablen übersprungen. `make check` ergänzt
den nativen Check um die Compose-Konfigurationsprüfung; den isolierten
Container- und Migrationstest startet erst `make integration-test`. Die
Prüfstufen können mit wachsendem Core weiter verfeinert werden; Änderungen
vorbehalten.

Standardmäßig verwendet PostgreSQL lokal Port `55432`, Valkey `56379`, Kafka
`59092`, die API `8081` und das Dashboard `3000`. Alle Werte können über die in
[`scripts/dev-env.sh`](scripts/dev-env.sh) aufgeführten `WERK_DEV_*`-Variablen
überschrieben werden. Enthält ein eigenes Kennwort URI-Sonderzeichen, muss die
zugehörige vollständige `WERK_DEV_*_DATABASE_URL` percent-codiert gesetzt werden.

Mit bereits nativ installiertem PostgreSQL kann der Infrastrukturstart
übersprungen werden. Die drei rollengetrennten Datenbank-URLs müssen dann auf die
vorbereitete Datenbank zeigen:

```bash
WERK_DEV_SKIP_INFRA=1 make dev
```

Valkey wird im aktuellen Fundament noch nicht von einem Anwendungsprozess
verwendet; der lokale Anschluss ist für die nächste Ausbaustufe vorbereitet.

## Vollständiger Container-Stack

```bash
docker compose up --build -d
```

Danach ist WERK unter `http://localhost:3000` erreichbar.

```bash
curl http://localhost:3000/health/live
curl http://localhost:3000/health/ready
curl http://localhost:3000/meta
```

Der Prometheus-Endpunkt bleibt absichtlich im internen Containernetz:

```bash
docker compose exec api wget -qO- http://127.0.0.1:8080/metrics
```

Die Kafka-Topics und ihre Konfiguration lassen sich intern prüfen:

```bash
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka:19092 --list
```

## Verschlüsseltes Datenbankbackup

WERK erzeugt PostgreSQL-Backups als verschlüsselten Stream. Ein
unverschlüsselter Dump wird nicht auf Platte geschrieben. Der private
Wiederherstellungsschlüssel bleibt außerhalb des Backup-Containers und sollte
getrennt sowie mindestens einmal off-site verwahrt werden.

Ein Schlüsselpaar lässt sich mit dem abgeschotteten Werkzeug-Image erzeugen:

```bash
mkdir -p .dev/recovery
chmod 700 .dev/recovery
docker compose -f compose.yaml -f compose.ops.yaml build backup
docker compose -f compose.yaml -f compose.ops.yaml run --rm --no-deps -T backup keygen > .dev/recovery/werk-backup.key
chmod 600 .dev/recovery/werk-backup.key
docker compose -f compose.yaml -f compose.ops.yaml run --rm --no-deps -T backup recipient < .dev/recovery/werk-backup.key
```

Der letzte Befehl gibt ausschließlich den öffentlichen Empfänger im Format
`age1...` aus. Mit diesem Wert wird ein Backup der laufenden Instanz erzeugt:

```bash
WERK_BACKUP_RECIPIENT=age1... make backup
```

Das Ergebnis liegt standardmäßig unter `backups/` als `.dump.age` zusammen mit
seiner SHA-256-Prüfsumme. `POSTGRES_BACKUP_PASSWORD` gehört wie die übrigen
Datenbankpasswörter in die lokale Secret-Verwaltung und darf in Produktion
keinen Entwicklungswert verwenden.

Ein Restore ist nur für eine frische, getrennte und noch nicht migrierte
Zieldatenbank vorgesehen. Beispiel für ein isoliertes Recovery-Projekt:

```bash
COMPOSE_PROJECT_NAME=werk-recovery docker compose up --detach --wait postgres
COMPOSE_PROJECT_NAME=werk-recovery docker compose run --rm database-roles
COMPOSE_PROJECT_NAME=werk-recovery \
WERK_RESTORE_ARCHIVE=/sicherer/pfad/werk-....dump.age \
WERK_RESTORE_IDENTITY=/sicherer/pfad/werk-backup.key \
WERK_RESTORE_CONFIRM=restore:werk \
make restore
```

Der Restore verweigert eine Datenbank mit vorhandenen WERK-Relationen, prüft die
Ciphertext-Prüfsumme und arbeitet vollständig in einer Transaktion. Erst nach
erfolgreicher Prüfung wird der restliche Stack im Recovery-Projekt gestartet.
Das Verfahren und seine Grenzen sind in
[ADR-005](docs/adr/ADR-005-backup-und-wiederherstellung.md) festgehalten.

## Entwicklung und Prüfung

```bash
make fmt
make check
make compose-build
```

Die CI prüft Formatierung, `go vet`, Tests mit Race Detector, die Compose-
Konfiguration, wiederholbare Migrationen, PostgreSQL-Rollen, RLS-Isolation und
alle eigenen Container-Images.

## Versionierte Releases

`Canary` ist der Entwicklungs- und Integrationskanal. Ein Tag im Format
`vMAJOR.MINOR.PATCH` oder `vMAJOR.MINOR.PATCH-PRERELEASE` startet die
Release-Pipeline, sofern der Commit Bestandteil von `Canary` ist. Sie wiederholt
die vollständigen Qualitäts-, Migrations- und Restore-Prüfungen und
veröffentlicht anschließend:

- Linux-Archive für `amd64` und `arm64` samt SHA-256-Prüfsummen,
- getrennte Multi-Arch-Images für API, Worker, Migration, Dashboard und Backup,
- signierte Herkunftsnachweise sowie Image-SBOM und -Provenance.

Ein freigegebenes Image wird mit seiner konkreten Version verwendet:

```bash
WERK_BUILD_VERSION=0.1.0-alpha.1 \
docker compose -f compose.yaml -f compose.release.yaml up -d
```

Für die Operations-Profile kommt der zweite Release-Overlay zuletzt hinzu:

```bash
WERK_BUILD_VERSION=0.1.0-alpha.1 \
docker compose -f compose.yaml -f compose.ops.yaml \
  -f compose.release.yaml -f compose.release.ops.yaml config
```

Die Release-Pipeline deployt nicht automatisch. Ein Quellstand auf GitHub ist
außerdem kein Ersatz für die verschlüsselten Datenbank- und späteren
Object-Storage-Backups. Versionierung und Lieferkette beschreibt
[ADR-019](docs/adr/ADR-019-release-kanal-und-softwarelieferkette.md); Änderungen
vorbehalten.

Der vollständige Datenbank-Sicherheitstest verwendet ein eigenes Docker-Projekt
mit Wegwerf-Volume:

```bash
make integration-test
```

Jeder Lauf erhält automatisch einen eigenen Compose-Projektnamen. Dadurch können
mehrere Integrationstests parallel laufen, ohne gegenseitig Container, Netze oder
Volumes zu entfernen. Für eine gezielte Fehlersuche kann der Name mit
`WERK_TEST_PROJECT_NAME` fest vorgegeben werden.

Der verschlüsselte End-to-End-Restore wird separat in zwei isolierten
PostgreSQL-Volumes geprüft:

```bash
make restore-test
```

Die lokale Compose-Datei verwendet erkennbare Entwicklungskennwörter. Vor jeder
nach außen erreichbaren Instanz müssen alle Werte aus `.env.example` durch
eigene starke Secrets ersetzt werden. `WERK_ENV=production` lehnt die bekannten
Entwicklungskennwörter und den PostgreSQL-Bootstrap-Login technisch ab. Der
produktive API-Prozess startet außerdem nur mit nativem `tls` oder `mtls`:

```bash
WERK_HTTP_TLS_MODE=tls
WERK_HTTP_TLS_CERT_FILE=/run/secrets/api-server.pem
WERK_HTTP_TLS_KEY_FILE=/run/secrets/api-server-key.pem
```

`mtls` verlangt zusätzlich `WERK_HTTP_TLS_CLIENT_CA_FILE`. Ein Reverse Proxy ist
dafür nicht erforderlich. Wird TLS trotzdem vor der API terminiert, vertraut
die Anwendung `X-Forwarded-Proto` nur für Netze aus
`WERK_HTTP_TRUSTED_PROXY_CIDRS`. Produktions-URLs zu PostgreSQL verwenden
`sslmode=verify-full`; Änderungen vorbehalten.

API-Namensräume bleiben hart getrennt:

- `/api/v1` – Arbeitskonten und Workspace
- `/admin/v1` – ausschließlich Administrationskonten
- `/service/v1` – ausschließlich technische Service-Identitäten

Der aktuelle fachneutrale Vertrag liegt als
[OpenAPI-Dokument](api/openapi.json) vor. Fehler verwenden RFC-9457-kompatible
Problem Details und jede Antwort enthält Request- und Correlation-ID.

Container-Stack beenden ohne Datenverlust:

```bash
docker compose down
```

`docker compose down -v` entfernt zusätzlich das lokale PostgreSQL-Entwicklungsvolume.

## Architektur

- [Gesamtprojektziel](docs/WERK_GESAMTPROJEKTZIEL.md)
- [Vision](docs/vision.md)
- [Datenmodell](docs/DATENMODELL.md)
- [Roadmap](docs/ROADMAP.md)
- [Backend-Implementierungsstand](docs/BACKEND-IMPLEMENTIERUNGSSTAND.md)
- [Clientarchitektur](docs/CLIENT-ARCHITEKTUR.md)
- [Architekturentscheidungen](docs/adr/)
- [ADR-013: Kotlin-/Compose-Multiplatform-Clients](docs/adr/ADR-013-native-clients-kotlin-compose-multiplatform.md)
- [ADR-014: Principals, Provider, Credentials und Audiences](docs/adr/ADR-014-principals-provider-credentials-und-audiences.md)
- [ADR-015: Identity-Autorität, Witness und Failover](docs/adr/ADR-015-identity-authority-witness-und-failover.md)
- [ADR-020: Kafka für Event-, Audit- und Log-Streaming](docs/adr/ADR-020-kafka-event-audit-und-log-streaming.md)
- [ADR-022: Deploymentprofile und Platform Witness](docs/adr/ADR-022-deploymentprofile-und-platform-witness.md)
- [ADR-023: Native Server-TLS- und Transportidentität](docs/adr/ADR-023-native-server-tls-und-transportidentitaet.md)
- [Betriebsprofil](docs/BETRIEBSPROFIL.md)
- [KI-Datenklassifikation](docs/KI_DATENKLASSIFIKATION.md)
- [API-Grundvertrag](docs/API_GRUNDVERTRAG.md)
- [PostgreSQL-Rollen und Tenant-RLS](docs/adr/ADR-004-postgresql-rollen-und-tenant-rls.md)
