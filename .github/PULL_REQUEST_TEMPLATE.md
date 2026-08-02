## Änderung

<!-- Was ändert sich und warum? -->

## Sicherheits- und Architekturprüfung

- [ ] Tenant-Kontext und serverseitige Autorisierung bleiben explizit.
- [ ] `work`, `admin` und `service` bleiben getrennt.
- [ ] Keine Secrets, Produktionsdaten oder direkten Agenten-/Plugin-DB-Zugriffe.
- [ ] Migrationen sind neu und unveränderlich; Audit/Outbox sind bei Mutationen atomar.
- [ ] Öffentliche API-, Ressourcen-, Berechtigungs- und Event-Verträge sind versioniert.
- [ ] Nicht zutreffende Punkte sind im PR begründet.

## Nachweis

<!-- Ausgeführte Tests sowie bewusst nicht ausgeführte Prüfungen. -->

## Agentennutzung

<!-- Falls Codex, Claude oder Copilot beteiligt war: Tool, Umfang und menschliche Prüfung nennen. -->

- [ ] Agentenänderungen wurden vollständig menschlich geprüft.
- [ ] Der Agent hatte keine Produktions-Secrets oder Deployment-Berechtigung.

