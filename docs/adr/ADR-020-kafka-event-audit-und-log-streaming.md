# ADR-020 – Kafka für Ereignis-, Audit- und Log-Streaming

**Status:** Angenommen  
**Datum:** 2026-07-22

## Kontext

Die Plattform benötigt von Beginn an eine gemeinsame Transportgrenze für
fachliche Ereignisse, minimierte Security-Audits und strukturierte
Betriebslogs. Mehrere spätere Verbraucher wie Integrationen, SIEM,
Suchprojektionen und Analysen sollen unabhängig nachziehen können. Der
Transport darf jedoch weder PostgreSQL als fachliche Wahrheit ersetzen noch
fachliche Transaktionen von einem erreichbaren Broker abhängig machen.

Kafka ist als verteilter Event-Stream für diesen Zweck geeignet und besitzt ein
offizielles Docker-Image. Seine Exactly-once-Eigenschaften gelten nicht
automatisch über die Grenze zu einer externen PostgreSQL-Transaktion. Topic-
Retention und Log-Compaction sind außerdem Transport- beziehungsweise
Speicherregeln und kein revisionssicherer Aufbewahrungsnachweis.

## Entscheidung

### Kafka ist ein mitgelieferter Plattformdienst

Das Single-Host-Compose-Profil enthält einen persistenten Apache-Kafka-Knoten im
KRaft-Modus und legt drei getrennte Topics an:

```text
platform.domain-events.v1
platform.security-audit.v1
platform.runtime-logs.v1
```

Der einzelne kombinierte Broker/Controller ist das lokale und Single-Host-
Profil. Er stellt keine Hochverfügbarkeit dar. Ein produktiver Mehrhostbetrieb
benötigt mehrere Broker beziehungsweise Controller, getrennte Fehlerdomänen,
Replikation, Kapazitätsplanung, Monitoring und ein getestetes
Wiederanlaufverfahren.

Die Go-Laufzeit verwendet einen Kafka-Adapter hinter einem schmalen
Publisher-Vertrag. Brokeradressen, Topics, Client-ID, TLS und SASL werden
konfiguriert. Produktion akzeptiert bei aktiviertem Kafka kein unverschlüsseltes
Clientprofil und verlangt SASL oder ein Client-Zertifikat.

### PostgreSQL bleibt die Wahrheit

Domain-Events entstehen weiterhin atomar mit der Fachänderung in der
PostgreSQL-Outbox. Der Worker veröffentlicht daraus nach Kafka und speichert
erst nach bestätigter Veröffentlichung seinen Consumer-Receipt. Bei
Broker-Ausfall bleiben Ereignis, Retry-Zustand und gegebenenfalls Dead Letter in
PostgreSQL erhalten.

Zustellung über die Systemgrenze ist **at least once**. Jede Nachricht trägt
eine stabile `event_id`; Verbraucher müssen idempotent sein oder diese ID
deduplizieren. Eine Wiederholung nach bestätigter Kafka-Schreiboperation, aber
vor gespeichertem PostgreSQL-Receipt, ist zulässig und wird nicht als
Exactly-once-Vertrag ausgegeben.

### Audit wird minimiert exportiert

Jeder neue Eintrag im autoritativen Security-Audit erzeugt durch einen
PostgreSQL-Trigger in derselben Transaktion einen Eintrag in einer getrennten
Audit-Export-Queue. Der Export enthält ausschließlich:

- Audit-ID, Typ, Zeitpunkt und Ergebnis,
- optional Tenant- und Account-Referenz,
- Request- und Correlation-ID,
- feste Klassifikations-, Zweck- und Aufbewahrungstags.

Freie `details`, Session-IDs, Credentials und Secrets werden nicht in den
Kafka-Auditstream übernommen. Die Queue enthält nur Zustellzustand; der
eigentliche Audit-Eintrag bleibt unveränderliche Wahrheit. Das Lesen des
Kafka-Topics ist ausschließlich vertrauenswürdigen Betriebsverbrauchern
gestattet. Tenant-Benutzer lesen Audit weiterhin über autorisierte APIs und
nicht direkt über Kafka.

### Gemeinsames Envelope und Tagging

Domain-Events, Audits und Laufzeitlogs verwenden das versionierte Envelope
`platform.event-envelope.v1`. Domain-Events erhalten einen begrenzten
`map[string]string`-Tagvertrag. Mindestens vorhanden sind:

```text
data.classification
processing.purpose
retention.class
```

Fehlende Domain-Tags werden konservativ als `restricted`,
`platform-event-delivery` und `domain-event` ergänzt. Tags sind Routing- und
Governancekontext, keine Berechtigungen. Sie dürfen keine Passwörter, Tokens,
freien Personendaten oder andere Geheimnisse enthalten. Autorisierung und
Tenant-Grenzen werden weiterhin vor der Erzeugung serverseitig entschieden.

Kafka-Headers spiegeln nur kleine Routingmerkmale wie Envelope-Version,
Event-ID, Event-Typ, Tenant und Klassifikation. Das JSON-Envelope bleibt der
kanonische Nachrichtenvertrag.

### Betriebslogs bleiben nicht blockierend

Go-Dienste schreiben weiterhin strukturierte JSON-Logs nach Standardausgabe.
Ein begrenzter asynchroner Handler spiegelt sie zusätzlich nach Kafka. Er
ergänzt Service, Umgebung, Build-Version und Instanz-ID und schwärzt bekannte
Credential-, Token-, Cookie-, Session- und Secret-Felder. Ein voller Puffer
darf Betriebslogs verwerfen und zählt diese Fälle; er darf keinen HTTP-Request
und keine Fachtransaktion blockieren. Security-Audit verwendet diesen
verlusttoleranten Pfad ausdrücklich nicht.

### Topics sind getrennte Sicherheitsräume

Domain-Events, Audit und Logs werden nicht in einem gemeinsamen Topic gemischt.
Sie besitzen getrennte ACLs, Retention, Größenlimits und Verbraucher. Im
Single-Host-Startprofil gilt `cleanup.policy=delete`; Audit-Topics werden nicht
kompaktiert. Kafka-Retention ersetzt weder PostgreSQL-Aufbewahrung noch Legal
Hold oder ein Langzeitarchiv.

## Folgen

- Kafka ist ab dem normalen Container- und nativen Entwicklungsstart praktisch
  angebunden und nicht nur als späterer Erweiterungspunkt dokumentiert.
- Neue Domain-Event-Produzenten verwenden denselben Tag- und Envelope-Vertrag.
- Ein Kafka-Ausfall erzeugt Rückstau und Alarmbedarf, verliert aber keine
  autoritativen Domain-Events oder Security-Audits.
- Direkte Topic-Nutzung durch Fachapps oder Tenant-Clients bleibt verboten;
  vertrauenswürdige Consumer erzeugen autorisierte Projektionen oder Exporte.
- Broker-ACLs schützen den Transport, ersetzen aber nicht die Plattform-Policy
  pro Ressource oder Datensatz.
- Ein späterer Clusterbetrieb benötigt ein eigenes Betriebs- und HA-ADR.

## Referenzen

- [Apache Kafka Docker](https://kafka.apache.org/43/getting-started/docker/)
- [Apache Kafka Delivery Semantics](https://kafka.apache.org/42/design/design/)
- [Apache Kafka Security](https://kafka.apache.org/43/security/security-overview/)
- [Apache Kafka Topic-Konfiguration](https://kafka.apache.org/43/configuration/topic-configs/)

## Änderbarkeit

Dies ist der angenommene Implementierungsstand. Partitionenzahl, Retention,
Schema-Registry, Topic-Aufteilung, Metrikexport und der spätere Mehrhostbetrieb
werden mit Lastprofil und Betriebsreife verfeinert; Änderungen vorbehalten.
Unverändert bleiben PostgreSQL als Wahrheit, atomare Outbox-/Audit-Erzeugung,
minimierter Export, stabile IDs und die Trennung von Transport-ACL und
Plattform-Policy.
