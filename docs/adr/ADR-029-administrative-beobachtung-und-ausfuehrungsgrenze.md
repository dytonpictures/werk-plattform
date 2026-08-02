# ADR-029 – Administrative Beobachtung und Ausführungsgrenze

**Status:** Angenommen  
**Datum:** 2026-07-28

## Kontext

Betreiber benötigen im Admin-Portal eine schnelle, verlässliche Übersicht über
API, Worker, asynchrone Zustellung und Migrationen. Dieselbe Oberfläche direkt
mit Docker, `systemd`, einer Shell oder dem Paketmanager zu verbinden, würde die
Admin-API jedoch zu einem privilegierten Host-Zugang machen. Eine kompromittierte
Browser-Sitzung könnte dann die Prozess- und Datenbankgrenzen des Plattformkerns
umgehen.

Health-Endpunkte beantworten außerdem eine andere Frage als eine
administrative Betriebsübersicht: Liveness und Readiness sind knappe
maschinenlesbare Signale. Sie belegen weder einen aktiven Worker noch einen
funktionsfähigen Updatekanal.

## Entscheidung

- Der Plattformkern stellt eine durch gültige Admin-Session und Berechtigung
  geschützte, installationsweite Betriebsübersicht bereit. Die Session darf
  gemäß
  [`ADR-032`](ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md)
  bekannte Single- oder Multi-Factor-Assurance tragen; `unknown` wird
  fail-closed abgewiesen.
- Die Übersicht ist ausschließlich beobachtend. Sie aggregiert nur
  nicht-geheime Zustände aus PostgreSQL: Worker-Heartbeat, Zustellstatus der
  Domain-Outbox und des Security-Audit-Exports sowie den angewendeten
  Migrationsstand.
- Heartbeats enthalten nur logische Komponente, Build-Version und begrenzte
  Zeitstempel. Hostname, PID, Adresse, Datenbank-URL und sonstige
  Infrastrukturkoordinaten werden weder gespeichert noch ausgegeben.
- Der Abruf der Übersicht wird als Security-Ereignis auditiert. Die Admin-Rolle
  erhält nur Zugriff auf die bereinigte Projektion, nicht auf Queue-Payloads,
  Fehlertexte, Leases oder Prozesszeilen.
- Ein fehlender oder abgelaufener Heartbeat wird als unbekannt beziehungsweise
  nicht aktiv dargestellt. Er wird nicht aus einem erfolgreichen API-Readiness-
  Check abgeleitet.
- Update, Neustart, Rollback und Host-Diagnose sind kein Bestandteil der API
  oder dieses ersten Dashboards. Insbesondere erhält der API-Prozess keinen
  Docker-Socket, keine Shell und keine `systemd`-/Paketmanager-Berechtigung.
- Eine spätere Ausführung benötigt einen eigenen minimal privilegierten
  Ops-Agenten oder Orchestrator, einen versionierten Command-Vertrag,
  aktions- und ressourcengebundene erneute starke Authentisierung,
  Autorisierung, Idempotenz, Audit, signierte Release-Metadaten und eine
  belastbare Rollbackstrategie.

## Folgen

Das Admin-Portal kann den tatsächlich belegten Core-Zustand zeigen, ohne eine
neue Host-Kontrollfläche zu öffnen. „Neustart“ oder „Update“ dürfen erst als
verfügbar erscheinen, wenn der getrennte Ausführungsvertrag implementiert und
abgenommen ist. Bis dahin erklärt die Oberfläche diese Grenze ausdrücklich.

Die nativen Prozessgrenzen aus
[`ADR-002`](ADR-002-container-und-betriebsgrenzen.md), die Lieferkette aus
[`ADR-019`](ADR-019-release-kanal-und-softwarelieferkette.md) und der First-Run-
Vertrag aus [`ADR-028`](ADR-028-native-linux-distribution-und-first-run.md)
bleiben unverändert. Die vorgeschlagene typisierte lokale CLI-/Runnergrenze
wird in
[`ADR-031`](ADR-031-werkctl-betriebsbefehle-und-runnergrenze.md)
konkretisiert.
