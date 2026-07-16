# Aufgaben

## REVIEW

### WERK-001 – Technisches Fundament implementieren

- **Status:** REVIEW
- **Ziel:** Reproduzierbare Compose-, Backend- und Frontend-Basis herstellen.
- **Nicht-Ziele:** Noch keine vollständigen ERP-/CRM-Module oder Plugin-Runtime.
- **Abhängigkeiten:** ADRs und Architekturgrundlagen.
- **Akzeptanz:** Builds, Health Checks, Migrationen und lokale Dokumentation funktionieren.
- **Tests:** Build, Lint, Unit, Integration und Compose-Konfiguration.
- **Sicherheit:** Keine Secrets in Git; Datenbank nicht öffentlich exponieren.
- **Risiken:** Toolchain- und Portkonflikte der Ziel-VM.

Implementiert sind Compose, sichere lokale Konfiguration, PostgreSQL-Persistenz, Go-Health/Readiness/Systeminfo, OpenAPI-Grundlage, Next.js-Plattformshell, Container-Härtung und Basisprüfungen. Vor `DONE` fehlen gemäß Agenten-Workflow ein unabhängiger Review sowie echte Frontend- und Integrationstests.

Weitere Aufgaben verwenden `docs/06-agents/TASK_TEMPLATE.md`.

## READY

### WERK-002 – Stabilität und Testfundament

- **Ziel:** API-Fehlerformate, CORS/Origin, OpenAPI-Tests, Frontend-Tests und reproduzierbare CI ergänzen.
- **Nicht-Ziele:** Neue Fachmodule.
- **Akzeptanz:** Fehlerobjekte sind konsistent, mutierende Requests geprüft, CI baut API/Web/Compose.
- **Tests:** Go HTTP-Tests, PostgreSQL-Integration, Frontend Smoke, Compose config.
- **Sicherheit:** Keine fremden Origins, keine sensiblen Logs, keine Secrets in CI.

### WERK-003 – Identity-Härtung und Sessions

- **Ziel:** Passwortwechsel, Sitzungsübersicht/-widerruf, Rollenpflege und letzter-Admin-Schutz vollständig machen.
- **Abhängigkeiten:** WERK-002.
- **Akzeptanz:** Jeder Auth-Lebenszyklus erzeugt Audit; deaktivierte Konten verlieren Sessions.
- **Sicherheit:** Argon2id, generische Loginfehler, serverseitiges RBAC, CSRF/Origin.

### WERK-004 – Enterprise-Webapp und Audit-UX

- **Ziel:** Globale Shell, Dashboard, Benutzerstatus, Auditfilter, Lade-/Leer-/Fehlerzustände und Tastaturfluss.
- **Abhängigkeiten:** WERK-003.
- **Akzeptanz:** Ein Admin kann Benutzer und Sessions ohne rohe API-Aufrufe verwalten.

### WERK-005 – Betrieb, CI und Backup/Restore

- **Ziel:** GitHub Actions, sichere Backup-/Restore-Prüfung, Diagnose und Release-Checkliste.
- **Abhängigkeiten:** WERK-002.
- **Akzeptanz:** CI prüft Go, Web, OpenAPI, Compose; Restore wird reproduzierbar dokumentiert.

### WERK-006 – Organisation und erster Business-Object-Schnitt

- **Ziel:** Organisationsprofil und ein abgegrenztes Objekt, bevorzugt Kunde oder Projekt.
- **Abhängigkeiten:** WERK-003 bis WERK-005.
- **Nicht-Ziele:** Dynamische Universal-Engine, ERP-Gesamtausbau.

## 15h-Ausführung

- **0,5h — WERK-PLAN:** Plan, Risiken, Taskstatus und Entscheidungslog pflegen.
- **1,5h — WERK-002a:** CI-Workflow und reproduzierbare Toolchain-Prüfung.
- **1,5h — WERK-002b:** API-Fehlerformat, Validierung, HTTP- und OpenAPI-Tests.
- **2,5h — WERK-003a:** Passwortwechsel, Rollenpflege und Sitzungswiderruf.
- **1,5h — WERK-003b:** Audit-Härtung, Origin-/CSRF-Tests und Security Review.
- **3,0h — WERK-004:** Enterprise-Webapp, globale Navigation, Dashboard und Admin-UX.
- **1,5h — WERK-005:** Backup/Restore, Diagnose, Migrationstatus und Operations-Doku.
- **1,5h — WERK-RELEASE:** Compose-Smoke-Test, Dokumentation, Commit und Draft-PR.

Die Summe beträgt exakt **15 Stunden**. Der Agent arbeitet diese Reihenfolge autonom ab und eskaliert nur bei den in `EXECUTION_PLAN.md` definierten echten Blockern.
