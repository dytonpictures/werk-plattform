# WERK – Agentenleitfaden

## Projektkontext

WERK ist ein modulares, selbst hostbares Unternehmensbetriebssystem. Der stabile
Core stellt Identität, Organisation, Rechte, Workflows, Aufgaben, Dokumente,
Suche, Audit, Ereignisse, Konfiguration und Betrieb bereit. Fachanwendungen
besitzen ihre Fachlogik und Fachdaten, verwenden aber keine parallelen
Core-Strukturen.

Die verbindlichen Architekturquellen sind:

- `docs/vision.md` – Zielbild und technische Leitplanken
- `docs/DATENMODELL.md` – Datenhoheit, Sicherheits- und Erweiterungsmodell
- `docs/ROADMAP.md` – Reihenfolge der Umsetzung

Bei einem Konflikt gilt: Sicherheitsinvariante und Datenmodell vor Roadmap;
Roadmap vor einer ad-hoc fachlichen Abkürzung.

## Nicht verhandelbare Regeln

- PostgreSQL ist die fachliche Wahrheit. Valkey ist nur austauschbare
  Infrastruktur für Cache, Sessions, Echtzeit und gegebenenfalls Queues.
- Jede mandantenbezogene Operation benötigt expliziten Tenant-Kontext und eine
  serverseitige Berechtigungsprüfung.
- `work`, `admin` und `service` sind getrennte Kontoarten mit getrennten APIs,
  Sessions und Berechtigungsbereichen. Ein Admin ist kein User mit zusätzlichen
  Rollen.
- Fachmodule schreiben nie direkt in Tabellen anderer Fachmodule. Sie verwenden
  versionierte Core-Verträge oder Ereignisse.
- Neue Fachobjekte registrieren Ressourcen, Berechtigungen und Ereignistypen.
  Sie duplizieren weder Aufgaben, Dokumente, Audit noch Identität.
- KI und Plugins erhalten keine direkten Datenbankzugriffe, keine implizite
  Benutzervertretung und keine Umgehung von Policy, Audit oder Freigaben.
- Verbindliche Änderungen und ihre Outbox-Einträge werden atomar in PostgreSQL
  gespeichert. Flüchtige Echtzeitnachrichten sind nie die einzige Wahrheit.

## Arbeitsweise

- Vor Änderungen die drei Architekturquellen und betroffene lokale Anweisungen
  lesen.
- Kleine, zusammenhängende Änderungen bevorzugen; bestehende Nutzeränderungen
  nicht überschreiben.
- Öffentliche APIs, Ereignisse, Ressourcen- und Berechtigungstypen versionieren
  und dokumentieren.
- Jede Änderung angemessen prüfen: Unit-/Integrationstests für Code,
  Migrationsprüfung für Datenbankänderungen, Strukturprüfung für Dokumentation.
- Ausgeführte Prüfungen und bewusst nicht ausgeführte Prüfungen im Abschluss
  benennen.
- Wenn ein Architekturentscheid langfristig schwer rückgängig zu machen ist,
  zuerst ein ADR vorschlagen oder anlegen.

## Delegation

Subagenten nur einsetzen, wenn der Nutzer ausdrücklich um Delegation oder
parallele Arbeit bittet oder eine konkrete Projektanweisung es verlangt.
Gleichzeitige schreibende Agenten nur bei klar getrennten Dateien einsetzen.

## Aktueller Projektstand

Das Repository enthält das Plattformfundament. Prüfungen für Go-Änderungen:

```bash
gofmt -w cmd internal
go test ./...
docker compose config --quiet
```

Änderungen an Rollen, Migrationen, RLS oder tenantgebundenem Datenzugriff müssen
zusätzlich `make integration-test` bestehen. Dieser Test verwendet ein eigenes
Docker-Projekt und löscht ausschließlich dessen Wegwerf-Volume.

Der vollständige Container-Stack startet mit `docker compose up --build -d`.
Für native Entwicklung startet `make dev` nur PostgreSQL und Valkey im isolierten
Compose-Projekt; Dashboard, API, Worker und Migration laufen als Host-Prozesse.
Einzelne Prozesse lassen sich über `make dev-api`, `make dev-worker` und
`make dev-dashboard` in getrennten Terminals starten. Datenbankmigrationen liegen
als eingebettete SQL-Dateien unter `internal/platform/migrate/migrations`;
angewendete Migrationen werden nie nachträglich verändert.
