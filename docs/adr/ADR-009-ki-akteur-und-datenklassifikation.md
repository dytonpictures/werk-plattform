# ADR-009 – Kontrollierter KI-Akteur und Datenklassifikation

**Status:** Angenommen  
**Datum:** 2026-07-19

## Kontext

KI kann Zusammenhänge erklären und Arbeit vorbereiten. Sie darf dabei weder zum
verdeckten Administrator noch zu einem unkontrollierten Datenexport werden.

## Entscheidung

- Jeder KI-Lauf besitzt einen registrierten `AiAgent`, einen auslösenden
  Benutzer oder Dienst, Zweck, Tenant-Kontext, Modellprofil und Audit-Korrelation.
- KI arbeitet standardmäßig read-only über begrenzte, berechtigungsgeprüfte
  Tools. Direkter Zugriff auf PostgreSQL, Object Storage, Secrets oder Agents ist
  ausgeschlossen.
- Schreibende oder folgenreiche Vorschläge werden als `AiActionProposal` mit
  Ziel, Capability, Argument-Digest, Policy-Entscheidung und Freigabestatus
  gespeichert. Ein deterministischer Core-/Modul-Command führt sie erst nach
  gültiger Policy und nötigen menschlichen Freigaben aus.
- Prompt-, Tool- und Ergebnisdaten werden nach Datenklasse minimiert,
  redigiert/maskiert und mit Aufbewahrungsregeln behandelt. Externe Modelle sind
  nur über ausdrücklich freigegebene, tenant- und datenklassifikationsgeprüfte
  Adapter zulässig.
- KI-Aktionen, Tool-Aufrufe, Modellprofil, Policy, Freigabe und Ergebnis werden
  getrennt auditiert. Eine KI kann keine Rolle, Kontoart, Tenant-Grenze oder
  Freigabe umgehen.

## Grenzen

„KI-gestützt“ bedeutet nicht autonom. Ein Modellwechsel ist ein kontrollierter
Betriebs- und Datenschutzentscheid. Die Plattform bleibt auch ohne externe
Modelle und ohne aktivierten KI-Advisor vollständig betreibbar.

## Nachweis

Tests müssen Tenant- und Tool-Grenzen, Maskierung, fehlende Datenbankzugriffe,
Policy-Verweigerung, menschliche Freigabe, Audit und das Verhalten bei
Modell-/Provider-Ausfall prüfen.
