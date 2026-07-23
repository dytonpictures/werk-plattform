# WERK-Codex-Konfiguration

Diese Projektkonfiguration ergänzt die versionskontrollierte `AGENTS.md`:

- `config.toml` begrenzt Projekt-Subagenten auf direkte, kontrollierbare
  Delegation.
- `agents/` enthält spezialisierte Rollen für Architektur, Review, Erkundung und
  klar beauftragete Umsetzung.

Codex lädt projektlokale `.codex`-Konfiguration nur in vertrauenswürdigen
Projekten. Persönliche Einstellungen, Modelle, Zugangsdaten, Provider und globale
Automatisierung gehören in die benutzerweite Codex-Konfiguration und werden nicht
in dieses Repository eingecheckt.

Die Rollen sind verfügbar, erzwingen aber keine automatische Delegation. Die
Regel dazu steht bewusst in `AGENTS.md`.
