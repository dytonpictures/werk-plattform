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

## Vertrag für Consumer

- Jeder Consumer besitzt einen stabilen, global eindeutigen Schlüssel.
- Ein Handler läuft innerhalb einer expliziten Tenant-Transaktion.
- Wirkung und Consumer-Receipt werden gemeinsam committed.
- Wiederholte Zustellung desselben Events wird anhand von Event-ID und
  Consumer-Schlüssel übersprungen.
- Externe Nebenwirkungen benötigen zusätzlich deren eigene Idempotency Keys.

## Fehlerverhalten

Leases laufen nach einem Prozessabsturz ab. Retries verwenden exponentielles
Backoff bis maximal fünf Minuten. Nach `max_attempts` bleibt das Ereignis als
Dead Letter in PostgreSQL erhalten. Fehlertexte sind begrenzt und dürfen keine
Secrets enthalten.

## Skalierung

`WERK_WORKER_CONCURRENCY` begrenzt die parallelen Slots eines Prozesses und ist
standardmäßig `4`. Mehrere Worker-Prozesse können durch `FOR UPDATE SKIP LOCKED`
zusammenarbeiten. Valkey kann später Claims beschleunigen, ist aber weder Quelle
der Ereignisse noch des Zustellstatus.
