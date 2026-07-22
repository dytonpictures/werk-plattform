# ADR-007 – Object Storage und Dokumentenhoheit

**Status:** Angenommen  
**Datum:** 2026-07-19

Die konkrete Grenze zwischen Core Documents, Core Storage, Transfer-Tickets,
Identity/Policy, Audit und einem späteren Collaboration-/Sync-Dienst beschreibt
[`ADR-021`](ADR-021-interner-dokument-blob-und-transfervertrag.md). ADR-021
konkretisiert diese Grundentscheidung.

## Kontext

Dokumente und Anhänge können größer werden als für PostgreSQL sinnvoll. Ihre
Berechtigungen, Versionen, Klassifikation und fachliche Zuordnung müssen dennoch
im WERK Core nachvollziehbar bleiben.

## Entscheidung

- PostgreSQL besitzt Dokument, Dokumentversion, Blob-Metadaten, Tenant-Zuordnung,
  Hash, MIME-Typ, Aufbewahrung und Berechtigungsbezug.
- Die Bytes liegen in einem S3-kompatiblen Object Store. Objektpfade sind opaque,
  tenantgebunden und enthalten keine vertraulichen Namen oder frei vom Client
  wählbaren Pfade.
- Clients laden niemals direkt mit dauerhaften Storage-Credentials. Die erste
  Ausbaustufe verwendet einen Backend-Transferendpunkt mit einem kurzlebigen,
  einmalig verbrauchbaren und ressourcengebundenen Ticket. Eine spätere direkte
  Providerübergabe benötigt eine eigene Sicherheitspräzisierung.
- Eine Version wird erst fachlich sichtbar, wenn Objekt und PostgreSQL-Metadaten
  vollständig geprüft und verknüpft sind. Unvollständige Uploads bleiben
  quarantänisiert und werden später sicher bereinigt.
- Integrität wird über einen vom Server berechneten Hash geprüft. Verschlüsselung,
  Retention und Löschung folgen der Datenklassifikation und dem Tenant-Kontext.
- Ein Backup des Object Stores ist ein eigener, mit dem PostgreSQL-Backup
  koordinierter Betriebsprozess. Ein Datenbankdump allein gilt nicht als
  Dokumentenbackup.

## Grenzen

Fachanwendungen besitzen ihre fachliche Dokumentbedeutung, aber keine parallele
Datei- oder Berechtigungsablage. Object Storage ist Infrastruktur und wird über
einen Core-Port austauschbar gehalten.

## Nachweis

Tests müssen Tenant-Isolation, fehlende Direktzugriffe, Hashprüfung,
Wiederaufnahme/Abbruch von Uploads, Versionierung, Lösch- und
Aufbewahrungsregeln sowie einen Restore mit konsistenten Metadaten prüfen.
Ticket-Replay, Widerruf zwischen Ausstellung und Einlösung, duale Actor-/Service-
Zuordnung und ein koordinierter PostgreSQL-/Object-Store-Restore gehören zum
Nachweis; Änderungen vorbehalten.
