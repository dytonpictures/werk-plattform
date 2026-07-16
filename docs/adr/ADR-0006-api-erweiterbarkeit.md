# ADR-0006: API-basierte Erweiterbarkeit vor Plugin-Runtime

- **Status:** Accepted
- **Kontext:** Erweiterbarkeit ist nötig, fremder Code im Kern aber riskant.
- **Entscheidung:** Zunächst APIs, Events und Webhooks; keine Plugin-Runtime.
- **Begründung:** Klare Vertrauensgrenzen und geringere Betriebskomplexität.
- **Alternativen:** In-Process-Plugins, Sandbox- oder WASM-Runtime.
- **Positive Folgen:** Kontrollierbare Schnittstellen.
- **Negative Folgen:** Weniger tiefe UI- und Kernintegration.
- **Sicherheitsauswirkungen:** Keine direkte Datenbankfreigabe oder Ausführung fremden Codes.
- **Überprüfung:** Nach stabiler öffentlicher API.
