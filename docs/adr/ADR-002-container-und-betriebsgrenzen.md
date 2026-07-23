# ADR-002: Container- und Betriebsgrenzen

**Status:** angenommen · **Datum:** 2026-07-19

## Entscheidung

Die erste Betriebsstufe nutzt Docker Compose und getrennte Images:

| Dienst | Aufgabe |
|---|---|
| `edge` | TLS/Reverse Proxy und gemeinsamer Einstiegspunkt |
| `dashboard` | Benutzeroberfläche; kein direkter Datenbankzugriff |
| `api` | versionierte Business-API und synchrone Commands/Queries |
| `worker` | asynchrone Outbox-, Job- und Scheduler-Verarbeitung |
| `migrate` | einmalige, versionierte Datenbankmigrationen |
| `postgres` | einziges fachliches System of Record |
| `valkey` | austauschbarer Cache-, Session-, Live- und Queue-Dienst |

Die erste Installation darf auf einem einzelnen Server laufen. Getrennte Images
schaffen dennoch klare Skalierungs-, Sicherheits- und Updategrenzen. Ein
HA-/Mehrserverprofil wird erst nach gemessener Notwendigkeit ergänzt.

## Folgen

- Dashboard, API und Worker können unabhängig ausgeliefert und skaliert werden.
- Der Worker verwendet denselben Go-Codebestand wie die API, aber keinen
  gemeinsamen Prozess.
- Migrations- und Applikationscontainer verwenden keine Datenbank-Superuser-
  Zugänge im späteren Produktionsprofil.
- Valkey-Ausfall darf zu eingeschränkter Echtzeit oder Leistung führen, aber nie
  zum Verlust fachlicher Daten oder verbindlicher Entscheidungen.
