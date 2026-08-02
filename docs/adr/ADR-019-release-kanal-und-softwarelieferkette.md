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
  Detector, native Paketprüfung, der vollständige Migrations-/RLS-Test und der
  verschlüsselte Restore-Test erneut. Ein Fehler verhindert die
  Veröffentlichung.
- GitHub Releases enthalten native Debian-/Ubuntu-Pakete für `amd64` mit
  API samt eingebetteter Weboberfläche, Worker, Migration, Verwaltungswerkzeug,
  systemd-Units und sicheren Konfigurationsvorlagen sowie eine
  SHA-256-Prüfsummendatei. Der verbindliche First-Run-Vertrag steht in ADR-028.
- Release-Archive und Linux-Pakete erhalten signierte GitHub-/Sigstore-
  Herkunftsnachweise, Prüfsummen und eine maschinenlesbare Komponentenliste.
- Die Pipeline veröffentlicht Artefakte, führt aber kein Deployment aus.
  Umgebungsfreigaben, Rollback und Promotion in einen späteren stabilen Kanal
  bleiben getrennte, auditierbare Betriebsentscheidungen.

## Folgen

Ein Release lässt sich auf Commit, Workflow und Prüfsummen zurückführen. Die
getrennten nativen Binärdateien bewahren die bestehenden Prozess- und
Sicherheitsgrenzen. GitHub wird damit zum Liefer- und Wiederbeschaffungskanal, nicht zur
fachlichen Wahrheit und nicht zum Ersatz der verschlüsselten Betriebsbackups.

Ein Security-Supportzeitraum, signierte native Clientpakete, formale Promotion
zwischen Canary-, Vorschau- und Stabilkanal sowie externe Reproduzierbarkeits-
prüfungen werden mit der Produktreife konkretisiert; Änderungen vorbehalten.
