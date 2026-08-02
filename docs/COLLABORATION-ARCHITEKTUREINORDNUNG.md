# WERK – Mögliche Architektureinordnung technischer Collaboration-Grundlagen

**Stand:** 27.07.2026  
**Status:** Analyse, nicht entschieden  
**Gegenstand:** Synchronisationsraum, Operationsstrom, Journal und Snapshot  
**Nicht enthalten:** Technologieauswahl, Datenbankschema, API-Vertrag,
fachliche Core-Objekte oder Implementierungsauftrag

## 1. Fragestellung

Diese Analyse untersucht, wo vier technische Collaboration-Mechanismen in den
bestehenden Architekturquellen von WERK beschrieben werden könnten:

1. ein begrenzter Synchronisationsraum,
2. ein geordneter Strom kollaborativer Operationen,
3. ein dauerhaftes Operationsjournal,
4. ein daraus ableitbarer verdichteter Snapshot.

Sie verändert weder `vision.md`, `DATENMODELL.md` noch `ROADMAP.md`. Sie bewertet
keinen Algorithmus und leitet keine neuen fachlichen Core-Objekte ab.

## 2. Bereits festgelegte Ausgangslage

Die Architektur enthält Collaboration nicht als völlig neues Thema:

- `vision.md` trennt veränderliche Arbeitskopien von unveränderlichen
  veröffentlichten Dokumentversionen.
- `DATENMODELL.md` ordnet einem späteren Collaboration-/Sync-Dienst
  Arbeitskopien, Revisionen, Sync-Cursor und Konflikte zu. Dieser Dienst besitzt
  weder eigene Identity noch eigene ACL- oder Dokumentwahrheit.
- `ROADMAP.md` ordnet Collaboration und Sync nach dem Dokument-/Storage-Fundament
  in Phase 3 ein.
- ADR-021 bestätigt dieselbe Verantwortungsgrenze und schließt CRDT,
  Live-Coauthoring und automatische Konfliktzusammenführung aus dem ersten
  Dokument-/Storage-Schnitt aus.
- `RealtimePort` ist ausschließlich für flüchtige Hinweise vorgesehen. Clients
  laden danach den autorisierten Zustand erneut über die Business-API.
- PostgreSQL bleibt die fachliche und auditierbare Wahrheit. Valkey und Kafka
  ersetzen weder Arbeitsstand noch Veröffentlichung.

Damit ist bereits entschieden, **wo Collaboration fachlich nicht Eigentümer
werden darf**. Noch nicht entschieden ist, wie ihre technische Laufzeit
geschnitten wird.

## 3. Mögliche Einordnung in `vision.md`

### Möglichkeit V1 – Konkretisierung im Abschnitt „Daten und Dateien“

Die vier Mechanismen könnten ausschließlich als technische Unterstruktur des
bereits genannten Collaboration-/Sync-Dienstes beschrieben werden.

Dabei würde die Vision nur folgende Grenzen festhalten:

- ein Synchronisationsraum gehört zu genau einer tenantgebundenen Arbeitskopie,
- Operationen und Snapshots sind keine veröffentlichten Dokumentversionen,
- nur Core Documents veröffentlicht einen akzeptierten Stand,
- Presence und Transport bleiben von dauerhaftem Inhalt getrennt.

Diese Einordnung macht keine Aussage darüber, ob die Laufzeit später auch von
anderen Datenformen verwendet werden darf.

### Möglichkeit V2 – Eigene technische Collaboration-Leitplanke

Die Vision könnte zusätzlich eine fachneutrale Leitplanke zwischen „Events und
Jobs“ und „Daten und Dateien“ erhalten. Sie würde Collaboration als besondere
Form synchroner, konvergierender Zustandsverteilung von Domain-Events,
Realtime-Hinweisen und Fachmutationen abgrenzen.

Eine solche Leitplanke wäre breiter als der bisher ausdrücklich
dokumentbezogene Ausbaupfad. Sie würde noch keinen allgemeinen fachlichen
Collaboration-Core rechtfertigen.

### Möglichkeit V3 – Keine Ergänzung der Vision

Die vorhandene Trennung kann als ausreichend betrachtet werden. Die vier
Mechanismen würden dann erst in Datenmodell, ADR und Ausbauplan präzisiert.

## 4. Mögliche Einordnung in `DATENMODELL.md`

### Möglichkeit D1 – Bestandteil der Dokumentdomäne

Die Mechanismen könnten unter „Dokumente und Suche“ als interne technische
Strukturen des späteren Collaboration-/Sync-Dienstes eingeordnet werden. Ihre
Identität wäre immer aus einer Dokument-Arbeitskopie abgeleitet.

Konzeptionell wären dabei Verantwortungen zu unterscheiden, ohne bereits
physische Objekte festzulegen:

```text
Arbeitskopie
  -> begrenzt den Synchronisationsraum
  -> verweist auf eine veröffentlichte Basisversion

Operationsverlauf
  -> enthält geordnete, idempotent identifizierbare Änderungen
  -> ist nicht selbst die veröffentlichte Dokumenthistorie

Snapshot
  -> verdichtet einen bestätigten Operationsstand
  -> bleibt veränderlicher Arbeitszustand

Veröffentlichung
  -> autorisiert erneut
  -> erzeugt über Core Documents eine neue unveränderliche Version
```

### Möglichkeit D2 – Fachneutrale technische Capability mit erstem Verbraucher

Das Datenmodell könnte die vier Mechanismen als wiederverwendbare technische
Capability beschreiben. Core Documents wäre ihr erster und zunächst einziger
Verbraucher. Fachliche Datenformen, Merge-Regeln und Veröffentlichungswirkung
blieben Eigentum des jeweiligen Owners.

Diese Möglichkeit würde eine zusätzliche Grenze benötigen: Die technische
Capability dürfte weder generische Fachobjekte noch eine globale ACL, Identity,
Suche oder Dokumentwahrheit besitzen.

### Möglichkeit D3 – Nur referenzierter Laufzeitvertrag

Das konzeptionelle Datenmodell könnte bei Arbeitskopie, Revision, Sync-Cursor
und Konflikt bleiben. Operationsjournal und Snapshot würden ausschließlich in
einem späteren technischen ADR beschrieben. Damit bliebe das Datenmodell frei
von Laufzeitdetails.

## 5. Mögliche Einordnung in `ROADMAP.md`

### Möglichkeit R1 – Vollständig in Phase 3

Synchronisationsraum, Operationsstrom, Journal und Snapshot würden erst als
Teil der Dokument- und Arbeitskoordination umgesetzt. Phase 2 stellte weiterhin
nur allgemeine Ressourcen-, Autorisierungs-, Event- und Infrastrukturgrundlagen
bereit.

Die Reihenfolge innerhalb von Phase 3 wäre dann mindestens:

```text
Dokument-Producer und sichere Veröffentlichung
  -> produktiver Bytepfad und Restore-Nachweis
  -> begrenzte Arbeitskopie
  -> Journal und Snapshot
  -> erster Synchronisationsnachweis
```

### Möglichkeit R2 – Technische Grundlage in Phase 2, Verbraucher in Phase 3

Phase 2 könnte einen fachneutralen Laufzeitnachweis für geordnete Operationen,
Idempotenz, Journalverdichtung und Wiederanlauf enthalten. Phase 3 würde daraus
eine dokumentbezogene Arbeitskopie mit Veröffentlichung bauen.

Der vorhandene Outbox-/Kafka-Pfad wäre dabei eine Abhängigkeit oder ein
Vergleichspunkt, aber nicht automatisch das Collaboration-Journal. Domain-Events
beschreiben bestätigte fachliche Tatsachen; Collaboration-Operationen verändern
einen noch nicht veröffentlichten Arbeitszustand.

### Möglichkeit R3 – Nach Phase 3 verschieben

Phase 3 könnte zunächst ohne Live-Coauthoring abgeschlossen werden. Die vier
Mechanismen würden erst nach einem produktiven Dokumentpfad oder einem
fachlichen Pilotprozess geplant. Der bestehende Roadmap-Satz „Collaboration und
Sync folgen danach“ legt dafür keinen exakten Abnahmeschnitt fest.

## 6. Abhängigkeiten, die bereits aus der Architektur folgen

Unabhängig von der später gewählten Einordnung sind folgende Abhängigkeiten im
heutigen Vertragsbestand sichtbar:

| Abhängigkeit | Begründung |
|---|---|
| tenantgebundene ResourceRef | ein Synchronisationsraum darf nicht tenantfrei adressiert werden |
| serverseitige Autorisierung | Sitzungszutritt und Operationen dürfen keine Cliententscheidung sein |
| Dokument-Application-Service | nur er kann akzeptierten Inhalt als kanonische Version veröffentlichen |
| atomare Veröffentlichung mit Audit und Outbox | Arbeitskopie und veröffentlichte Wahrheit müssen unterscheidbar bleiben |
| sichere Blob- und Transfergrenze | ein Snapshot darf keine parallele dauerhafte Byte-Hoheit erzeugen |
| Versionierung von Operation und Snapshotformat | alte und neue Clients müssen denselben Stand interpretieren |
| Restore-Vertrag | Journal, Snapshot und referenzierte Basisversion dürfen nach Restore nicht auseinanderfallen |
| explizites Ausfallverhalten | ein defekter Collaboration-Pfad darf keine unkoordinierten Mehrfachschreibwege öffnen |

## 7. Ausschlusskriterien

Ein späterer Vorschlag läge außerhalb der bestätigten Architektur, wenn er
mindestens eines der folgenden Merkmale besitzt:

1. Er macht Fluid, Valkey, Kafka, einen Object Store oder einen externen Relay
   zur alleinigen fachlichen Wahrheit.
2. Er veröffentlicht Änderungen ohne erneute serverseitige Prüfung von Actor,
   Kontoart, Tenant, Ressource, Permission und Processing-Policy.
3. Er behandelt den vorhandenen Domain-Event-Outbox-Strom ungeprüft als
   Collaboration-Operationsjournal.
4. Er schreibt jede flüchtige Cursor- oder Zwischenoperation als fachlichen
   Domain-Event oder unveränderlichen Security-Audit.
5. Er erlaubt Clients direkten Datenbank-, Valkey-, Kafka- oder
   Object-Storage-Zugriff.
6. Er führt eine zweite Dokumentwahrheit, Identity oder ACL-Welt ein.
7. Er setzt einen universellen Merge-Algorithmus für Text, Bäume, Whiteboards,
   Formulare und beliebige Fachobjekte voraus.
8. Er kann Journal und Snapshot nach Absturz oder Restore nicht auf einen
   nachweisbaren gemeinsamen Stand bringen.
9. Er lässt Rechteentzug, Sessionwiderruf oder Tenant-Sperre bis zum Ende einer
   beliebig langen Collaboration-Sitzung unbeachtet.
10. Er erklärt optimistische Anzeige im Client bereits zur bestätigten
    Veröffentlichung.

## 8. Noch nicht entschiedene Architekturachsen

| Achse | Noch offene Grenzen |
|---|---|
| Eigentum der Laufzeit | dokumentinterner Dienst oder fachneutrale technische Capability |
| Roadmap-Zeitpunkt | vollständig Phase 3, Grundlage Phase 2 oder nach Phase 3 |
| Persistenzform | Operationen, Snapshot oder Kombination; konkrete PostgreSQL-Grenze offen |
| Ordnungsautorität | einzelner Server, partitionierter Dienst oder anderer Mechanismus |
| Snapshotrolle | reine Beschleunigung, Recovery-Anker oder beides |
| Granularität | Dokument, Abschnitt, Datenbaum oder anderer Raumzuschnitt |
| Wiederanlauf | Replay ab Snapshot, erneuter Vollzustand oder hybrider Ablauf |
| Aufbewahrung | Verdichtung, Löschung und Auditbezug noch nicht bestimmt |
| erster Datentyp | Rich Text, strukturierter Baum, Whiteboard oder Formular nicht gewählt |

## 9. Ergebnis dieser Einordnungsanalyse

Die vorhandene Architektur bietet drei konsistente Positionen:

- ausschließlich dokumentbezogen in Phase 3,
- fachneutrale technische Grundlage in Phase 2 mit Dokumenten als erstem
  Verbraucher in Phase 3,
- spätere Ergänzung nach dem ersten vollständigen Dokument- oder Pilotpfad.

Keine dieser Positionen ist durch diese Analyse ausgewählt. Eine Auswahl würde
langfristige Daten-, Betriebs- und Clientgrenzen festlegen und benötigt daher
bestätigte Antworten auf die offenen Architekturachsen sowie voraussichtlich
einen eigenen ADR vor einer Änderung der drei Architekturquellen.
