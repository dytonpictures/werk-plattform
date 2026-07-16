# WERK

WERK ist das Fundament einer offenen, self-hosted Business Operating Platform. Das Projekt startet als modularer Monolith mit Go, Next.js, TypeScript und PostgreSQL.

## Status

Das erste lauffähige Plattformfundament verbindet Next.js, die Go-API und PostgreSQL über Docker Compose. Health, Readiness, Systeminfo, eine erste Migration und die Desktop-first-Übersicht sind implementiert. Als nächster vertikaler Schritt folgen Identität, Benutzer, Rollen, Sitzungen und Auditierung.

## Lokal starten

```bash
make setup
make up
make migrate
make health
```

- Web: `http://127.0.0.1:3000`
- API Health: `http://127.0.0.1:8080/health`
- API Readiness: `http://127.0.0.1:8080/ready`
- Systeminfo: `http://127.0.0.1:8080/api/v1/system/info`

Verwendete Toolchains: Go 1.26.1, Node 24.18.0 LTS, Next.js 16.2.10 und PostgreSQL 18.4. Die exakten JavaScript-Abhängigkeiten stehen im Lockfile.

## Einstieg

1. `AGENTS.md` und `TASKS.md` lesen.
2. Mit `docs/README.md` in die Projektdokumentation einsteigen.
3. Entscheidungen unter `docs/adr/` berücksichtigen.

Die Lizenz ist noch nicht abschließend gewählt; siehe ADR-0016.
