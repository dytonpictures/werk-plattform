# Globale asynchrone Verarbeitung

WERK stellt eine fachneutrale Runtime für zuverlässige Hintergrundarbeit bereit.
Sie ist keine Fachanwendung und enthält selbst keine Dokument-, Fahrzeug-, CRM-
oder Workflowlogik.

## Ablauf

```text
fachliche Tenant-Transaktion
  -> Änderung speichern
  -> versioniertes Domain-Event per outbox.Enqueue speichern
  -> Commit

werk-worker
  -> ältestes fälliges Event einer Partition leasen
  -> registrierte Consumer in Tenant-Transaktionen ausführen
  -> Kafka-Consumer veröffentlicht platform.event-envelope.v1
  -> Consumer-Receipt atomar mit der Wirkung speichern
  -> Event abschließen oder mit Backoff erneut einplanen
  -> nach maximalen Versuchen als dead markieren
```

## Parallelität und Reihenfolge

Der Worker besitzt einen begrenzten Pool. Verschiedene Partitionen dürfen
parallel laufen. Innerhalb derselben Kombination aus Tenant und
`partition_key` wird das ältere offene Ereignis zuerst verarbeitet. Ein Modul
wählt beispielsweise `document:<id>`, `import:<id>` oder `workflow:<id>` als
Partition. Eine globale Sammelpartition wäre korrekt, würde aber Parallelität
unnötig reduzieren.

## Vertrag für Produzenten

- Event-Typen sind versioniert, beispielsweise `documents.file.created.v1`.
- Payloads sind JSON-Objekte und maximal 1 MiB groß.
- Ereignisse beschreiben Tatsachen, keine implizite Benutzervertretung.
- `outbox.Enqueue` muss dieselbe `database.TenantTx` wie die fachliche Änderung
  erhalten.
- Producer, Subject und Partition sind stabile technische Schlüssel.
- Jedes Ereignis trägt begrenzte String-Tags. `data.classification`,
  `processing.purpose` und `retention.class` sind verpflichtend; fehlen sie,
  ergänzt die Laufzeit konservative Standardwerte.
- Tags enthalten keine Secrets oder freien Personendaten und erteilen keine
  Berechtigungen.

## Vertrag für Consumer

- Jeder Consumer besitzt einen stabilen, global eindeutigen Schlüssel.
- Ein Handler läuft innerhalb einer expliziten Tenant-Transaktion.
- Wirkung und Consumer-Receipt werden gemeinsam committed.
- Wiederholte Zustellung desselben Events wird anhand von Event-ID und
  Consumer-Schlüssel übersprungen.
- Externe Nebenwirkungen benötigen zusätzlich deren eigene Idempotency Keys.

## Kafka-Export

Kafka ist als regulärer Infrastrukturpfad angebunden. Der globale Consumer
`platform.kafka.domain-events.v1` veröffentlicht jedes Domain-Event in
`platform.domain-events.v1`. Der Kafka-Schlüssel kombiniert Tenant und
fachliche Partition; damit bleibt die Reihenfolge innerhalb derselben
Tenant-Partition erhalten.

Das Envelope enthält Event-ID, Typ, Zeitpunkt, Tenant, Producer, Subject,
Partition, Correlation-/Causation-ID, Tags und Payload. Die Zustellung ist über
die PostgreSQL-/Kafka-Grenze at least once. Verbraucher deduplizieren anhand der
stabilen Event-ID. Eine bestätigte Veröffentlichung wird als Consumer-Receipt
gespeichert; Kafka-Ausfälle führen zu Retry und schließlich zu einem in
PostgreSQL sichtbaren Dead Letter.

Security-Audits verwenden eine eigene atomar befüllte Export-Queue und das
Topic `platform.security-audit.v1`. Freie Auditdetails und Session-IDs werden
nicht exportiert. Strukturierte Betriebslogs werden unabhängig und
verlusttolerant nach `platform.runtime-logs.v1` gespiegelt. Diese drei Pfade
dürfen weder Topic noch Retention oder ACLs teilen. Details stehen in
[`ADR-020`](adr/ADR-020-kafka-event-audit-und-log-streaming.md).
Die Ressourcen-, Batch- und Wecksignalgrenzen konkretisiert
[`ADR-044`](adr/ADR-044-begrenzte-asynchrone-runtime-und-batch-claims.md).

Die Topics werden betreiberseitig angelegt. Eine gemeinsame Metadatenabfrage des
Workers und von `werkctl doctor` beweist die Brokerverbindung, erlaubt keine
automatische Topic-Erzeugung und prüft vor der Verarbeitung, dass alle drei
Topics samt Partitionen erreichbar sind. Ein zusätzlicher Broker-Ping wäre für
diesen stärkeren Nachweis redundant.
Retention, Replikation und ACL-Inhalte bleiben Betreiberkonfiguration und
werden nicht durch zusätzliche Kafka-Adminrechte des Runtime-Principals
ausgelesen oder verändert.

## Fehlerverhalten

Leases laufen nach einem Prozessabsturz ab. Retries verwenden exponentielles
Backoff bis maximal fünf Minuten. Nach `max_attempts` bleibt das Ereignis als
Dead Letter in PostgreSQL erhalten. Fehlertexte sind begrenzt und dürfen keine
Secrets enthalten.

Jeder Slot claimt höchstens vier aktuell zustellbare Einträge in einer
Transaktion. Outbox-Batches enthalten weiterhin nur den ältesten offenen
Eintrag je Tenant-/Partition, Audit-Batches nur den ältesten je Tenant oder
Installationsstrom. Veröffentlichung und Statusübergang bleiben pro Eintrag.
Bei geordnetem Shutdown werden noch nicht begonnene Claims sofort samt
Attempt-Zähler freigegeben; nach einem Prozessabsturz übernimmt weiterhin der
Leaseablauf.

Der Runtime-Log-Puffer blockiert keine Fachtransaktion. Beim geordneten
Shutdown nimmt er keine neuen Einträge mehr an und leert bereits akzeptierte
Einträge bis zur vorgegebenen Frist. Kodierungs-, Puffer-, Publish- und
Shutdownverluste erhöhen `werk_kafka_runtime_logs_dropped_total`. Lokales
`stdout` behält Diagnosefehler; in der externen Kafka-Spiegelung werden
konventionelle `error`-Attribute sowie Credential-, Token-, Cookie-, Session-
und Secretfelder geschwärzt.

Der pro Prozess feste Runtime-Log-Puffer umfasst höchstens 2.048 Einträge und
gleichzeitig höchstens 64 MiB kodierte Payload. Damit bleiben kurze Bursts aus
üblichen kleinen Logs aufnahmefähig, während große Nachrichten den Speicher
nicht über das Byte-Budget hinaus belegen. Bei Überlast werden neue
Kafka-Spiegelungen verworfen und gezählt; der lokale Logpfad bleibt unberührt.

## Skalierung

`WERK_WORKER_CONCURRENCY` begrenzt die parallelen Slots eines Prozesses und ist
standardmäßig `4`. Mehrere Worker-Prozesse können durch `FOR UPDATE SKIP LOCKED`
zusammenarbeiten. Leere Outbox- und Audit-Slots erhöhen ihren Pollabstand
schrittweise von 500 Millisekunden auf höchstens zwei Sekunden und wechseln nach
einem erfolgreichen Claim wieder auf den kurzen Abstand. Ein stabiler Jitter
von höchstens zehn Prozent verteilt die Slots und Prozesse zeitlich. Ein
payloadloses PostgreSQL-`NOTIFY` weckt zusätzlich einen wartenden Slot, enthält
aber keine ID, keinen Tenant und keine Fachdaten. Verbindliches Polling bleibt
bei verlorenen Signalen oder Listener-Ausfall aktiv. Valkey kann später Claims
beschleunigen, ist aber weder Quelle der Ereignisse noch des Zustellstatus.

Topic-Aufbewahrung ist kein fachliches Archiv. PostgreSQL bleibt Quelle für
Wiederanlauf, Auditnachweis und Dead-Letter-Zustand. Partitionierung, Retention
und Clustergröße können mit realen Lastwerten angepasst werden; Änderungen
vorbehalten.
