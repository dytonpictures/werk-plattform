# ADR-002: Native Prozess- und Betriebsgrenzen

**Status:** angenommen · **Datum:** 2026-07-19

## Entscheidung

Die erste Betriebsstufe nutzt native Prozesse. Der Quellstart und das
Debian-/Ubuntu-Paket für `amd64` verwenden dieselben Grenzen:

| Dienst | Aufgabe |
|---|---|
| `api` | direkter TLS-Einstieg, eingebettete Weboberfläche, versionierte Business-API und synchrone Commands/Queries |
| `worker` | asynchrone Outbox-, Job- und Scheduler-Verarbeitung |
| `migrate` | einmalige, versionierte Datenbankmigrationen |
| `postgres` | einziges fachliches System of Record |
| `valkey` | austauschbarer Cache-, Session-, Live- und Queue-Dienst |

Die erste Installation darf auf einem einzelnen Server laufen. Getrennte Prozesse
schaffen dennoch klare Skalierungs-, Sicherheits- und Updategrenzen. Ein
HA-/Mehrserverprofil wird erst nach gemessener Notwendigkeit ergänzt.

## Folgen

- API und Worker können unabhängig gestartet und skaliert werden; das Dashboard
  wird mit der API ausgeliefert.
- Der Worker verwendet denselben Go-Codebestand wie die API, aber keinen
  gemeinsamen Prozess.
- Migration und Laufzeit verwenden keine Datenbank-Superuser-Zugänge.
- Valkey-Ausfall darf zu eingeschränkter Echtzeit oder Leistung führen, aber nie
  zum Verlust fachlicher Daten oder verbindlicher Entscheidungen.

Den nativen Installations-, First-Run- und systemd-Vertrag beschreibt
[`ADR-028`](ADR-028-native-linux-distribution-und-first-run.md); Änderungen
vorbehalten.
