# ADR-010: Core Identity als interne Identitätsquelle

## Status

Angenommen

## Kontext

WERK benötigt eine verlässliche Identitäts- und Zugriffsschicht für Work-,
Admin- und Service-Konten. Externe Provider können in verschiedenen
Installationen fehlen, unterschiedliche Aussagen über Kontoarten liefern oder
den Tenant-Kontext nicht kennen. Diese Sicherheitsentscheidungen müssen deshalb
innerhalb des Plattformkerns bleiben.

## Entscheidung

`Core Identity` ist die interne Identitäts- und Zugriffsschicht von WERK und
kann selbst als Identity Provider arbeiten. Sie besitzt Konten, Credentials,
Sessions, Kontoarten, Audiences, Tenant-Zuordnung, Berechtigungsentscheidungen
und Audit-Bezüge.

Externe Identity Provider werden ausschließlich als optionale Adapter
angebunden. Ein Adapter darf eine Identität bestätigen oder ablehnen, aber
nicht selbst Kontoart, Tenant, Session-Audience oder WERK-Berechtigungen
festlegen. Jede externe Identität wird vor der Session-Ausstellung gegen ein
serverseitiges WERK-Konto aufgelöst.

## Konsequenzen

- Eine Single-Host-Installation funktioniert ohne externe Identitätsdienste.
- `work`, `admin` und `service` bleiben unabhängig vom Anmeldeverfahren getrennt.
- Provider-Adapter können später ergänzt oder ausgetauscht werden.
- Login- und Session-Verträge müssen providerunabhängig bleiben.
- Credential- und Session-Daten bleiben in PostgreSQL die fachliche Wahrheit.
