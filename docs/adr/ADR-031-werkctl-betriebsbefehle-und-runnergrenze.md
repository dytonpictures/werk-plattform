# ADR-031 – `werkctl`-Betriebsbefehle und Runnergrenze

**Status:** Vorgeschlagen  
**Datum:** 2026-07-29

## Kontext

Betreiber benötigen neben dem Browser eine lokale, skriptfähige Schnittstelle
für Diagnose, Prozessstart, Migration, kontrollierte Updates und später das
Pairing eines Hybrid-Cloud-Profils. Diese Aufgaben besitzen unterschiedliche
Privilegien und Beweisgrenzen. Ein allgemeiner Shell-Runner in der Admin-API
oder eine automatische Ableitung verändernder Aktionen aus einem Healthcheck
würde die in ADR-029 festgelegte Host-Ausführungsgrenze aufheben.

`werkctl migrate` ist bereits ein ausdrücklicher, einmaliger Mutationsbefehl.
`doctor` und `status` sind dagegen beobachtend: Diagnose der lokalen
Konfiguration und Laufzeitrollen beziehungsweise Prüfung der öffentlichen
API-Signale. Diese Unterschiede müssen auch bei weiteren Befehlen sichtbar
bleiben.

Die Vision verlangt für Desktop-Clients, fachliche CLI-Werkzeuge, SDKs,
Integrationen und Agents definierte APIs statt direkten Datenzugriff. `werkctl`
ist davon als installationslokales Betriebswerkzeug getrennt: Es besitzt keine
Fachoperationen und keinen Tenant-Schreibpfad. Direkter PostgreSQL-Zugriff
bleibt auf read-only Probes mit den fünf zweckgebundenen Rollen sowie den
ausdrücklichen Migrator-Command begrenzt.

## Entscheidung

- `werkctl` wird als typisierte lokale Betriebs-CLI aufgebaut, nicht als
  allgemeiner Befehlsinterpreter. Jeder Command besitzt einen versionierten
  Eingabe-, Ergebnis-, Exitcode- und Redaktionsvertrag.
- `version` gibt ausschließlich die Build-Kennung des lokalen Binärs aus.
- Beobachtende Diagnosebefehle wie `doctor`, `status` und später begrenzte Log-
  oder Health-Sichten führen keine Reparatur automatisch aus. Sie melden stabile
  Check-IDs, `pass`/`warn`/`fail`, Evidenz ohne Secrets und eine konkrete
  nächste Aktion.
- Verändernde Befehle verwenden getrennte Command-Handler und eng begrenzte
  Runner. Ein Runner akzeptiert typisierte Operationen und eine explizite
  Zielart; er nimmt weder freie Shell-Zeilen noch vom Server gelieferte
  Argumentlisten entgegen.
- Das erste ausführbare Betriebsprofil bleibt nativ. Weitere Adapter wie
  Docker Compose, `systemd` oder Kubernetes benötigen jeweils einen eigenen
  Vertrag für Erkennung, Berechtigungen, Zeitlimits, Idempotenz und
  Fehlerabbildung. Eine Diagnose darf einen Adapter erkennen, aber nicht allein
  dadurch eine Mutation auslösen.
- Start, Stop und Neustart werden erst aktiviert, wenn API und Worker getrennt
  adressierbar sind und der Runner weder PostgreSQL noch einen fremden Prozess
  als WERK-Komponente fehlklassifizieren kann. `restart` ist kein Ersatz für
  Readiness, Drain oder einen Upgradevertrag.
- Migration bleibt ausschließlich aufwärtsgerichtet und checksumgebunden.
  `status` und `plan` sind getrennt von `apply`; angewendete Migrationen werden
  nie editiert und ein generisches Down-Migration-Kommando wird nicht angeboten.
- Updates erhalten die Stufen `check`, `plan` und `apply`. `apply` setzt
  signierte Release-Metadaten, exakte Zielversion, Kompatibilitätsprüfung,
  Backup-/Restore-Nachweis, Wartungszustand und eine dokumentierte
  Rücknahmestrategie voraus. Ein Paketdownload allein ist kein Updateerfolg.
- Hybrid-Cloud-Pairing erhält einen eigenen Identity-/Transportvertrag. Es
  verwendet kurzlebiges, einmaliges Pairing-Material, stellt eine begrenzte
  Installationsidentität und mTLS-Vertrauen her und legt keine gemeinsame
  Core-Datenbank, kein dauerhaftes Klartext-Secret und keine implizite
  Benutzervertretung an.
- Maschinenlesbare Ausgabe verwendet ein versioniertes JSON-Schema und dieselbe
  fachliche Auswertung wie die menschenlesbare Ausgabe. Secrets, DSN-Passwörter,
  Token, private Schlüssel und ungefilterte Prozessumgebungen sind in beiden
  Formen ausgeschlossen.
- Nicht interaktive Mutationen verlangen alle notwendigen Parameter
  ausdrücklich. Interaktive Bestätigungen dürfen Sicherheit verbessern, sind
  aber kein Ersatz für Autorisierung, Audit, Idempotenzschlüssel und sichere
  Defaults.
- Der API-Prozess erhält durch `werkctl` keine Host-Rechte. Eine spätere
  browserausgelöste Operation benötigt weiterhin den getrennten,
  minimal-privilegierten Ops-Agenten aus ADR-029 und denselben typisierten
  Command-Vertrag.

## Vorgesehene Befehlsfamilien

```text
werkctl version
werkctl doctor [--json]
werkctl status --url URL [--json]
werkctl migrate status|plan|apply
werkctl service start|stop|restart --target TARGET
werkctl update check|plan|apply --version VERSION
werkctl pair create|accept|status|revoke
```

Die Auflistung ist ein Zielvertrag, keine Behauptung über bereits ausführbare
Befehle. Zunächst implementiert bleiben nur die im jeweiligen CLI-Help
aufgeführten Commands.

## Folgen

`werkctl` kann schrittweise zum verlässlichen lokalen Werkzeug für Single Host,
Paketbetrieb und spätere Hybrid-Profile wachsen, ohne eine zweite unkontrollierte
Admin- oder Shell-Schnittstelle zu schaffen. Die zusätzliche Struktur kostet
pro Runner einen eigenen Test- und Sicherheitsvertrag, verhindert dafür aber,
dass Deploymentdetails, Healthsignale und fachliche Plattformautorität
miteinander vermischt werden.

Die Prozessgrenzen aus
[`ADR-002`](ADR-002-container-und-betriebsgrenzen.md), die Lieferkette aus
[`ADR-019`](ADR-019-release-kanal-und-softwarelieferkette.md), das native
Startprofil aus
[`ADR-028`](ADR-028-native-linux-distribution-und-first-run.md) und die
administrative Ausführungsgrenze aus
[`ADR-029`](ADR-029-administrative-beobachtung-und-ausfuehrungsgrenze.md)
bleiben verbindlich.
