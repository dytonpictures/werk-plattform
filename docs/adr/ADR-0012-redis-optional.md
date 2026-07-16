# ADR-0012: Redis zunächst optional

- **Status:** Accepted
- **Kontext:** Phase 1 benötigt keinen zusätzlichen Pflichtdienst.
- **Entscheidung:** Redis wird vorbereitet, aber nicht vorausgesetzt.
- **Begründung:** PostgreSQL deckt Sitzungen und frühe Lastprofile ab.
- **Alternativen:** Redis als Pflichtdienst, keinerlei Redis-Vorbereitung.
- **Positive Folgen:** Weniger Betriebsaufwand.
- **Negative Folgen:** Manche Cache- oder Queue-Funktionen folgen später.
- **Sicherheitsauswirkungen:** Falls aktiviert, bleibt Redis intern und authentisiert.
- **Überprüfung:** Bei konkreten Cache-, Queue- oder Rate-Limit-Anforderungen.
