# WERK – KI-Datenklassifikation und externe Modellnutzung

**Status:** verbindliche Leitlinie für zukünftige KI-Funktionen

KI wird datenklassenabhängig eingesetzt. Externe Modelle sind möglich, aber nie
der Standard für ungeprüfte Unternehmensdaten.

| Klasse | Beispiele | Externe KI-Nutzung |
|---|---|---|
| `public` | freigegebene öffentliche Inhalte | zulässig |
| `internal` | allgemeine interne Arbeitsinformationen | nur mit freigegebenem Anbieter und Zweck |
| `confidential` | Kunden-, Vertrags-, Projekt- und Betriebsdaten | nur maskiert/minimiert und nach expliziter Policy |
| `restricted` | Personal-, Finanz-, Zugangsdaten, Geheimnisse, sensible Audits | keine externe Übertragung; lokale oder isolierte Verarbeitung erforderlich |

## Verbindliche Regeln

- Vor jedem externen Modellaufruf klassifiziert ein Tool die Daten und entfernt
  nicht erforderliche Felder.
- Maskierung ersetzt direkt identifizierende Inhalte durch stabile Platzhalter;
  die Zuordnung verbleibt ausschließlich innerhalb von WERK.
- Anonymisierung wird nur behauptet, wenn eine Re-Identifizierung nach dem
  dokumentierten Verfahren ausgeschlossen oder vertretbar unwahrscheinlich ist.
- Jeder KI-Lauf protokolliert Datenklasse, Modellprofil, Übertragungsziel,
  Maskierungs-/Redaktionsregel, Auslöser und Zweck.
- `restricted`-Daten, Secrets und Zugangsdaten werden nie an externe Modelle
  gesendet.
- Schreibende KI-Vorschläge bleiben `AiActionProposal`s und benötigen weiterhin
  Core-Policy, Audit und gegebenenfalls menschliche Freigabe.

Die konkrete Providerliste, Datenregion, Auftragsverarbeitung und
Aufbewahrungsdauer werden vor Aktivierung eines externen KI-Profils als separate
administrative Konfiguration und Sicherheitsprüfung festgelegt.
