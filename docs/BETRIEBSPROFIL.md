# WERK – Betriebsprofil 1: nativer Single Host

**Status:** Startprofil für interne Produktiv- und Testinstanzen  
**Stand:** 2026-07-29

## Ziel

WERK läuft auf einem vom Unternehmen kontrollierten Debian-/Ubuntu-Host auf
`amd64` als kleine Gruppe nativer Prozesse. Die API liefert die
Business-API und die eingebettete Weboberfläche auf demselben Origin. Worker und
Migration bleiben eigene Prozesse, PostgreSQL bleibt die fachliche Wahrheit.

## Einfacher Start

Im Quellstand ist `.env` die zentrale Konfigurations- und Secret-Datei:

```sh
cp .env.example .env
chmod 600 .env
sh scripts/start.sh
```

Die Startskripte validieren `.env`, führen Migrationen aus und starten die API.
Das Dashboard braucht keinen eigenen Prozess. Kafka ist im einfachen Profil
deaktiviert; bei aktivierter Ereigniszustellung startet der Worker separat mit
`go run ./cmd/worker`.

## Prozess- und Rechtegrenzen

- `werk-api` bedient Web und HTTP-API, prüft Tenant und Berechtigungen
  serverseitig und verwendet getrennte Work-, Identity- und Admin-Rollen.
- `werk-worker` verarbeitet Outbox und Auditexport mit einer eigenen
  Non-Owner-Datenbankrolle.
- `werk-migrate` führt einmalige, versionierte Migrationen über die
  Migrator-Rolle aus.
- PostgreSQL ist das einzige fachliche System of Record.
- Valkey und Kafka sind optionale, austauschbare Infrastruktur und nie die
  alleinige Wahrheit.

Die Konfiguration liegt zentral, die Zugangsdaten bleiben trotzdem nach Zweck
getrennt: `WORK_DATABASE_URL`, `IDENTITY_DATABASE_URL`, `ADMIN_DATABASE_URL`,
`WORKER_DATABASE_URL` und `MIGRATOR_DATABASE_URL` verwenden verschiedene
PostgreSQL-Rollen. Ein Bootstrap-Superuser ist kein Laufzeitcredential.

## Mindestbetrieb

- Entwicklung ohne TLS bindet ausschließlich an Loopback.
- Produktion verlangt direktes TLS oder mTLS und PostgreSQL mit
  `sslmode=verify-full`.
- `.env` wird nicht versioniert. Unter Linux ist sie nur für den Betreiber und
  die ausdrücklich berechtigte WERK-Gruppe lesbar.
- Vor Updates werden Datenbackup, Wiederherstellbarkeit, Migrationspfad und
  Zielversion geprüft.
- `/health/live` und `/health/ready` werden überwacht; strukturierte Logs dürfen
  keine Secrets oder unnötigen Geschäftsdaten enthalten.
- Testinstanzen verwenden eigene Tenants und eigene Daten.

## Kafka und Worker

Kafka ist optional. Ist Kafka deaktiviert, speichert PostgreSQL autoritative
Änderungen, Audits und Outbox-Einträge weiterhin atomar. Bei späterer
Aktivierung arbeitet der Worker den Rückstau nach. Ein Brokerausfall darf die
fachliche Wahrheit nicht verlieren.

Vor dem Start mit aktiviertem Kafka müssen die drei getrennten Topics
`platform.domain-events.v1`, `platform.security-audit.v1` und
`platform.runtime-logs.v1` mit passenden ACLs, Retention und Replikation
betreiberseitig angelegt sein. WERK legt Topics nicht automatisch an.
`werkctl doctor` sowie der Worker prüfen Broker und Topic-Metadaten fail-fast.
Die API-Metrik `werk_kafka_runtime_logs_dropped_total` zählt lokal verworfene
oder fehlgeschlagene Logexporte ohne hochkardinale Labels. Domain-Event- und
Audit-Rückstände werden unabhängig aus PostgreSQL in der administrativen
Betriebsübersicht angezeigt.

Der Worker erneuert zusätzlich alle zehn Sekunden eine minimierte, expierende
Kafka-Beobachtung in PostgreSQL. Die Adminübersicht zeigt Kafka nur bei einer
frischen erfolgreichen Broker- und Topic-Prüfung als `ready`; Prüffehler werden
`degraded`, ein fehlender oder abgelaufener Worker wird `unknown`, und eine
deaktivierte Konfiguration bleibt `disabled`. Brokeradressen, Topicnamen,
Fehlertexte und Credentials werden nicht in der Beobachtung gespeichert. Der
vorgeschlagene generische Ausbau zu anklickbaren Statusintervallen steht in
[`ADR-043`](adr/ADR-043-generische-komponentenbeobachtung-und-statushistorie.md).

Für Produktion verlangt die Kafka-Konfiguration TLS sowie SASL oder ein
Client-Zertifikat. Ein einzelner Broker ist keine HA-Lösung. Der Vertrag steht
in [`ADR-020`](adr/ADR-020-kafka-event-audit-und-log-streaming.md).

## Optionaler Cache

`WERK_CACHE_URL` aktiviert einen austauschbaren Cacheadapter; der mitgelieferte
Adapter akzeptiert `valkey://`/`valkeys://` und `redis://`/`rediss://`. In
Produktion sind ausschließlich die TLS-Varianten mit Zertifikatsprüfung
zulässig. Ohne Konfiguration oder bei Adapterausfall bleibt die API korrekt und
verwendet PostgreSQL. Der erste Anwendungsfall puffert nur gehashte Hinweise auf
eindeutig ungültige Sessions für 15 Sekunden; vollständige Sessions, Rollen,
Widerrufe und Audits werden nicht in den Cache verlagert. Siehe
[`ADR-039`](adr/ADR-039-austauschbarer-cache-und-sessionhinweise.md).
Ein wegwerfbarer Adapter kann mit `WERK_TEST_CACHE_URL=... go test
./internal/platform/valkeycache` gegen `set`/`get`/`delete` geprüft werden. Die
Test-URL gehört nicht in die versionierte Konfiguration.

## Lokales Betriebswerkzeug

`werkctl doctor` inventarisiert die lokale Konfiguration, Listener-/TLS-Grenze,
fünf getrennte Datenbankrollen und – sofern aktiviert – Kafka. Die Ausgabe
verwendet stabile Check-IDs und `PASS`, `WARN` oder `FAIL`; `--json` stellt
denselben Vertrag als Schema `v1` bereit. `--config-only` öffnet keine
Netzwerkverbindung. Mit einer ausdrücklich gesetzten `--url` kann `doctor`
zusätzlich die öffentlichen API-Signale prüfen.

`werkctl status --url URL` liest ausschließlich `/meta`, `/health/live` und
`/health/ready`. Der Befehl behauptet weder Worker-, Kafka-, Migrations-,
Backup-, Host- noch HA-Zustand und erzeugt keine Admin-Sitzung. HTTP ist nur
für Loopback-Ziele zulässig; HTTPS behält die normale Zertifikatsprüfung und
folgt keinen Redirects.

`werkctl migrate` bleibt ein ausdrücklich verändernder, aufwärtsgerichteter
Command mit eigener Migrator-Rolle. Start, Stop, Neustart, Update und
Hybrid-Cloud-Pairing sind noch keine ausführbaren Befehle. Ihre typisierte
Runnergrenze ist in
[`ADR-031`](adr/ADR-031-werkctl-betriebsbefehle-und-runnergrenze.md)
vorgeschlagen; weder `doctor` noch `status` führen Reparaturen automatisch aus.

## Administrative Betriebsübersicht

Das Admin-Portal liest über `/admin/v1/operations/summary` eine
installationsweite Betriebsübersicht. Der Abruf verlangt eine gültige
interaktive Admin-Sitzung mit Admin-Audience, ohne Tenant und mit bekannter
Single- oder Multi-Factor-Assurance sowie die Berechtigung
`core.platform.operations.read`; `unknown` wird fail-closed abgewiesen. Der
Abruf wird selbst als Security-Ereignis auditiert. Die Antwort ist nicht
cachebar. Der allgemeine Assurance-Vertrag steht in
[`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md).

Die Übersicht enthält ausschließlich bereinigte Zustände:

- API- und PostgreSQL-Erreichbarkeit aus dem erfolgreichen autorisierten Abruf,
- den aktiven Worker-Heartbeat mit Build- und begrenzten Zeitkoordinaten,
- aggregierte Zustände und Anzahlen der Domain-Outbox und des
  Security-Audit-Exports einschließlich des Alters des ältesten offenen
  Eintrags,
- Anzahl und neuesten Stand angewendeter Migrationen,
- nicht geheime Installationskoordinaten wie Build, API-Version und -Laufzeit,
  Laufzeitumgebung, TLS- und Kafka-/Transportstatus.

Der Worker erneuert seinen Heartbeat alle zehn Sekunden mit einer regulären TTL
von 30 Sekunden. Er darf den Eintrag ausschließlich über die eng begrenzten
`SECURITY DEFINER`-Funktionen zum Erneuern und Entfernen verändern; direkte
`SELECT`-, `INSERT`-, `UPDATE`- oder `DELETE`-Rechte auf der zugrunde liegenden
Tabelle besitzt die Worker-Runtime nicht. PostgreSQL stellt alle Zeitpunkte
bereit und erzwingt serverseitig eine TTL von höchstens zehn Minuten. Der
Worker verwendet eine zufällige Lauf-ID und entfernt den eigenen Eintrag bei
geordnetem Herunterfahren ebenfalls über den Funktionsvertrag. Die Build-Kennung
ist getrimmt und auf 128 Zeichen begrenzt.

Hostname, PID, Adresse, Datenbank-URL und sonstige Infrastrukturkoordinaten
werden weder im Heartbeat gespeichert noch an das Admin-Portal ausgegeben. Ein
fehlender oder abgelaufener Eintrag wird nicht aus einem erfolgreichen
API-Healthcheck hergeleitet, sondern als unbekannt beziehungsweise nicht aktiv
bewertet.

Die Admin-Datenbankrolle darf nur die bereinigte Projektion lesen. Sie erhält
keinen Zugriff auf Heartbeat-Zeilen, Queue-Payloads, Fehlertexte oder Leases.
Ist Kafka deaktiviert, bleiben die autoritativen PostgreSQL-Rückstände sichtbar,
während der externe Zustellpfad ausdrücklich als deaktiviert erscheint.

Die Übersicht ist bewusst keine Ausführungsoberfläche. Update, Neustart,
Rollback, Logs und Hostdiagnose bleiben deaktiviert, weil der API-Prozess weder
Docker-Socket noch Shell-, `systemd`- oder Paketmanagerrechte besitzt. Dafür ist
noch kein Ops-Executor angebunden. Ein späterer Ausführungsweg benötigt einen
getrennten minimal privilegierten Agenten, erneute starke Authentisierung,
Autorisierung, einen versionierten und idempotenten Command-Vertrag, Audit,
signierte Release-Metadaten und eine geprüfte Rollbackstrategie. Die verbindliche
Grenze steht in
[`ADR-029`](adr/ADR-029-administrative-beobachtung-und-ausfuehrungsgrenze.md).

## Backup und Wiederherstellung

`deploy/backup/werk-backup` nutzt die getrennte Rolle `werk_backup`, streamt
`pg_dump` direkt durch `age` und schreibt kein unverschlüsseltes
Zwischenartefakt. Wiederherstellungen sind ausschließlich für eine frische,
isolierte Zieldatenbank vorgesehen und verlangen eine explizite Bestätigung.

Der vollständige Restore-Nachweis prüft Daten, Migrationschecksummen, Besitzer,
Grants, RLS und Tenant-Isolation gegen eine ausdrücklich angegebene
Wegwerf-Datenbank. Er darf nie implizit eine lokale oder produktive Datenbank
löschen. WAL/PITR und ein späterer Object Store benötigen eigene
Wiederherstellungsverträge.

## Release und Aktualisierung

SemVer-Tags erzeugen native Linux-Pakete und Quellarchive mit Prüfsummen und
Herkunftsnachweisen. Die Pipeline veröffentlicht Artefakte, führt aber kein
Deployment aus. Der Release-Vertrag steht in
[`ADR-019`](adr/ADR-019-release-kanal-und-softwarelieferkette.md).

## Abgrenzung zum HA-Profil

Das Startprofil besitzt genau eine autoritative PostgreSQL-Datenbank. Mehrere
API- oder Worker-Prozesse an dieser Datenbank teilen dieselbe Wahrheit. Eine
zweite Datenbankkopie benötigt vor automatischem Failover einen unabhängigen
Platform Witness, exklusive Lease, monotone Generation, bestätigte
Replikationsschranke und wirksames Fencing. Ohne diese Nachweise bleibt der
Single Host bewusst einfach.
