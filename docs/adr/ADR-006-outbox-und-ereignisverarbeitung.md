# ADR-006 – Versionierte Ereignisse und Transactional Outbox

**Status:** Angenommen  
**Datum:** 2026-07-19

## Kontext

WERK muss Änderungen zwischen Core, Capabilities und Fachanwendungen
zuverlässig weitergeben. Flüchtige Echtzeitnachrichten oder eine externe Queue
dürfen dabei niemals die fachliche Wahrheit ersetzen. Eine Änderung darf nicht
gespeichert werden, ohne dass ihre verbindliche Zustellung nachvollziehbar
geplant werden kann.

## Entscheidung

- Ein besitzendes Modul speichert fachliche Änderung und Outbox-Eintrag in
  derselben PostgreSQL-Transaktion.
- Ein Domain-Event ist eine unveränderliche Tatsache, kein auszuführender
  Befehl. Es besitzt Event-ID, Tenant, Typ, Schema-Version, Producer, Subject,
  Zeit, Correlation-/Causation-ID und validiertes Payload.
- Der Worker liest fällige Outbox-Einträge aus PostgreSQL und stellt sie an
  registrierte Consumer zu. Zustellung ist mindestens einmalig; Consumer müssen
  anhand von Event-ID und Subscriber idempotent sein.
- Retries verwenden Backoff, Zustellversuche werden protokolliert und endgültige
  Fehler landen in einer nachvollziehbaren Dead-Letter-Ansicht.
- Valkey Streams oder eine spätere Queue-Implementierung dürfen als beschleunigte
  Zustell- und Claim-Infrastruktur dienen. PostgreSQL bleibt Quelle für Outbox,
  Ereignisse, Zustellstatus und Audit.
- Valkey Pub/Sub und SSE/WebSocket sind ausschließlich flüchtige Hinweise. Ein
  Client lädt autorisierte Daten nach einer Nachricht erneut über die Business-
  API.

## Grenzen

Fachanwendungen schreiben nicht in fremde Tabellen und rufen einander nicht über
eine synchrone Kette für eine gemeinsame Transaktion auf. Ein Consumer darf ein
weiteres Modul nicht stillschweigend als Benutzer vertreten; jede Folgeaktion
läuft über dessen öffentliche Commands, Policy und Audit.

Neue Event-Typen werden registriert und additiv versioniert. Semantisch
inkompatible Änderungen erhalten eine neue Hauptversion und einen dokumentierten
Migrationspfad.

## Nachweis

Der spätere Ereignistest muss Commit/Outbox-Atomizität, Wiederanlauf, doppelte
Zustellung, Backoff, Dead Letter, Tenant-Grenze und Verhalten bei Valkey-Ausfall
prüfen. Ein Valkey-Ausfall darf keine fachliche Änderung oder Audit-Aufzeichnung
verlieren.

## Implementierungsstand 2026-07-20

Die PostgreSQL-Grundlage, versionierte Go-Verträge, atomare Enqueue-Funktion,
Leasing mit `SKIP LOCKED`, Partitionsreihenfolge, begrenzter Worker-Pool,
idempotente Consumer-Receipts, exponentielle Retries und Dead-Letter-Status sind
implementiert. Der Worker verwendet ausschließlich seine Non-Owner-Rolle und
öffnet fachliche Verarbeitung als explizite Tenant-Transaktion.

Noch offen sind registrierte fachliche Event-Typen und Consumer, ein
administrativer Dead-Letter-/Replay-Vertrag, Heartbeats für sehr lange Handler,
Metriken sowie der optionale Valkey-Queue-Adapter. Die PostgreSQL-Verarbeitung
funktioniert unabhängig von Valkey.
