# ADR-028 – Nativer Betrieb, zentrale `.env` und First-Run

**Status:** Angenommen
**Datum:** 2026-07-26

## Kontext

WERK soll von einer kleinen Betreiber- und Entwicklungsgruppe nachvollziehbar
gestartet werden können. Mehrere gleichwertige Laufzeitwege, zusätzliche
Proxykonfiguration und verteilte Secret-Dateien erhöhen die Fehlerfläche, bevor
sie einen belegten Nutzen liefern. PostgreSQL bleibt die fachliche Wahrheit und
darf nicht durch eine eingebettete Ersatzdatenbank oder gemeinsame
Superuser-Credentials vereinfacht werden.

## Entscheidung

### Ein Betriebsweg

WERK wird als native Go-Binärdateien und Linux-Paket ausgeliefert. Das erste
Referenzprofil ist Debian/Ubuntu auf `amd64`. Die API liefert die
statische Weboberfläche am selben Origin aus. Worker und Migration bleiben
eigene Binärdateien. PostgreSQL wird betreiberseitig bereitgestellt; Kafka und
Valkey sind keine Voraussetzung für die synchrone erste Nutzung.

### Eine zentrale Konfigurationsdatei

Der Quellstart verwendet `.env`, das Linux-Paket `/etc/werk/.env`. Die Datei
wird niemals eingecheckt, überschreibt keine bereits gesetzte Betriebssystem-
Variable und wird ohne Shellauswertung gelesen. Unter Linux muss sie vor Zugriff
anderer Benutzer und vor Gruppen-Schreibzugriff geschützt sein.

Zentral bedeutet nicht „ein gemeinsames Datenbankkonto“. Mindestens Work,
Identity, Admin, Worker und Migration erhalten getrennte URLs und PostgreSQL-
Rollen. Dienste laufen unter getrennten unprivilegierten Konten. Die gemeinsame
Datei ist im Paket nur für die WERK-Dienstgruppe lesbar.

### Start und Diagnose

`sh scripts/start.sh` führt genau drei Schritte aus:

1. `.env` ohne Secret-Ausgabe validieren,
2. Migrationen anwenden,
3. API samt eingebetteter Weboberfläche starten.

Kafka ist im einfachen Profil deaktiviert. Wird Kafka aktiviert, startet der
Betreiber den Worker als eigenen Prozess. `werkctl doctor` prüft dieselbe
zentrale Datei und kann wahlweise nur die Struktur oder zusätzlich die externen
Abhängigkeiten prüfen.

### Transport und First-Run

Lokales HTTP ohne TLS bindet ausschließlich an Loopback. Produktion verlangt
die direkte TLS-/mTLS-Implementierung aus ADR-023. Ein vorgeschalteter Proxy ist
nicht Teil des Startvertrags.

Das Paket startet keine unkonfigurierte Installation. Jede `CHANGE_ME`-Marke
muss ersetzt werden. Der First-Run erzeugt weder schwache Passwörter noch eigene
Vertrauensanker oder stille TLS-Fallbacks.

### Release- und Abnahmevertrag

Ein natives Release wird nur veröffentlicht, wenn:

- API, Worker, Migration und `werkctl` aus demselben Commit und derselben
  Version gebaut werden,
- Paketinhalt, Dateirechte und systemd-Units statisch geprüft sind,
- Migration, API, Worker, Weboberfläche und Diagnose einen frischen nativen
  Linux-Smoke-Test bestehen,
- direkte TLS-Konfiguration bei fehlendem oder ungültigem Material geschlossen
  scheitert,
- Prüfsummen und Herkunftsnachweise veröffentlicht werden.

## Bewusste Nicht-Ziele

- keine eingebettete PostgreSQL-, Kafka- oder Valkey-Instanz,
- kein automatischer Cloud-Rollout,
- kein HA-, Witness- oder Orchestratorprofil,
- keine eigene CA oder automatische öffentliche Zertifikatsausstellung,
- kein stilles Upgrade über nicht getestete Migrationssprünge.

## Folgen

Es gibt einen verständlichen lokalen und serverseitigen Laufzeitpfad. Weniger
Betriebsvarianten reduzieren Pflege- und Testaufwand, ohne die getrennten
Kontoarten, Datenbankrollen, Policy-, Audit- oder Tenantgrenzen aufzuweichen.
Spätere Betriebsprofile benötigen einen messbaren Bedarf, ein eigenes ADR und
vollständige Migrations- und Wiederherstellungstests.
