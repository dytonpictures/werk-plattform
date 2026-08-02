# ADR-043 – Generische Komponentenbeobachtung und Statushistorie

**Status:** Vorgeschlagen  
**Datum:** 2026-08-02

## Kontext

Die administrative Betriebsübersicht kennt heute API, PostgreSQL, Worker und
Kafka als feste Sonderfälle. API und PostgreSQL werden aus dem erfolgreichen
Abruf abgeleitet, der Worker besitzt einen expierenden Heartbeat und Kafka war
zunächst nur als konfiguriert oder unbekannt darstellbar. Dieses Modell kann
weder einen belastbaren aktuellen Zustand aller Komponenten noch einen
Uptime-Kuma-artigen zeitlichen Verlauf liefern. Jede spätere Infrastruktur
würde erneut eigene Tabellen-, API- und UI-Logik benötigen.

Eine Betriebsbeobachtung ist keine fachliche Wahrheit und kein Auditnachweis.
Sie darf keine Broker-, Host-, Prozess-, Netzwerk-, Credential- oder freien
Fehlerdaten offenlegen. Gleichzeitig darf ein alter positiver Ping niemals
dauerhaft als `ready` erscheinen.

## Entscheidung

### Registrierte Komponenten

Komponenten werden installationsweit mit stabilem `component_key`,
Anzeigenamen, Sortierung und Reporterklasse registriert. Der erste Katalog
enthält:

```text
api          reporter=api
postgresql   reporter=api
worker       reporter=worker
kafka        reporter=worker
```

Neue Komponenten benötigen eine neue registrierte Komponente und einen
ausdrücklich berechtigten Reporter. Freie, vom Client gelieferte Schlüssel
werden nicht akzeptiert.

### Expierende Beobachtungen

Ein Reporter schreibt ausschließlich:

- seinen zufälligen laufzeitgebundenen Reporter-Identifier,
- den registrierten Komponentenschlüssel,
- `ready`, `degraded` oder `disabled`,
- PostgreSQL-seitig bestimmte Beobachtungs- und Ablaufzeit,
- eine begrenzte Build-Version, soweit sie zur Reporterklasse gehört.

`unknown` wird nie als positive Beobachtung geschrieben. Der Lesepfad leitet
`unknown` aus einer fehlenden oder abgelaufenen Beobachtung ab. Dadurch kann ein
abgestürzter Reporter keinen dauerhaft positiven Zustand hinterlassen.

Die Security-Definer-Schreibgrenze bindet Reporterklassen an Datenbankrollen:
die API-/Admin-Runtime darf nur `api` und `postgresql`, die Worker-Runtime nur
`worker` und `kafka` beobachten. Sie erhält keinen direkten Tabellenzugriff.

### Zustandsintervalle statt Ping-Rohdaten

Wiederholte Pings im selben Zustand verlängern nur die aktuelle Beobachtung.
Nur ein tatsächlicher Zustandswechsel schließt das vorherige Intervall und
öffnet ein neues. Fällt eine Beobachtung durch TTL-Ablauf aus, synthetisiert der
Lesepfad ab dem Ablaufzeitpunkt ein `unknown`-Intervall. Damit bleibt die
Historie klein und bildet dennoch Zustandsdauer ab.

Intervalle enthalten keine Ursache oder Fehlermeldung. Eine spätere begrenzte
Ursachentaxonomie benötigt einen eigenen versionierten Vertrag; freie
Fehlertexte bleiben ausgeschlossen. Geschlossene Intervalle werden nach einer
konfigurierten, serverseitig begrenzten Betriebsretention entfernt. Retention
ist kein Security-Audit und kein Legal Hold.

### Aktueller Zustand und Historie

Die bestehende Summary liefert den aktuellen aggregierten Zustand. Bei mehreren
aktiven Reportern gilt für eine Komponente:

1. mindestens eine frische `ready`-Beobachtung: `ready`,
2. sonst mindestens eine frische `degraded`-Beobachtung: `degraded`,
3. sonst ausschließlich frische `disabled`-Beobachtungen: `disabled`,
4. keine frische Beobachtung: `unknown`.

Queues bleiben eine getrennte PostgreSQL-Wahrheit. `dead` führt zu
`attention`, Retry oder ein nicht bereiter erforderlicher Transport zu
`degraded`.

Ein neuer read-only Admin-Endpunkt liefert für genau einen registrierten
Komponentenschlüssel einen begrenzten Zeitraum und begrenzte, chronologisch
sortierte Intervalle. Er verwendet dieselbe Berechtigung
`core.platform.operations.read`, `Cache-Control: no-store` und einen eigenen
Security-Audit-Typ. Der Browser liest weder Heartbeat- noch Historientabellen
direkt.

### UI

Komponentenzeilen sind auswählbar, wenn eine Historie verfügbar ist. Die
Detailansicht zeigt eine kompakte Zeitleiste, Zustandsdauer und den
Beobachtungszeitraum. Farben sind nie das einzige Signal. `unknown` wird nicht
als Ausfall und `disabled` nicht als Erfolg dargestellt.

## Folgen

- Kafka ist der erste externe Infrastrukturverbraucher, aber kein Sondermodell.
- Neue Komponenten erweitern Katalog und Reporter, nicht Summary und UI mit
  parallelen Statuswelten.
- PostgreSQL hält die minimierte Betriebsprojektion; Kafka, Valkey oder ein
  Telemetriesystem werden dafür nicht zur Wahrheit.
- Der Verlauf ist betriebliche Telemetrie und bleibt strikt von Security-Audit,
  Domain-Events und fachlicher Historie getrennt.
- Rohmetriken, Logs, Hostdiagnose und Ausführungsbefehle bleiben außerhalb
  dieses Vertrags und der bestehenden Admin-Ausführungsgrenze aus ADR-029.

## Änderbarkeit

Retentionsdauer, Darstellungsgranularität und zusätzliche registrierte
Komponenten dürfen mit Betriebserfahrung angepasst werden. Unverändert bleiben
registrierte Schlüssel, expierende positive Beobachtungen, rollenbegrenzte
Reporter, fehlende freie Fehlertexte und die Trennung von Beobachtung und
Audit.
