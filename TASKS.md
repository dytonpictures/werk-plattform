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
