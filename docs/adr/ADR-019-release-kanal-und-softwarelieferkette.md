# ADR-019: Release-Kanal und Softwarelieferkette

**Status:** Angenommen  
**Stand:** 22.07.2026

## Kontext

Der Quellstand benötigt neben der laufenden CI einen nachvollziehbaren,
versionierten Lieferweg. Ein Git- oder GitHub-Backup allein ist kein
auslieferbares Produkt und ersetzt weder Datenbank- noch Object-Storage-Backups.
Gleichzeitig soll die erste Pipeline noch keinen automatischen
Produktiv-Rollout oder einen vorgetäuschten Supportvertrag einführen.

## Entscheidung

- `Canary` ist der Entwicklungs- und Integrationskanal. Datierte
  `backup-YYYY-MM-DD`-Tags sichern einen Quellstand, lösen aber kein Release aus.
- Produktartefakte entstehen ausschließlich aus Tags im Format
  `vMAJOR.MINOR.PATCH` oder `vMAJOR.MINOR.PATCH-PRERELEASE`. Der getaggte Commit
  muss Bestandteil von `Canary` sein.
- Vor einer Veröffentlichung laufen Formatprüfung, `go vet`, Tests mit Race
  Detector, Compose-Prüfung, der vollständige Migrations-/RLS-Test und der
  verschlüsselte Restore-Test erneut. Ein Fehler verhindert die
  Veröffentlichung.
- GitHub Releases enthalten Linux-Pakete für `amd64` und `arm64` mit API,
  Worker und Migration sowie eine SHA-256-Prüfsummendatei.
- GHCR erhält getrennte Multi-Arch-Images für API, Worker, Migration,
  Dashboard und Backup. Ein `latest`-Tag wird nur für stabile SemVer-Releases
  geschrieben; Vorabversionen bleiben ausschließlich über ihre konkrete
  Version adressierbar.
- Release-Archive und Images erhalten signierte GitHub-/Sigstore-
  Herkunftsnachweise. Images enthalten zusätzlich BuildKit-SBOM- und
  Provenance-Attestierungen.
- Die Pipeline veröffentlicht Artefakte, führt aber kein Deployment aus.
  Umgebungsfreigaben, Rollback und Promotion in einen späteren stabilen Kanal
  bleiben getrennte, auditierbare Betriebsentscheidungen.

## Folgen

Ein Release lässt sich auf Commit, Workflow und Prüfsummen zurückführen. Die
getrennten Images bewahren die bestehenden Prozess- und Sicherheitsgrenzen.
GitHub und GHCR werden damit zum Liefer- und Wiederbeschaffungskanal, nicht zur
fachlichen Wahrheit und nicht zum Ersatz der verschlüsselten Betriebsbackups.

Ein Security-Supportzeitraum, signierte native Clientpakete, formale Promotion
zwischen Canary-, Vorschau- und Stabilkanal sowie externe Reproduzierbarkeits-
prüfungen werden mit der Produktreife konkretisiert; Änderungen vorbehalten.
