# Autonomer Ausführungsplan

## Zweck

Dieser Plan ist die operative Arbeitsgrundlage für WERK. Der Agent entscheidet normale technische Details selbstständig, solange sie Vision, Sicherheitsbaseline, ADRs und Definition of Done einhalten.

## Priorität

1. **Sicherheit und Datenintegrität** – keine Funktion darf Authentifizierung, Autorisierung, Sessions oder Audit schwächen.
2. **Lauffähigkeit** – jeder Slice muss im Compose-Stack reproduzierbar starten und einen überprüfbaren Test besitzen.
3. **Vertikaler Nutzwert** – bevorzugt werden Ende-zu-Ende-Funktionen statt isolierter Framework-Arbeit.
4. **Enterprise-UX** – klare Arbeitsbereiche, Status, Fehler, Leerzustände, Tastaturbedienung und konsistente Sprache.
5. **Betrieb** – Health, Backups, Migrationen, Logs und nachvollziehbare Updates.

## Arbeitszyklus

Für jeden Task:

1. relevante Dokumente und bestehende Implementierung lesen,
2. kleinsten vollständigen Slice bestimmen,
3. Entscheidung in Task/ADR dokumentieren, falls sie dauerhaft ist,
4. implementieren,
5. Unit-, Integrations- und Buildprüfungen ausführen,
6. Sicherheits- und Dokumentationsfolgen prüfen,
7. Status auf `REVIEW` setzen,
8. committen und den Draft-PR aktualisieren.

## Entscheidungsregeln

- Standardmäßig sichere, einfache und reversible Lösung wählen.
- Keine neue Infrastruktur einführen, wenn PostgreSQL, Go oder Next.js die Aufgabe abdecken.
- Keine unbestimmten Versionen und keine Secrets im Repository.
- API-Autorisierung bleibt serverseitig; Clientlogik ist Komfort, keine Sicherheitsgrenze.
- Bei widersprüchlichen Anforderungen gilt: Sicherheit, Datenintegrität, Lauffähigkeit, dann UX.
- Nur bei externen Secrets, produktiven DNS/TLS-Änderungen, irreversiblen Datenaktionen oder fehlender Berechtigung an den Auftraggeber eskalieren.

## Ausführungsreihenfolge

- `WERK-002` Stabilität und Testfundament
- `WERK-003` Identity-Härtung und Sessions
- `WERK-004` Enterprise-Webapp und Audit-UX
- `WERK-005` Betrieb, CI und Backup/Restore
- `WERK-006` Organisation und erster Business-Object-Schnitt

## 15-Stunden-Arbeitsfenster

| Block | Zeit | Ergebnis |
|---|---:|---|
| 1. Bestandsaufnahme und Backlogpflege | 0,5 h | Status, Risiken, priorisierte Tasks und Entscheidungslog aktualisiert |
| 2. CI- und Qualitätsfundament | 1,5 h | GitHub Actions für Go, Web, OpenAPI und Compose |
| 3. API-Verträge und Fehlerstandard | 1,5 h | konsistente Fehlerobjekte, Validierung, OpenAPI-Abgleich und HTTP-Tests |
| 4. Identity-Härtung | 2,5 h | Rollenpflege, Passwortwechsel, Sitzungswiderruf und letzter-Admin-Schutz |
| 5. Audit- und Sicherheitsprüfung | 1,5 h | Auditfilter, sensible Metadatenbereinigung, Origin/CSRF-Tests, Security-Checkliste |
| 6. Enterprise-Webapp | 3,0 h | globale Shell, Dashboard, Benutzer-/Audit-UX, Lade-/Fehler-/Leerzustände |
| 7. Betrieb und Datenpflege | 1,5 h | Backup/Restore-Prüfung, Diagnose, Migrationstatus und Betriebsdokumentation |
| 8. Integration und Release | 1,5 h | Compose-End-to-End, Smoke-Test, PR-Aktualisierung, Abschlussbericht |

**Gesamt: 15,0 Stunden.**

Zeitbudget-Regel: Wenn ein Block früher fertig wird, wandert die Zeit in Tests und Härtung des nächsten Blocks. Neue Features werden erst nach Abschluss der Sicherheits- und Lauffähigkeitskriterien begonnen.

## Übergaben

Jeder Slice dokumentiert geänderte Dateien, Tests, Sicherheitsauswirkungen, Annahmen, Risiken und den nächsten Task. Ein unfertiger Task bleibt sichtbar `BLOCKED` oder `REVIEW`; er wird nicht stillschweigend als fertig markiert.
