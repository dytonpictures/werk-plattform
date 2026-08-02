# ADR-040 – Identity-Retention und Session-Anonymisierung

**Status:** Vorgeschlagen  
**Datum:** 2026-08-01

## Kontext

PostgreSQL ist die autoritative Sessionquelle. Abgelaufene und widerrufene
Sessions werden bei jeder Authentifizierung korrekt ausgeschlossen, bleiben
aber erhalten. Security-Audit-Ereignisse referenzieren Session-IDs über einen
Fremdschlüssel. Ein direktes Löschen würde deshalb Auditbezüge verletzen; ein
Cleanup im Login-Request würde außerdem dessen Latenz und Schreiblast erhöhen.

Der allgemeine Worker verwendet bewusst nur `WORKER_DATABASE_URL` und besitzt
keinen direkten Zugriff auf Identity-Tabellen. Ihm zusätzliche Identity-Rechte
zu geben würde die bestehende Prozess- und Credentialgrenze aufweichen.

## Vorschlag

1. Ein eigener Identity-Maintenance-Runner erhält eine eng begrenzte
   Datenbankrolle und eine eigene `IDENTITY_MAINTENANCE_DATABASE_URL`.
2. Eine additive Migration macht `sessions.token_hash` nach endgültigem Ablauf
   und einer festgelegten Sicherheitsfrist löschbar und ergänzt einen
   expliziten Anonymisierungszeitpunkt. Aktive Sessions behalten weiterhin
   zwingend genau einen eindeutigen Token-Hash.
3. Der Runner anonymisiert in kleinen, geordneten Batches mit `FOR UPDATE SKIP
   LOCKED`. Session-ID, Account-/Tenantbezug, Zeitkoordinaten, Assurance und
   Audit-Fremdschlüssel bleiben erhalten, solange deren Aufbewahrungsvertrag es
   verlangt.
4. Verbrauchte oder abgelaufene MFA-, WebAuthn- und OIDC-Ceremonies werden über
   eigene, typabhängige Fristen ebenfalls begrenzt in Batches entfernt.
5. Security-Audits werden nie implizit durch Session-Maintenance gelöscht. Ihre
   Retention, Archivierung und gegebenenfalls Pseudonymisierung erhalten einen
   getrennten Compliance-Vertrag.
6. Maintenance ist idempotent, beobachtbar und bei Ausfall folgenlos für
   Anmeldung, Logout und Autorisierung. Sie verwendet weder Cache noch Valkey
   als Fortschrittswahrheit.

## Vor Umsetzung zu entscheiden

- verbindliche Fristen je Datentyp und Rechts-/Complianceprofil,
- ob Account- und Tenantbezüge nach Audit-Retention erhalten bleiben dürfen,
- Backup-/Restore- und Legal-Hold-Verhalten,
- maximale Batchgröße, Intervall, Lock- und Statement-Timeouts,
- Metriken, Adminstatus und Auditierung des Maintenance-Laufs,
- Paketierung als eigener Prozess oder eng begrenzter Modus eines späteren
  Identity-Runners.

## Nicht entschieden

Dieses ADR autorisiert noch keine Migration, neue Datenbankrolle oder Löschung.
Insbesondere erhält der vorhandene allgemeine Worker keine Identity-Rechte und
der API-Requestpfad führt keine opportunistische Sessionbereinigung aus.
