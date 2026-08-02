# GitHub-Automation für WERK (Free-Profil)

Dieses Verzeichnis stellt die wiederholbaren Prüfungen und Vorlagen bereit. Alle
Jobs verwenden kurzlebige Standard-Runner, minimale `GITHUB_TOKEN`-Rechte und
speichern keine fachlichen Daten. Die Konfiguration benötigt auch bei einem
privaten Repository weder GitHub Code Security noch GitHub Enterprise.
Produktions-Deployments sind absichtlich nicht enthalten.

## Workflows

| Workflow | Zweck | Trigger |
|---|---|---|
| `CI` | Format, OpenAPI/JavaScript, `go vet`, Module, Race-Tests, Builds | PR, `Canary` |
| `Integration` | Wegwerf-PostgreSQL, getrennte Rollen, Migrationen und RLS-Tests | relevante PRs, `Canary` |
| `Security` | erreichbare Go-Schwachstellen mit `govulncheck` | PR, `Canary`, wöchentlich |
| `Release` | SemVer-, Canary-, Paket-, Prüfsummen- und SBOM-Nachweis | `v*`-Tag, manuell |
| `Agent PR guard` | Secret-Dateinamen und sensible Pfade in PRs markieren/stoppen | PR |
| `Copilot setup steps` | read-only Go-Umgebung für den GitHub Coding Agent | Konfigurationsänderung, manuell |

Dependabot bündelt wöchentlich Go- und Actions-Aktualisierungen. Codex, Claude
und Copilot müssen zusätzlich `AGENTS.md` und die drei dort genannten
Architekturquellen lesen. `copilot-instructions.md` spiegelt die wichtigsten
Grenzen für GitHub Copilot; diese Datei ersetzt keine menschliche Prüfung.

## Einmalig in GitHub konfigurieren

Für `Canary` ein Repository Ruleset aktivieren:

1. Pull Requests erzwingen, mindestens eine Freigabe und Code-Owner-Review
   verlangen; Freigaben bei neuen Commits verwerfen.
2. Die Jobs aus CI, Integration, Security und Agent PR Guard als Required Checks
   setzen.
3. Force-Push, Branch-Löschung und Umgehung der Regeln sperren; signierte Commits
   nach Teamentscheidung aktivieren.
4. Actions auf ausgewählte, verifizierte Actions begrenzen. Falls die
   Organisation zwingendes SHA-Pinning aktiviert, die Major-Referenzen vorher
   auf Commit-SHAs umstellen; Dependabot kann diese anschließend aktuell halten.
5. Private Vulnerability Reporting aktivieren, soweit GitHub es im Repository
   anbietet. Secret Scanning/Push Protection sind im privaten Free-Profil nicht
   vorausgesetzt. Agenten und Fork-PRs erhalten keine Repository-Secrets.
6. GitHub-Apps für Codex/Claude nur mit `contents: read` und, wenn PR-Erstellung
   gewünscht ist, eng begrenztem `pull_requests: write` installieren. Kein
   `administration`, `secrets`, `deployments` oder Produktions-Environment.

Releases führen kein Deployment aus. Ein späteres Deployment braucht ein
separates geschütztes GitHub Environment mit benannten menschlichen Reviewern,
OIDC statt langlebiger Cloud-Schlüssel und einen dokumentierten Rollback-Vertrag.

## Release

Nur ein vorhandener Tag `vMAJOR.MINOR.PATCH[-PRERELEASE]`, dessen
Commit in `Canary` enthalten ist, darf veröffentlicht werden. Beispiel:

```bash
git tag -s v0.1.0-preview.0 -m "WERK v0.1.0-preview.0"
git push origin v0.1.0-preview.0
```

Der Workflow erzeugt Debian-/Ubuntu-Artefakte, SHA-256-Prüfsummen und ein
SPDX-SBOM. GitHub Artifact Attestations sind bewusst nicht aktiviert, weil sie
für private Repositories GitHub Enterprise Cloud voraussetzen. Der Workflow
verändert keine Installation.

## Kostenkontrolle

Öffentliche Repositories können Standard-Runner kostenlos nutzen. Ein privates
Repository auf GitHub Free besitzt derzeit 2.000 Actions-Minuten pro Monat und
500 MB gemeinsam genutzten Artifact-/Package-Speicher. Dieses Setup lädt keine
gewöhnlichen CI-Artefakte hoch; nur bewusst erstellte GitHub Releases bleiben
gespeichert. In den Billing-Einstellungen sollte ein Budget mit „Stop usage when
budget limit is reached“ gesetzt und keine Zahlungsmethode für Actions-Mehrverbrauch
freigegeben werden. Größere Runner und selbst gehostete Runner für fremde PRs
sind nicht vorgesehen.

CodeQL, GitHub Dependency Review, private Artifact Attestations, Secret Scanning
und Push Protection werden für das private Free-Profil nicht vorausgesetzt.
Dependabot-Versionupdates und `govulncheck` bleiben aktiv. Bei einem späteren
öffentlichen Repository können die GitHub-Sicherheitsfunktionen kostenlos
ergänzt werden.
