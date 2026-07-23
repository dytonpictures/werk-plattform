# ADR-021 – Interner Dokument-, Blob- und Transfervertrag

**Status:** Angenommen  
**Datum:** 2026-07-22

## Kontext

Die Plattform benötigt einen eigenen Dokument- und Speicherkern. Er soll die
tragfähigen Funktionen moderner Datei- und Kollaborationssysteme bereitstellen,
ohne deren Quellcode, Datenmodell oder Produktgrenzen zu übernehmen. Dokumente,
unveränderliche Versionen, veränderliche Arbeitskopien und physische Bytes sind
verschiedene Verantwortungen. Werden sie als ein einziges Objekt behandelt,
entstehen parallele Berechtigungswelten, nicht prüfbare Uploadzustände und eine
enge Bindung an einen konkreten Storage-Provider.

[`ADR-007`](ADR-007-object-storage-und-dokumente.md) legt PostgreSQL als
Metadatenwahrheit und S3-kompatiblen Object Storage als Byte-Speicher fest. Für
eine Implementierung fehlen dort noch die Grenzen zwischen Core Documents,
Core Storage, Core Identity, Plattform-Policy, Audit und einem späteren
Collaboration-/Sync-Dienst. Dieses ADR konkretisiert ADR-007, ersetzt dessen
Grundentscheidung aber nicht.

## Entscheidung

### Zwei logische Core-Dienste, zunächst ein Deployment

`Core Documents` und `Core Storage` werden als getrennte Bounded Contexts im
modularen Go-Monolithen umgesetzt. Sie laufen zunächst in denselben API- und
Worker-Artefakten und verwenden dieselbe autoritative PostgreSQL-Datenbank.
Eine spätere Prozess- oder Datenbanktrennung ist kein Bestandteil dieses
Ausbauschnitts.

```text
Core Identity und Plattform-Policy
                |
                v
Core Documents  --->  Core Storage  --->  S3-kompatibler Byte-Provider
                |
                v
später: Collaboration / Sync
```

Die Verantwortungen sind verbindlich getrennt:

| Bereich | Besitzt | Besitzt nicht |
|---|---|---|
| Core Documents | Dokument, veröffentlichte Version, Klassifikationshistorie, Retention-Zuordnung, fachliche Sichtbarkeit und Ressourcenbezug | Provider-Credentials, Objektpfade, Uploadteile und technische Garbage Collection |
| Core Storage | Blob, Blob-Location, serverseitigen Hash, Quarantäne, Transferzustand, Ticket-Digest und Provideradapter | Dokumenttitel, fachliche Dokumentrollen und eine eigene Benutzer-ACL |
| Collaboration/Sync | später Arbeitskopien, Revisionen, Sync-Cursor und Konflikte | kanonische Dokumentversionen, eigene Identity und dauerhafte Blob-Hoheit |
| Object Store | Bytes unter opaken Schlüsseln | Berechtigungen, Tenant-Policy, Dokumentstatus und Retention-Entscheidung |

Tabellen bleiben modulprivat. Die Bindung einer veröffentlichten
`DocumentVersion` an einen versiegelten `Blob` ist ein ausdrücklich versionierter
interner Vertrag. Sie wird über das Application-API des besitzenden Core-Moduls
geschrieben und kann im gemeinsamen Deployment an derselben Tenant-Transaktion
teilnehmen. Fachapps schreiben niemals direkt in diese Tabellen.

### Tenant und Datenhoheit

Jede Dokument-, Versions-, Klassifikations-, Blob-, Location-, Transfer- und
Binding-Zeile trägt `tenant_id`. Stabile IDs werden zusätzlich mit
`UNIQUE (tenant_id, id)` abgesichert. Beziehungen über diese Grenze verwenden
zusammengesetzte Tenant-Fremdschlüssel. `FORCE ROW LEVEL SECURITY`, ein
servergesetzter Transaktions-Tenant und explizite Runtime-Grants bleiben auch
für interne Service- und Workerzugriffe verpflichtend.

Ein Blob darf innerhalb eines Tenants von mehreren unveränderlichen Versionen
referenziert werden. Eine Deduplizierung über Tenantgrenzen ist verboten. Ein
global gleicher Hash erzeugt weder Wiederverwendung noch einen für einen
anderen Tenant beobachtbaren Treffer.

### Dokumente und veröffentlichte Versionen

Ein Dokument wird fachlich erst sichtbar, wenn mindestens Version 1 erfolgreich
veröffentlicht wurde. Eine veröffentlichte `DocumentVersion` ist INSERT-only.
Titel- oder Inhaltskorrekturen erzeugen eine neue Version beziehungsweise eine
explizite Metadatenänderung mit optimistischer Nebenläufigkeitskontrolle; eine
vorhandene Version wird nicht überschrieben.

Der Dokument-Core speichert keine Providerpfade. Eine Version referenziert nur
eine tenantgebundene `BlobID`. Größe, tatsächlich erkannter Medientyp und
SHA-256-Digest sind Eigenschaften des versiegelten Blobs. API-Projektionen
dürfen diese Werte anzeigen, erzeugen daraus aber keine zweite Wahrheit.

Klassifikation, Retention-Klasse, Retention-Zeitpunkt und Legal-Hold-Zustand
werden historisiert. Eine Änderung ersetzt keinen früheren Nachweis. Solange der
vollständige Retention-/Legal-Hold-Vertrag noch nicht implementiert ist, gibt es
keine physische Löschfunktion für veröffentlichte Blobs.

### Blob, Location und Quarantäne

Ein Blob beschreibt den geprüften Inhalt unabhängig von seinem physischen Ort.
Eine oder mehrere `BlobLocation`s ordnen ihn einem Provider und einem opaken
Schlüssel zu. Objektpfade werden ausschließlich serverseitig aus zufälligen
technischen IDs erzeugt und enthalten keine Dokumenttitel, Dateinamen,
Login-Namen oder Fachschlüssel.

Der erste Zustandsautomat lautet:

```text
quarantined -> available
            -> rejected

available   -> unknown  -> available
    |            |
    +------------+-> missing -> available     // erst nach verifizierter Reparatur

available   -> deletion-pending -> deleted    // erst nach späterer Retention-Freigabe
```

`unknown` ist kein Verlustnachweis, sondern das fail-closed Ergebnis einer
nicht entscheidbaren Providerprüfung. `missing` wird erst gesetzt, wenn keine
nutzbare Location mehr bestätigt ist. Beide Zustände verhindern die Ausgabe
von Bytes. Digest, Größe, Medientyp und ursprünglicher Verifikationszeitpunkt
bleiben unverändert; eine Rückkehr zu `available` erfordert eine erneut
verifizierte oder reparierte Location.

Nur der Server berechnet den kanonischen SHA-256-Digest und die Größe über die
tatsächlich gespeicherten Bytes. Ein Client-Hash darf lediglich ein erwarteter
Transportwert sein. Provider-ETags sind kein kryptografischer Inhaltsnachweis.
Nach der Versiegelung sind Inhalt, Digest, Größe, Medientyp und die technische
Identität einer Location unveränderlich. Ein fehlendes oder nicht prüfbares
physisches Objekt führt fail-closed zu `missing` beziehungsweise `unknown`;
PostgreSQL darf nicht vortäuschen, die Bytes seien vorhanden.

### Autorisierung ohne Policy-Monster

Core Identity bestätigt Actor, Kontoart, Access Plane, Tenant und
Authentifizierungsstärke. Die Plattform-Policy prüft die unverhandelbaren
Grenzen und die registrierte Dokumentressource. Danach entscheidet Core
Documents seine lokalen Regeln, etwa fachliche Verknüpfung, Klassifikation,
Dokumentstatus oder eine konkrete Einzelfreigabe.

Core Storage interpretiert keine Work-Rollen. Es akzeptiert ausschließlich eine
bereits autorisierte, exakt begrenzte interne Operation. Eine Policy-Prüfung
wird nicht für jeden Byteblock wiederholt. Stattdessen erhält der Transfer eine
kurzlebige Capability; vor Veröffentlichung beziehungsweise Downloadbeginn wird
der aktuelle Dokumentzustand erneut geprüft.

`admin` darf Providerkonfiguration und Betriebszustand verwalten, aber keine
Tenant-Dokumente oder Blobs lesen. `service`, `worker` und `agent` handeln als
eigene technische Principals und vertreten keinen Work-Benutzer implizit.

### Delegation und Audit-Zuordnung

Eine spätere Prozessgrenze verwendet eine begrenzte `DelegatedOperation`:

```text
tenant_id
initiated_by            // Work-Actor oder technischer Auslöser
executed_by             // authentifizierter Service-Principal
source_resource_ref
storage_operation
constraints
policy_decision_ref
processing_context_ref
correlation_id
expires_at
nonce / max_uses
```

Das ist keine Benutzervertretung und kein Generalschlüssel. Audit hält
`initiated_by` und `executed_by` getrennt fest. Es protokolliert Ressource,
Aktion, Ergebnis, Policy-/Processing-Kontext und Correlation-ID, aber niemals
Ticket-Rohwerte, Provider-Credentials, Objektpfade oder unnötige Dateinamen.
Der vorhandene reine Security-Audit-Vertrag wird vor einer öffentlichen
Dokumentmutation um diesen fachlichen Actor-/Subject-Vertrag ergänzt.

Die Erweiterung verwendet denselben autoritativen Core-Auditbestand und keine
zweite Dokument-Auditwelt. Der Identity-Domänentyp bleibt absichtlich auf
Identity-Ereignisse begrenzt; fachliche Module verwenden einen eigenen
providerunabhängigen Core-Auditvertrag. Der erste Producer-Schnitt ist
tenantgebunden und requestgetrieben. Hintergrundaktionen werden erst nach einer
expliziten Operation-ID-Regel zugelassen, damit keine erfundenen HTTP-Request-
IDs im Nachweis entstehen.

`executed_by` wird beim Persistieren aus dem bereits authentifizierten,
tenantgebundenen Workload-Actor eingesetzt und nicht aus Command- oder
Auditdaten übernommen. Ein versionierter Action-Vertrag koppelt Ereignistyp und
Aktion an Permission und Ressourcenart. Bestehende Identity-/Admin-Produzenten
bleiben auf ihren Legacy-Satz begrenzt und können den strukturierten
Dokument-/Storage-Pfad nicht über eine ältere RLS-Policy umgehen.

### Transfer-Tickets

Der erste produktive Transfer läuft über einen Backend-Transferendpunkt. Der
Client erhält keinen Bucket, Provider-Key und keine dauerhaften
Storage-Credentials. Direkte vorsignierte Provider-URLs sind eine spätere
Skalierungsoption und benötigen eine eigene Sicherheits- und Clientgrenzen-
Präzisierung.

Ein Ticket ist:

- opaque, zufällig und kurzlebig;
- in PostgreSQL ausschließlich als kryptografischer Digest gespeichert;
- an Tenant, Actor, Richtung, HTTP-Methode, Transfer, Blob und Dokumentressource
  gebunden;
- durch Maximalgröße, erwarteten Medientyp und optionalen erwarteten Hash
  begrenzt;
- einmalig verbrauchbar, widerrufbar und atomar gegen Replay geschützt;
- bei Ausstellung und bei Einlösung beziehungsweise Finalisierung erneut gegen
  Tenant-, Actor- und Ressourcenzustand geprüft.

Ticket und Token-Digest erscheinen nie in Domain-Events, Kafka-Nachrichten oder
Betriebslogs.

### Veröffentlichungsablauf

```text
1. Work-Actor fordert einen Upload für die Dokument-Collection oder ein Dokument an
2. Documents prüft Plattform-Policy und dokumentlokale Regeln
3. Storage erzeugt Transfer, Quarantäne-Blob, Location und Einmalticket
4. Backend streamt Bytes in den konfigurierten Provider
5. Server prüft Größe, erkannten Medientyp, SHA-256 und spätere Scan-Policies
6. Storage versiegelt den Blob als available
7. Documents prüft Actor, Ressource und Zustand erneut
8. Dokument/Version, Klassifikation, Blob-Bindung, Audit und Outbox werden atomar gespeichert
9. Erst danach ist die Version über die Business-API sichtbar
```

Ein Provider- oder Datenbankfehler hinterlässt höchstens einen unsichtbaren
Quarantäne-Blob. Nur abgelaufene, nie veröffentlichte und nachweislich
ungebundene Quarantäneobjekte dürfen im ersten Produktstand bereinigt werden.

### Ereignisse und Kafka

Dokumentereignisse verwenden typisierte, versionierte Payloads. Sie enthalten
nur stabile Ressourcen-IDs, fachlichen Status und notwendige Governance-Tags.
Object-Key, Ticket, Token-Digest, Hash und frei übernommene Dateinamen werden
nicht veröffentlicht. PostgreSQL-Outbox und Fachänderung bleiben atomar; Kafka
ist ausschließlich Distributionspfad.

### Backup und Restore

Ein PostgreSQL-Dump ist kein Dokumentenbackup. Vor produktiver Speicherung von
Bytes benötigt das Betriebsprofil einen koordinierten, verschlüsselten
PostgreSQL-/Object-Store-Sicherungsschnitt, ein Objektmanifest und einen
automatisierten Reconciliation-/Restore-Test. Ein Restore meldet fehlende oder
zusätzliche Objekte und schaltet betroffene Versionen nicht stillschweigend
frei.

### Collaboration und Sync folgen später

Collaboration besitzt veränderliche `WorkingCopy`s mit `base_version_id`,
Revisionen und Konfliktzustand. Eine akzeptierte Arbeitskopie wird ausschließlich
über Core Documents als neue unveränderliche Version veröffentlicht. Shares
erzeugen keine parallele ACL-Welt. Content-defined Chunking, Delta-Sync, CRDT,
Live-Coauthoring und automatische Konfliktzusammenführung sind nicht Teil des
ersten Dokument-/Storage-Schnitts.

## Erster Implementierungsschnitt

Der erste Schnitt umfasst ausschließlich:

- die logischen Core-Verträge für Dokument, veröffentlichte Version, Blob und
  Blob-Location;
- Registrierungen für `core.documents` und `core.storage`, Dokumentressourcen,
  Work-Berechtigungen, Datenprofile und Processing-Policies;
- tenantgesicherte PostgreSQL-Tabellen mit zusammengesetzten Fremdschlüsseln,
  RLS und DB-seitiger Unveränderlichkeit veröffentlichter Versionen und
  verfügbarer Blobinhalte;
- Unit- und Migration-/RLS-Tests.

Er enthält noch keinen öffentlichen Upload-/Download-Endpunkt, keinen
S3-Adapter, keine physische Löschung, keine Deduplizierung und keinen
Collaboration-/Sync-Dienst. Diese Begrenzung verhindert eine scheinbar fertige
Dateifunktion ohne Ticket-, Audit- und Restore-Nachweis.

Als nachgelagerter kleiner Fundamentschnitt ist der fachliche Auditvertrag mit
dualen Actors, kanonischem Subject, Ergebnis sowie servergeprüftem Policy- und
Processing-Snapshot umgesetzt. Er ist noch an keine öffentliche
Dokumentmutation angeschlossen; Änderungen vorbehalten.

## Folgen

- Die Plattform erhält eine eigene, providerunabhängige Dokument- und
  Speicherarchitektur.
- Identity bleibt Identitätsquelle; Documents bleibt fachliche
  Autorisierungsinstanz; Storage bleibt technischer Byte-Dienst.
- Versionen und Blobs lassen sich später in getrennte Prozesse verschieben,
  ohne Fachapps an Providerpfade zu koppeln.
- Ein produktiver Bytepfad benötigt vor Freigabe zusätzliche Audit-, Transfer-,
  Provider- und Restore-Arbeit.
- Seafile oder ähnliche Systeme können später nur über ausdrücklich gerichtete
  Import-/Collaboration-Adapter angebunden werden und sind keine Grundlage des
  Core.

## Änderbarkeit

Providerwahl, Ticket-TTL, Transferprotokoll, Scan-Pipeline, tenantinterne
Deduplizierung und der spätere Sync-Algorithmus werden mit realen Last- und
Bedrohungsprofilen verfeinert; Änderungen vorbehalten. Unverändert bleiben die
Tenant-Grenze, serverseitige Autorisierung, opake Storage-Schlüssel,
unveränderliche veröffentlichte Versionen, duale Audit-Zuordnung und das Verbot
einer parallelen Storage-ACL.
