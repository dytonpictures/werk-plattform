# WERK – Betriebsprofil 1: Single Host

**Status:** Startprofil für interne Produktiv- und Testinstanzen

## Ziel

Eine WERK-Installation wird zunächst auf einem einzelnen, vom Unternehmen
kontrollierten Linux-Server über Docker Compose betrieben. Dashboard, API,
Worker und Infrastruktur laufen in getrennten Containern.

## Dienstgrenzen

- Nur `edge` veröffentlicht im vollständigen Stack einen Port nach außen;
  Datenbank, Valkey und Kafka bleiben im Compose-Netz.
- `dashboard` kommuniziert ausschließlich über die öffentliche Business-API.
- `api` und `worker` verwenden PostgreSQL für fachliche Daten und Outbox.
- `kafka` läuft als einzelner persistenter KRaft-Broker/Controller;
  `kafka-init` legt die getrennten Domain-, Audit- und Log-Topics idempotent an.
- `migrate` endet nach erfolgreicher Migration und wird nicht dauerhaft betrieben.
- `database-roles` gleicht vor Migrationen die lokalen PostgreSQL-Rollen ab und
  endet danach. API und Worker kennen das Bootstrap-Credential nicht.
- Valkey besitzt keine alleinigen fachlichen Daten.
- Kafka verteilt Ereignisse, minimierte Security-Audits und Betriebslogs, ist
  aber weder fachliche Wahrheit noch revisionssicheres Langzeitarchiv.

## Mindestbetrieb

- Der native API-Server verlangt in Produktion `tls` oder `mtls`. Zertifikat,
  privater Schlüssel und gegebenenfalls Client-CA werden als sichere Dateien
  bereitgestellt und bei Rotation fail-closed neu geladen. Ein vorgeschalteter
  Proxy ersetzt diesen Softwarevertrag nicht; weitergereichte HTTPS-Information
  wird nur aus ausdrücklich vertrauten Proxy-Netzen akzeptiert.
- PostgreSQL-Verbindungen verwenden in Produktion `sslmode=verify-full`; TLS,
  externe Backups, sichere Secrets und ein nicht triviales
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
- Bei aktiviertem Kafka verlangt die Produktionskonfiguration TLS sowie SASL
  oder ein Client-Zertifikat. Das mitgelieferte Plaintext-Profil ist auf das
  interne lokale Compose-Netz begrenzt und kein fertiges Produktions-
  Sicherheitsprofil.

Der native Transportvertrag und seine bewusste Trennung von Policy, Lease und
Fencing sind in
[`ADR-023`](adr/ADR-023-native-server-tls-und-transportidentitaet.md)
festgelegt; Änderungen vorbehalten.

## Kafka und Streamingbetrieb

Der gepinnte Broker verwendet ein eigenes persistentes Volume. Domain-Events
werden sieben Tage, minimierte Auditexporte dreißig Tage und Laufzeitlogs sieben
Tage im lokalen Startprofil gehalten. Diese Werte begrenzen ausschließlich den
Transportpuffer und dürfen nach Datenklassifikation und Betreiberpflichten
angepasst werden. `cleanup.policy=delete` verhindert eine versehentliche
Kompaktion des Auditverlaufs.

Ein Broker-Ausfall macht die Business-API nicht automatisch fachlich
unbrauchbar: PostgreSQL nimmt autoritative Änderungen, Audits und Outbox-
Einträge weiter atomar an. Der Worker baut Rückstau auf und verarbeitet ihn nach
Wiederkehr. Betreiber überwachen Broker-Health, nicht abgeschlossene
`outbox_events`, `security_audit_export_queue`, Retry-/Dead-Zustände sowie die
Anzahl verworfener, nicht revisionsrelevanter Laufzeitlogs.

Der einzelne Broker ist keine HA-Lösung. Mehrere Broker/Controller auf
getrennten Hosts, Replikationsfaktoren größer eins, gesicherte Listener,
ACL-Verwaltung, Kapazitäts- und Wiederherstellungstests werden in einem späteren
Clusterprofil festgelegt. Der verbindliche Vertrag steht in
[`ADR-020`](adr/ADR-020-kafka-event-audit-und-log-streaming.md); Änderungen
vorbehalten.

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

Das tenantgesicherte Dokument-/Blob-Metadatenschema darf vorher als inaktives
Fundament vorhanden sein. Ein produktiver Upload bleibt gesperrt, bis der
S3-kompatible Provider, ein verschlüsseltes Objektmanifest, ein definierter
Backup-Cut, Orphan-/Missing-Object-Reconciliation und ein gemeinsamer
PostgreSQL-/Object-Store-Restore-Test geliefert sind. Der Storage-Dienst bleibt
im internen Datennetz und erhält keine rohe öffentliche `/service`-Route. Der
Vertrag steht in
[`ADR-021`](adr/ADR-021-interner-dokument-blob-und-transfervertrag.md);
Änderungen vorbehalten.

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

Dieses Profil besitzt genau eine Identity-Autorität und keinen Platform Witness.
Zusätzliche API- oder Worker-Prozesse an derselben PostgreSQL-Datenbank
ändern daran nichts; sie teilen dieselbe fachliche Wahrheit.

Eine zweite Instanz mit eigener Datenbankkopie ist ein eigenes
Active/Passive-Betriebsprofil. Automatischer Failover erfordert dort gemäß
[`ADR-015`](adr/ADR-015-identity-authority-witness-und-failover.md) und
[`ADR-022`](adr/ADR-022-deploymentprofile-und-platform-witness.md) einen
unabhängigen QDevice-artigen Platform Witness mit `identity-control`, eine
exklusive Lease, eine monotone
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
