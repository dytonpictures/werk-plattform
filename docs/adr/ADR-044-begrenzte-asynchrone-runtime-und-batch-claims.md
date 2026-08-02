# ADR-044 – Begrenzte asynchrone Runtime, Batch-Claims und Wecksignale

**Status:** Angenommen
**Datum:** 2026-08-02

## Kontext

Outbox- und Security-Audit-Export fragen PostgreSQL auch im Leerlauf periodisch
ab und claimen je Transaktion genau einen Eintrag. Das ist korrekt, erzeugt bei
Rückstau jedoch zusätzliche Roundtrips. Mehrere Worker können ihre Abfragen
außerdem zeitlich bündeln. Der verlusttolerante Runtime-Log-Puffer besitzt
bisher nur eine indirekte Speichergrenze über Anzahl und Maximalgröße.

Fachliche Änderung, Outbox und Security-Audit bleiben atomare
PostgreSQL-Wahrheit. Ein flüchtiges Signal oder eine noch nicht bestätigte
Kafka-Veröffentlichung darf nie als dauerhafter Zustellerfolg gelten.

## Entscheidung

### Batch-Claims

Outbox und Security-Audit dürfen in einer Transaktion eine kleine, fest
begrenzte Menge zustellbarer Einträge claimen. Ein Outbox-Batch enthält nur den
jeweils ältesten offenen Eintrag einer Tenant-/Partitionskombination. Für Audit
gilt dieselbe Reihenfolgenregel je Tenant einschließlich der
installationsweiten `NULL`-Partition. `FOR UPDATE SKIP LOCKED`, individuelle
Leases, Attempts und stabile IDs bleiben verbindlich.

Jeder Eintrag wird einzeln veröffentlicht und einzeln als `completed`, `retry`
oder `dead` überführt. Ein Teilfehler setzt niemals den gesamten Batch auf
Erfolg. Nicht begonnene Einträge werden beim geordneten Shutdown nach
Möglichkeit freigegeben und bleiben spätestens nach Leaseablauf wieder claimbar.

### Kafka-Parallelität

Durable Domain-Events und Security-Audits gelten erst nach dem
Broker-Acknowledgement als veröffentlicht. Begrenzte Parallelität entsteht
durch konfigurierte Worker-Slots und kleine Claims, nicht durch vorzeitige
Receipts. Eine asynchrone Kafka-API darf intern verwendet werden, wenn der Slot
vor der PostgreSQL-Erfolgsmarkierung auf sein Ergebnis wartet und die Anzahl
ausstehender Records hart begrenzt bleibt.

Runtime-Logs dürfen über ihren begrenzten In-Memory-Puffer asynchron gespiegelt
und bei Überlast gezählt verworfen werden. Ihre Queue wird nach Bytes und
Einträgen begrenzt. Lokales Logging bleibt der unabhängige Diagnosepfad.

### PostgreSQL-Wecksignale

Neue Outbox- und Security-Audit-Einträge dürfen nach erfolgreicher Transaktion
ein payloadloses PostgreSQL-`NOTIFY` auslösen. Das Signal enthält weder Tenant,
Eventtyp, ID noch andere Fach- oder Personendaten. Worker verwenden es nur, um
einen wartenden Poll früher auszuführen. Adaptives Polling bleibt Fallback, weil
`NOTIFY` flüchtig ist und bei Verbindungsabbruch verloren gehen kann.

Leerlaufwartezeiten erhalten einen kleinen pro Slot abgeleiteten Jitter, damit
mehrere Prozesse PostgreSQL nicht regelmäßig gleichzeitig abfragen.

### Beobachtung und Grenzen

Poolzustand, Claimdauer und -anzahl, Publishdauer, Queue-Einträge und Queue-Bytes
werden als niedrig kardinale Prozessmetriken bereitgestellt. Datenbankpools
erhalten validierte, pro Prozessrolle konfigurierbare Obergrenzen sowie
begrenzte Idle- und Lebenszeiten. Zugangsdaten, SQL, Topics, Tenant-IDs,
Eventtypen und Fehlertexte erscheinen nicht als Metriklabels.

## Konsequenzen

- Leerlauf und Rückstau benötigen weniger Datenbank-Roundtrips.
- Speicher- und Connection-Verbrauch werden explizit begrenzt und beobachtbar.
- Latenz bleibt durch Wecksignal und Poll-Fallback begrenzt.
- Batch-Teilfehler, Leaseablauf und Shutdown benötigen ausdrücklich geprüfte
  Übergänge.
- Optimierungen dürfen At-least-once nicht als Exactly-once darstellen.

## Nicht entschieden

- Brokertransaktionen über mehrere Topics.
- Eine fachliche Queue in Valkey oder ein Ersatz von PostgreSQL als Wahrheit.
- Automatisch lastabhängige Pool- oder Batchgrößen.
