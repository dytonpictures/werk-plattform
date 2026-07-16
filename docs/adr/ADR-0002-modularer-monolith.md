# ADR-0002: Modularer Monolith als Startarchitektur

- **Status:** Accepted
- **Kontext:** WERK braucht klare Grenzen ohne verteilte Betriebskomplexität.
- **Entscheidung:** Das Backend startet als modularer Monolith.
- **Begründung:** Einfache Transaktionen, Builds und Deployments bei späterer Auslagerbarkeit.
- **Alternativen:** Microservices, unstrukturierter Monolith.
- **Positive Folgen:** Schnelle Entwicklung und überschaubarer Betrieb.
- **Negative Folgen:** Moduldisziplin muss intern durchgesetzt werden.
- **Sicherheitsauswirkungen:** Weniger Netzwerkgrenzen; Berechtigungen bleiben explizit.
- **Überprüfung:** Wenn messbare Skalierungs- oder Teamgrenzen entstehen.
