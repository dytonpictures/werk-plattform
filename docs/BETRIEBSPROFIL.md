# WERK – Betriebsprofil 1: Single Host

**Status:** Startprofil für interne Produktiv- und Testinstanzen

## Ziel

Eine WERK-Installation wird zunächst auf einem einzelnen, vom Unternehmen
kontrollierten Linux-Server über Docker Compose betrieben. Dashboard, API,
Worker und Infrastruktur laufen in getrennten Containern.

## Dienstgrenzen

- Nur `edge` veröffentlicht einen Port nach außen; Datenbank und Valkey bleiben
  im Compose-Netz.
- `dashboard` kommuniziert ausschließlich über die öffentliche Business-API.
- `api` und `worker` verwenden PostgreSQL für fachliche Daten und Outbox.
- `migrate` endet nach erfolgreicher Migration und wird nicht dauerhaft betrieben.
- `database-roles` gleicht vor Migrationen die lokalen PostgreSQL-Rollen ab und
  endet danach. API und Worker kennen das Bootstrap-Credential nicht.
- Valkey besitzt keine alleinigen fachlichen Daten.

## Mindestbetrieb

- TLS, externe Backups, sichere Secrets und ein nicht triviales
  `POSTGRES_PASSWORD` sind vor nicht-lokaler Nutzung Pflicht. Dasselbe gilt für
  die getrennten Migrator-, Work-, Admin-, Service- und Worker-Passwörter.
- Vor jedem risikoreichen Update wird ein getestetes Datenbank- und
  Dokumentenbackup erzeugt.
- Der Betreiber prüft `/health/live` und `/health/ready`; Logs werden zentral
  gesammelt, ohne Geheimnisse oder unnötige Geschäftsdaten aufzunehmen.
- Testinstanzen erhalten einen eigenen Tenant und eigene Daten. Produktivdaten
  werden nur anonymisiert oder über einen ausdrücklich freigegebenen Export
  übertragen.
- Runtime-Dienste verbinden sich ausschließlich als Non-Owner-Rollen ohne
  `SUPERUSER` oder `BYPASSRLS`. Der Bootstrap-Superuser ist kein
  Anwendungscredential.

## Backup und Wiederherstellung

- Logische PostgreSQL-Backups laufen ausschließlich über `werk_backup`, das nur
  explizit zur nicht anmeldbaren Lesefähigkeit `werk_backup_reader` wechseln
  darf. Nur diese Lesefähigkeit besitzt den für einen vollständigen Dump
  erforderlichen RLS-Bypass; sie besitzt keine Schreib- oder DDL-Rechte.
- `pg_dump` wird direkt in `age` gestreamt. Es gibt kein unverschlüsseltes
  Zwischenartefakt. Der Backup-Container erhält nur öffentliche Empfänger.
- Ciphertext und SHA-256-Prüfsumme werden gemeinsam auf ein vom
  PostgreSQL-Datenvolume getrenntes Medium kopiert. Mindestens eine private
  Recovery-Identität wird getrennt und off-site verwahrt.
- Wiederherstellungen erfolgen nur in eine frische, isolierte Zieldatenbank. Der
  Restore verlangt ein Bestätigungswort, prüft die leere Datenbank und läuft als
  einzelne Transaktion über `werk_migrator` und `werk_owner`.
- `make restore-test` prüft falsche Schlüssel, Datenvollständigkeit,
  Migration-Checksummen, Objektbesitz, Grants, RLS und Tenant-Isolation in
  Wegwerf-Volumes. Ein erfolgreicher Test ersetzt nicht den regelmäßigen
  betrieblichen Restore-Drill mit den tatsächlich verwahrten Artefakten.

Das aktuelle logische Backup deckt PostgreSQL ab. WAL/PITR sowie ein konsistentes
Backup des späteren Object Storage werden ergänzt, sobald die dafür notwendige
Infrastruktur Teil des Betriebsprofils wird.

## Release und Aktualisierung

SemVer-Tags auf einem in `Canary` enthaltenen Commit erzeugen geprüfte
GitHub-Release-Archive und getrennte GHCR-Images. Produktive Aktualisierungen
referenzieren eine konkrete Version über `WERK_BUILD_VERSION` und verwenden
`compose.release.yaml` als letzten Overlay. Operations-Images werden zusätzlich
mit `compose.release.ops.yaml` eingebunden. `latest` ist kein zulässiger
Produktions-Pin, auch wenn die Pipeline ihn bei stabilen Releases als
Komfortalias veröffentlicht.

Vor einer Aktualisierung werden Datenbackup, Wiederherstellbarkeit,
Migrationspfad und Zielversion geprüft. Die Pipeline veröffentlicht und
attestiert Artefakte, deployt aber keine Instanz. Promotion, Wartungsfenster und
Rollback bleiben Betreiberentscheidungen. Der Vertrag steht in
[`ADR-019`](adr/ADR-019-release-kanal-und-softwarelieferkette.md); Änderungen
vorbehalten.

## Abgrenzung zum späteren HA-Profil

Dieses Profil besitzt genau eine Identity-Autorität und keinen Identity
Witness. Zusätzliche API- oder Worker-Prozesse an derselben PostgreSQL-Datenbank
ändern daran nichts; sie teilen dieselbe fachliche Wahrheit.

Eine zweite Instanz mit eigener Datenbankkopie ist ein eigenes
Active/Passive-Betriebsprofil. Automatischer Failover erfordert dort gemäß
[`ADR-015`](adr/ADR-015-identity-authority-witness-und-failover.md) einen
unabhängigen QDevice-artigen Witness, eine exklusive Lease, eine monotone
Autoritätsgeneration, eine bestätigte Replikationsschranke und extern wirksames
Fencing. `/health/live` und `/health/ready` bleiben Diagnose- und
Orchestrierungssignale; sie vergeben keine Schreibhoheit.

Ohne erreichbaren Witness darf eine Reserve nicht automatisch zur
Identity-Hauptinstanz werden. Das Single-Host-Profil enthält deshalb noch keine
ungenutzte Quorum-, Replikations- oder Promotion-Infrastruktur.

## Ausbauregel

Ein Wechsel zu mehreren Hosts, HA, Kubernetes oder einem getrennten Object Store
erfolgt nur mit ADR, Last-/Wiederherstellungstest und dokumentiertem
Migrationspfad. Für Identity-HA gelten zusätzlich Netztrennungs-,
Replikations-, Fencing-, Schlüsselrotations- und Rückkehrtests. Die fachlichen
APIs und Datenhoheiten bleiben unverändert.
