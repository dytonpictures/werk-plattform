# ADR-0011: PostgreSQL als Session-Quelle der Wahrheit

- **Status:** Accepted
- **Kontext:** Sitzungen müssen ohne zusätzlichen Pflichtdienst widerrufbar sein.
- **Entscheidung:** PostgreSQL speichert gehashte Sitzungstoken und deren Status.
- **Begründung:** Transaktional, persistent und bereits erforderlich.
- **Alternativen:** Redis, zustandslose JWTs.
- **Positive Folgen:** Einfacher Betrieb und konsistenter Widerruf.
- **Negative Folgen:** Zusätzliche Datenbanklast.
- **Sicherheitsauswirkungen:** Nur Hashes speichern; Ablauf und Widerruf strikt prüfen.
- **Überprüfung:** Bei belegtem Skalierungsproblem.
