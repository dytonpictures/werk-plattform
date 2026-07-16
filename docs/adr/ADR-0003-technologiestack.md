# ADR-0003: Go, TypeScript, Next.js und PostgreSQL

- **Status:** Accepted
- **Kontext:** Backend, Web und Datenhaltung benötigen einen langfristig wartbaren Stack.
- **Entscheidung:** Go für APIs, TypeScript/Next.js für Web und PostgreSQL für Persistenz.
- **Begründung:** Starke Typisierung, reife Ökosysteme und guter Self-Hosting-Support.
- **Alternativen:** Java/Kotlin, .NET, Node-only, andere SQL-Datenbanken.
- **Positive Folgen:** Klare Verantwortlichkeiten und robuste Toolchains.
- **Negative Folgen:** Zwei Programmiersprachen und Buildketten.
- **Sicherheitsauswirkungen:** Abhängigkeiten und Laufzeiten werden gepinnt und geprüft.
- **Überprüfung:** Vor einem Major-Stack-Upgrade.
