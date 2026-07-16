# ADR-0015: Explizite Migrationen und System-Seeds

- **Status:** Accepted
- **Kontext:** Datenbankschema und Systemrollen müssen reproduzierbar sein.
- **Entscheidung:** Versionierte Migrationen ändern das Schema; idempotente Seeds erzeugen Systemdaten.
- **Begründung:** Nachvollziehbare Updates ohne Schema-Drift.
- **Alternativen:** Automatische ORM-Synchronisation, manuelle SQL-Änderungen.
- **Positive Folgen:** Prüfbare Deployments und Wiederherstellungen.
- **Negative Folgen:** Migrationen benötigen Disziplin und Tests.
- **Sicherheitsauswirkungen:** Migrationen laufen mit begrenzten, kontrollierten Rechten.
- **Überprüfung:** Bei Änderungen am Datenbank-Deployment.
