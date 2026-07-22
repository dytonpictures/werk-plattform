# WERK – Dokument-, Storage- und Collaboration-Plan

**Stand:** 2026-07-22  
**Status:** Ausführbarer Ausbauplan auf Basis von
[`ADR-021`](adr/ADR-021-interner-dokument-blob-und-transfervertrag.md)

## Ziel

WERK erhält einen eigenen internen Dokument- und Storage-Dienst. „Dienst“
bedeutet zunächst eine klar getrennte Domäne im modularen Monolithen, nicht
sofort einen zusätzlichen Microservice. PostgreSQL bleibt die Wahrheit über
Dokumente, Zustände, Bindungen, Hashes und Tenant-Zuordnung; ein
S3-kompatibler Provider hält später ausschließlich die Bytes.

```text
Identity
  -> Plattform-Policy
     -> dokumentlokale Regel
        -> Core Documents
           -> begrenzte Storage-Operation
              -> Core Storage
                 -> Byte-Provider
```

Die Plattform-Policy ist dabei das gemeinsame äußere Gate. Sie entscheidet
nicht jeden Byteblock und übernimmt keine Dokumentfachlogik. Für einen bereits
autorisierten Transfer wird eine kurzlebige, eng gebundene Capability
ausgestellt; Veröffentlichung und Downloadbeginn prüfen den aktuellen Zustand
erneut.

## Kanonische Objekte

### Core Documents

```text
Document
  id, tenant_id, title, status, source_module,
  created_by, created_at, updated_at, version

DocumentVersion
  id, tenant_id, document_id, version_number,
  blob_id, source, created_by, published_at

DocumentClassificationRevision
  id, tenant_id, document_id, revision,
  classification, retention_class, retention_until,
  legal_hold, recorded_by, recorded_at
```

Eine `DocumentVersion` existiert nur als veröffentlichte, unveränderliche
Version. Ein neues Dokument wird erst gemeinsam mit Version 1 sichtbar.
`source_module` beschreibt den fachlichen Erzeuger; Core Documents bleibt
unabhängig davon Datenowner.

`core.documents.collection` ist im Fundament eine virtuelle, tenantgebundene
Root-Ressource für die spätere Erstellprüfung (`id = root`) und noch keine
eigene persistente Ordner- oder Bibliotheksstruktur. Ein Erstellrecht auf dieser
Ressource gewährt weder Lesen noch Download vorhandener Dokumente.

### Core Storage

```text
Blob
  id, tenant_id, state,
  size_bytes, sha256, media_type,
  created_by, created_at, verified_at

BlobLocation
  id, tenant_id, blob_id,
  provider_key, opaque_key, state,
  provider_checksum, created_at

Transfer                         // nächster Schnitt
  id, tenant_id, direction, state,
  actor, target_ref, blob_id,
  expected_size, expected_media_type, expected_sha256,
  token_digest, expires_at, consumed_at
```

Ein Blob kann innerhalb desselben Tenants mehrere Versionen versorgen und
später mehrere Locations besitzen. Hashsuche und Deduplizierung werden nie
tenantübergreifend ausgeführt.

Die Basis unterscheidet Fehlerzustände ausdrücklich: `unknown` bedeutet, dass
eine Providerprüfung wegen eines technischen Fehlers kein belastbares Ergebnis
liefern konnte; `missing` bedeutet, dass kein nutzbarer physischer Speicherort
mehr bestätigt ist. Beide Zustände sperren Inhaltszugriffe fail-closed. Der
versiegelte Hash, die Größe und der Medientyp bleiben für Reconciliation und
Restore erhalten. Erst eine erneut verifizierte oder reparierte Location darf
den Blob wieder auf `available` setzen.

### Collaboration/Sync – später

```text
WorkingCopy
  id, tenant_id, document_id, base_version_id,
  state, owner, current_revision, updated_at

WorkingCopyRevision
  id, working_copy_id, revision, blob_id,
  created_by, created_at

SyncCursor / Conflict
  client, base_revision, server_revision, state
```

Eine Arbeitskopie ist veränderlich. Erst das ausdrückliche Veröffentlichen
erzeugt eine kanonische `DocumentVersion`. Providerfreigaben ersetzen niemals
die Plattform- und Dokumentberechtigung.

## Umsetzungsstufen

### Stufe A – Vertrag und persistentes Fundament

- ADR, Datenhoheit und Zustände festlegen.
- Core-Module, Ressourcen, Permissions, Datenprofile und Processing-Policies
  registrieren.
- Dokument-, Versions-, Klassifikations-, Blob- und Locationtabellen mit
  zusammengesetzten Tenant-Fremdschlüsseln und `FORCE RLS` anlegen.
- veröffentlichte Versionen und verfügbare Blobinhalte DB-seitig unveränderlich
  machen.
- noch keine öffentliche Dateischnittstelle anbieten.

**Umsetzungsstand 2026-07-22:** Stufe A ist als inaktive Plattformbasis
implementiert. Domänenmodelle, Registrierungen, tenantgesicherte Tabellen,
zusammengesetzte Fremdschlüssel, monotone Versionen, Blob-Zustandsübergänge und
DB-seitige Unveränderlichkeit sind durch Unit-, Migrations- und
Zwei-Tenant-Integrationstests für Work-, Service- und Worker-Runtime sowie
parallele Location-Ausfälle geprüft. Es gibt weiterhin weder einen öffentlichen
Bytepfad noch einen Provideradapter, Transfer-Tickets, Collaboration oder
physische Löschung; Änderungen vorbehalten.

### Fachliche Auditbasis – umgesetzt, noch ohne Dokument-Producer

Der bestehende autoritative Core-Auditdatensatz wurde additiv erweitert; es
entsteht keine zweite Audit-Tabelle nur für Dokumente. Ein fachlicher Eintrag
enthält strukturiert:

```text
initiated_by              // Work-, Service- oder Agent-Konto
executed_by               // immer authentifizierter Service oder Agent
subject_ref               // tenantgebundene Plattformressource
action + outcome
permission + policy_contract_version
processing_required + ProcessingContext
request_id + correlation_id
```

PostgreSQL prüft Tenant und beide Konten, die Service-/Agent-Kontoart des
Executors sowie den Policy-/Processing-Snapshot gegen die aktive serverseitige
Policy. Event, Aktion, Permission und Ressourcenart sind zusätzlich durch einen
versionierten serverseitigen Action-Vertrag fest gekoppelt. Eine semantische
Änderung benötigt eine neue Event-/Action-Version und darf alte Auditzeilen
nicht umdeuten. Der bestehende Vertrag ist unveränderlich und darf nur von
`active` auf `retired` wechseln.

Der Audit-Store übernimmt `executed_by` ausschließlich aus einem bereits
authentifizierten, tenantgebundenen Workload-Actor; ein Command oder frei
zusammengesetzter Audit-Eintrag kann keinen anderen Executor behaupten. Die
Service-Runtime darf nur tenantgebundene Dokument-/Storage-Einträge anfügen und
den Auditbestand nicht lesen oder verändern. Alte Identity- und Admin-Policies
sind auf unstrukturierte Legacy-Einträge begrenzt. Titel, Dateinamen, Hashes,
Providerpfade, Ticketwerte und Credentials werden nicht im fachlichen Audit
gespeichert.

Die Basis erzeugt selbst noch keinen Dokumenteintrag. Eine erfolgreiche spätere
Veröffentlichung muss Dokument/Version/Klassifikation, Audit und Outbox in
derselben Tenant-Transaktion speichern. `denied` und `failed` benötigen bei
einer zurückgerollten Fachtransaktion einen getrennten, eng begrenzten
Auditpfad. Der erste Vertrag verlangt eine echte Request-ID; bevor Worker
Produzenten werden, wird eine ehrliche Operation-ID-Semantik ergänzt. Die
Kafka-Projektion bleibt vorerst absichtlich minimiert und exportiert die neuen
fachlichen Felder noch nicht; Änderungen vorbehalten.

### Stufe B – sicherer Upload

**Erster Leseschnitt 2026-07-22:** Vor dem Bytepfad ist ein bewusst schmaler
Work-Vertrag für Dokumentliste und Detailansicht vorbereitet. Die Collection-
Berechtigung `core.documents.document.list` autorisiert ausschließlich die
schmale persönliche Metadatenprojektion. Erst die Detailansicht benötigt die
konkrete Ressourcenberechtigung `core.documents.document.read`.
Core Documents begrenzt die dokumentlokale Sichtbarkeit in diesem ersten
Schnitt auf `created_by_account_id` des authentifizierten Work-Kontos. Das ist
eine fail-closed persönliche Ansicht, keine Aussage, dass alle Dokumente eines
Tenants allgemein sichtbar seien. Dokumente technischer Producer und spätere
geteilte Dokumente bleiben unsichtbar, bis ein eigener Binding-/Sichtbarkeits-
vertrag angenommen ist.

Die API gibt ausschließlich Titel, Dokumentstatus, Erzeugermodul,
Klassifikation, Aufbewahrungszuordnung und veröffentlichte Versionsmetadaten
aus. Blob-IDs, Hashes, Größen, Medientypen, Providerzustände und Objektpfade
bleiben außerhalb der Work-Projektion. Die neue Work-Oberfläche bietet deshalb
noch keine Upload- oder Downloadaktion an.

- erste Work-UI für Dokumentliste, Dokumentdetail, Klassifikation und
  unveränderliche Versionshistorie auf dem versionierten Business-API-Vertrag
  aufbauen;
- Uploaddialog mit Fortschritt, Quarantäne-, Prüf-, Fehler- und
  Wiederholungszuständen liefern, ohne Provider oder opake Schlüssel anzuzeigen;
- den vorhandenen fachlichen Auditvertrag als verpflichtenden Producer in den
  Dokument-/Storage-Application-Service einbinden;
- Transfer- und Einmalticket-Zustandsautomat implementieren;
- ersten S3-kompatiblen Adapter und einen deterministischen Testadapter bauen;
- Backend-Streaming, Quarantäne, Größenlimit, MIME-Erkennung, SHA-256,
  Idempotenz und Abbruch umsetzen;
- Dokument plus Version 1 beziehungsweise eine Folgeversion erst nach erneuter
  Policy-Prüfung atomar mit Audit und Outbox veröffentlichen.

### Stufe C – sicherer Download und Betrieb

- autorisierte Downloadaktion und verständliche fail-closed Anzeigen für
  `unknown`, `missing` und vorübergehend nicht verfügbare Inhalte ergänzen;
- Downloadticket an Actor, Tenant, Version, Methode und kurze TTL binden;
- Browserheader gegen aktive Inhaltsausführung und unsichere Dateinamen setzen;
- tatsächlichen Download nach Datenklasse auditieren;
- Object-Store-Health, Orphan-Reconciliation und Metriken ergänzen;
- koordiniertes verschlüsseltes PostgreSQL-/Object-Store-Backup samt Restore-
  und Reconciliation-Test liefern.

### Stufe D – Aufbewahrung und Verarbeitung

- Betreiberregister für Processing-Aktivitäten anbinden;
- Retention-Berechnung, Legal Hold, Löschfreigabe und Löschbeweis umsetzen;
- ausschließlich ungebundene und freigegebene Blobs physisch löschen;
- Vorschau, Malwareprüfung, OCR und Suche als getrennte, idempotente Worker
  ergänzen.

### Stufe E – Collaboration und Sync

- UI für Arbeitskopien, Synchronisationsstatus und bewusst sichtbare Konflikte
  auf dem stabilen Collaboration-Vertrag aufbauen;
- Arbeitskopien mit `base_version_id`, ETag und sichtbaren Konflikten;
- zuerst Vollobjekt-Synchronisation und resumierbare Transfers;
- danach messwertgestützt Chunking, Delta-Sync oder parallele Bearbeitung;
- externe Repository-Systeme nur über gerichtete Adapter mit klarer
  System-of-Record-Zuordnung anbinden.

## Abnahmematrix

| Grenze | Mindestnachweis |
|---|---|
| Tenant | Zwei-Tenant-RLS-Test für jede neue Tabelle und zusammengesetzte Fremdschlüssel |
| Kontoart | Work initiiert Fachzugriff; Admin kann keine Dokumentbytes lesen; Service handelt als eigener Actor |
| Policy | Plattform-Gate plus dokumentlokale Regel; Widerruf vor Veröffentlichung wird wirksam |
| Blob | serverseitiger SHA-256, opaker Schlüssel, fail-closed bei `unknown`/`missing`, keine Inhaltsänderung nach Versiegelung |
| Version | keine Änderung oder Löschung nach Veröffentlichung; monotone Nummer je Dokument |
| Ticket | Digest statt Rohwert, Single-Use, Ablauf, Methoden-/Größen-/Ressourcenbindung |
| Audit | Auslöser und Ausführer getrennt, keine Tickets, Secrets oder Objektpfade |
| Event | stabile IDs und begrenzte Tags; keine Dateiinhalte oder Storage-Locators in Kafka |
| Retention | keine physische Löschung vor gültiger Freigabe; Legal Hold hat Vorrang |
| Restore | PostgreSQL-Metadaten und Bytes gemeinsam prüfbar wiederhergestellt |
| UI | zeigt Serverzustand und Berechtigungen fail-closed; keine Providerdaten, erfundenen Erfolgszustände oder parallele ACL |

## Bewusst nicht im Fundament

- keine öffentliche Share-Link-Funktion;
- keine Cross-Tenant-Deduplizierung;
- keine langlebigen Presigned URLs;
- keine allgemeine Storage-Berechtigung für Benutzer, Apps, KI oder Agents;
- keine eigene ACL-Welt eines Collaboration-Providers;
- kein WebDAV, SMB, CRDT oder Live-Coauthoring;
- keine automatische physische Löschung veröffentlichter Inhalte.

Stufen, Provideradapter, Ticketparameter und Sync-Verfahren werden mit realen
Nutzungs-, Last- und Bedrohungsprofilen verfeinert; Änderungen vorbehalten.
