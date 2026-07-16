# ADR-0013: RBAC für Phase 1

- **Status:** Accepted
- **Kontext:** Benutzerverwaltung braucht verständliche Berechtigungen.
- **Entscheidung:** Rollen bündeln explizite Berechtigungen; das Backend erzwingt sie.
- **Begründung:** Überschaubar und administrierbar für den Start.
- **Alternativen:** ABAC, ACLs, Policy-Engine.
- **Positive Folgen:** Klare Rollen und Tests.
- **Negative Folgen:** Feingranulare Kontextregeln sind begrenzt.
- **Sicherheitsauswirkungen:** Default deny und Schutz privilegierter Rollen sind Pflicht.
- **Überprüfung:** Bei komplexeren Organisations- und Objektregeln.
