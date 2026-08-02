# ADR-025 – Globale Service- und Provider-Registry

**Status:** Angenommen
**Datum:** 2026-07-22

## Kontext

Identity, Kafka, TLS-Material und das Dokument-/Storage-Fundament verwenden
bereits providerneutrale Grenzen, besitzen aber noch keinen gemeinsamen
Metadatenvertrag für verfügbare Dienste und Adapter. Ohne eine kleine globale
Basis würden spätere Key-, Secret-, Certificate-, Storage-, Signatur- oder
Notification-Provider jeweils eigene Lifecycle-, Capability- und
Tenantkonzepte erfinden.

Eine universelle Providerkonfiguration wäre zugleich eine gefährliche
Abkürzung. Identity-Bindings, Storage-Locations, Schlüsselmaterial,
Betriebszustand und Mehrinstanz-Schreibhoheit haben unterschiedliche
Datenowner und Sicherheitsregeln. Die Registry darf diese Grenzen weder
zusammenziehen noch aus einem Status automatisch Berechtigungen ableiten.

## Entscheidung

### Deklarativer Metadatenkatalog

PostgreSQL ist die autoritative Wahrheit für vier getrennte Verträge:

1. `ServiceContract` beschreibt einen versionierten logischen Dienst und sein
   besitzendes Plattformmodul.
2. `CapabilityContract` beschreibt eine versionierte technische Fähigkeit
   dieses Dienstes und ihre Installations- oder Tenant-Operationsgrenze.
3. `ProviderRegistration` beschreibt genau eine logische, service-spezifische
   Providerinstanz und den im Code bekannten Adapter.
4. `ProviderCapabilityBinding` erlaubt dieser Providerinstanz eine
   ausdrücklich registrierte Capability-Version.

Ein physisches Produkt kann mehrere getrennte Providerregistrierungen besitzen.
Ein externer Secret Store wird beispielsweise nicht automatisch zu einem
gemeinsamen Secret-, Key- und Certificate-Superprovider. Die getrennten
Registrierungen erhalten eigene Capabilities und können unabhängig deaktiviert
oder ersetzt werden.

### Explizite Auflösung

Ein Verbraucher fordert immer eine konkrete Provider-ID,
Registry-Vertragsversion, Dienstversion, Capability-Version, Operationsgrenze
und gegebenenfalls Tenant-ID an. Der Core wählt niemals implizit den ersten
aktiven oder vermeintlich gesunden Provider. Die Auflösung schlägt geschlossen
fehl, wenn Dienst, Capability, Provider, Binding, Version oder Grenze nicht
exakt übereinstimmen beziehungsweise nicht aktiv sind. Ihr Ergebnis bindet
Provider, Adapter, Dienst, Capability und den unveränderten Tenant-Kontext der
Operation gemeinsam; nur die Providerregistrierung zurückzugeben wäre für
tenantweite Operationen über eine installationsweite Konfiguration zu
mehrdeutig.

`ConfigScope` und `OperationBoundary` bleiben getrennte Achsen:

- `ConfigScope=installation` bedeutet, dass dieselbe Providerkonfiguration für
  mehrere Tenantoperationen verwendet werden kann. Der Tenant-Kontext der
  konkreten Operation bleibt trotzdem zwingend erhalten.
- `ConfigScope=tenant` bindet die Providerinstanz an genau einen Tenant. Eine
  Operation muss denselben Tenant tragen.
- Eine installationsweite Operation besitzt keinen Tenant-Kontext und kann
  nicht über einen tenantgebundenen Provider ausgeführt werden.

Die Datenbank lehnt deshalb auch eine Bindung zwischen einer
installationsgebundenen Capability und einem tenantkonfigurierten Provider ab;
ein solcher Vertrag wäre in jeder möglichen Anfrage unauflösbar.

### Lifecycle und Nebenläufigkeit

Dienst, Capability, Provider und Binding verwenden
`active | disabled | retired`. Neue Provider beginnen `disabled`; `active`
bedeutet nur, dass der Betreiber sie ausdrücklich auswählbar gemacht hat. Es
ist keine Health-, Readiness- oder Authority-Aussage. `retired` ist terminal;
historische Referenzen bleiben erhalten. Provider besitzen eine positive,
optimistisch zu prüfende Revision und eine unveränderliche Version des
Registry-Vertrags. Eine andere Registry-Vertragsversion benötigt eine neue
Providerregistrierung mit eigener ID; derselbe stabile Provider-Key darf über
Registry-Vertragsversionen hinweg bestehen bleiben. Eine Runtime löst nur die
von ihrem Code ausdrücklich unterstützten Registry-Vertragsversionen auf;
unbekannte zukünftige Versionen schlagen geschlossen fehl.

Der erste Schnitt ist migrations- und ownerverwaltet. Laufzeitrollen erhalten
nur die jeweils erforderliche nichtgeheime Lesesicht. Es gibt noch keine
öffentliche Verwaltungs-API. Wenn Laufzeitmutationen hinzukommen, müssen
Änderung, Business-/Security-Audit und Outbox atomar gespeichert sowie durch
Versionsprüfung gegen verlorene Updates geschützt werden.

### Datenminimierung und Datenhoheit

Die Registry speichert ausschließlich stabile IDs, Keys, Versionen, Scope,
Tenantbindung und Lifecycle. Verboten sind insbesondere:

- Credentials, Tokens und Secretwerte,
- private oder öffentliche Schlüssel und Zertifikatsinhalte,
- Endpunkte, Bucket-/Objektpfade und freie JSON-Konfiguration,
- Identity-Subjects und Kontobindungen,
- Betriebs-Health, Latenzen oder Fehlerraten.

Identity behält Hoheit über `identity_providers` und Subject-Bindings. Storage
behält Hoheit über Blobs und opake Locations. Kafka-Outbox, TLS-Dateien und
MFA-Keyring werden in diesem Schritt nicht nachträglich als vollständig
registrierte Provider ausgegeben. Die erste echte Integration erfolgt später
additiv in genau einer Domäne.

### Runtime- und Mehrinstanzgrenze

Konkrete Adapter werden weiterhin typisiert an der Composition Root des
jeweiligen Dienstes zusammengesetzt. Es entsteht keine globale `any`-Map, keine
dynamische Go-Plugin-ABI und keine Möglichkeit, fachfremde Adapter durch bloße
Metadaten zu verwenden.

Bei gemeinsamem PostgreSQL serialisiert die Datenbank Registryänderungen. Bei
getrennten Datenbankkopien erteilt eine Registryzeile weder Schreibhoheit noch
Failoverrecht. Eine spätere Authority-Domain benötigt weiterhin Lease,
Generation und technisches Fencing. Health darf niemals allein eine Promotion
oder einen automatischen Providerwechsel auslösen.

## Bewusste Nicht-Ziele

- keine Admin- oder Fach-API im ersten Schnitt,
- keine automatische Providerwahl, Fallbacks oder Lastverteilung,
- kein Health-, Retry- oder Circuit-Breaker-System,
- kein allgemeines Konfigurations-, Secret- oder PKI-System,
- keine sofortige Umstellung bestehender Identity- oder Storage-Tabellen,
- keine Berechtigung durch Capability-Besitz; Capabilities sind keine RBAC-
  Permissions oder App-Entitlements,
- keine Witness-, Lease- oder Fencing-Implementierung.

## Folgen

Neue Backenddienste erhalten eine gemeinsame, kleine Registrierungsgrammatik,
ohne ihre Fachhoheit oder konkrete Adaptertypen zu verlieren. Der nächste
Schritt kann Certificate-, Key- und Secret-Provider auf diese Metadatenbasis
setzen; danach folgt genau ein realer Storage- oder Event-Verbraucher. Vor einer
produktiven Aktivierung bleiben datenbankgestützte RLS-, Lifecycle- und
Parallelitätstests verpflichtend; Änderungen vorbehalten.
