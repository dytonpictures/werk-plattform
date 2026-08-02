# WERK – Inventur technischer Collaboration-Mechanismen

**Stand:** 27.07.2026  
**Art:** neutrale Konzeptinventur ohne Architekturentscheidung  
**Grenze:** technische Mechanismen und Architekturmerkmale; keine neuen
fachlichen Core-Objekte, Verträge oder Implementierungen

## 1. Untersuchungsauftrag

Diese Inventur untersucht, welche technischen Mechanismen sich in aktiven,
über mehrere Jahre produktiv eingesetzten Collaboration-Technologien größerer
Unternehmen wiederholen. Sie gleicht diese Mechanismen ausschließlich mit dem
sichtbaren WERK-Core ab.

Der Abgleich verwendet nur diese Zustände:

- **vorhanden** – ein passender ausführbarer Mechanismus ist sichtbar,
- **teilweise vorhanden** – Grundlage oder Vertrag ist sichtbar, aber kein
  vollständiger Collaboration-Mechanismus,
- **nicht vorhanden** – im sichtbaren Bestand nicht nachweisbar,
- **konfliktbehaftet** – eine direkte Übernahme würde einer bestätigten
  WERK-Invariante widersprechen.

Die Zustände sind keine Empfehlungen und keine Priorisierung.

## 2. Betrachtete Technologien

| Technologie | Unternehmen | Produktiver Nachweis | Technisch sichtbarer Schwerpunkt |
|---|---|---|---|
| Fluid Framework / Fluid Service | Microsoft | aktive Framework-Dokumentation; Einsatzpfade für Microsoft-/SharePoint-Anwendungen | clientseitige verteilte Datenstrukturen, zentral sequenzierte Operationen, Container |
| Figma Multiplayer | Figma | öffentlich seit 2016; weiterhin Kernfunktion des Produkts | autoritativer Multiplayer-Server, WebSockets, eigenschaftsbezogene Konfliktauflösung, Journal und Checkpoints |
| Canva Collaboration | Canva | bestehende Echtzeitbearbeitung; Presence-Infrastruktur für bis zu eine Million Nutzer beschrieben | getrennte Daten- und Presence-Pfade, WebSockets, Redis Pub/Sub und WebRTC |
| Confluence Collaborative Editing / Synchrony | Atlassian | aktive Cloud- und Data-Center-Funktion mit gemeinsamem Bearbeiten | gemeinsamer Entwurf, Synchrony-Sitzung, WebSockets, Snapshot und Veröffentlichung |
| Google Docs | Google | langjähriger produktiver Einsatz; Google veröffentlichte messbare organisationsweite Nutzung | erfolgreicher Referenzfall für gleichzeitige Dokumentbearbeitung; aktuelle interne Mechanik ist öffentlich nicht hinreichend beschrieben |

Google Docs bleibt als Erfolgsreferenz enthalten. Aussagen zu seiner konkreten
heutigen Synchronisationsimplementierung werden mangels ausreichend aktueller
offizieller technischer Offenlegung nicht abgeleitet. Eingestellte oder nur noch
im Wartungsmodus befindliche Technologien wurden nicht als Vergleichssystem
verwendet.

## 3. Wiederkehrende technische Mechanismen

### 3.1 Begrenzter Synchronisationsraum

Collaboration wird in einen technisch adressierbaren Raum geschnitten: Fluid
verwendet Container, Figma einen Multiplayer-Prozess beziehungsweise eine
Sitzung je Datei und Confluence einen gemeinsamen Entwurf je Seite. Dadurch
werden Reihenfolge, Teilnehmer, Wiederanlauf und Last nicht installationsweit
vermischt.

Der Raum ist nicht automatisch dasselbe wie ein Tenant oder eine dauerhafte
fachliche Ressource. Er ist die technische Konvergenzgrenze einer konkreten
Zusammenarbeit.

### 3.2 Operationen statt vollständiger Zustandsübertragung

Lokale Änderungen werden als kleine Operationen übertragen. Ein Dienst ordnet
oder autorisiert den Strom; Clients wenden die bestätigten Änderungen auf ihren
lokalen Zustand an. Fluid verlagert die Merge-Logik in gemeinsame
Clientbibliotheken. Figma hält dagegen einen autoritativen Server für Ordnung,
Validierung und Konfliktauflösung.

Damit sind mindestens drei Verantwortungen getrennt:

1. lokale, sofort sichtbare Änderung,
2. globale Ordnung beziehungsweise Konfliktentscheidung,
3. dauerhafte Rekonstruktion oder Verdichtung.

### 3.3 Datenmodellabhängige Konfliktregeln

Es gibt keinen einzigen sinnvollen Merge-Algorithmus für alle Datenformen.
Fluid stellt spezialisierte Distributed Data Structures bereit und versieht
`SharedTree` mit Schema, Transaktionen und Schemaentwicklung. Figma verwendet
für Designobjekte unter anderem eine servergeordnete Last-Writer-Wins-Regel auf
der Ebene einzelner Objekteigenschaften und eine gesonderte Behandlung von
Objektbäumen. Texteditoren stellen andere Anforderungen als Whiteboards oder
strukturierte Formulare.

### 3.4 Gemeinsame deterministische Logik

Wenn Clients selbst Operationen zusammenführen, müssen sie kompatible
Merge-Logik und Schemas verwenden. Versions- und Schemaentwicklung sind damit
Teil des Laufzeitprotokolls und nicht nur eine Build-Frage. Ein alter Client
darf neue Operationen nicht stillschweigend anders interpretieren.

### 3.5 Journal plus verdichteter Zustand

Ein alleiniger In-Memory-Zustand ist nicht ausreichend. Figma ergänzt
regelmäßige Datei-Checkpoints durch ein Write-ahead-Journal. Fluid-Dienste
verwenden sequenzierte Operationen und Zusammenfassungen. Confluence hält den
gemeinsamen Entwurf im Synchronisationsdienst und speichert zusätzlich
Snapshots im Hauptsystem.

Das wiederkehrende Muster lautet:

```text
Operationen -> geordneter/journalisierter Verlauf -> Snapshot oder Summary
                                              -> Wiederanlauf und Rekonstruktion
```

### 3.6 Wiederverbinden und Offline-Änderungen

Verbindungsabbrüche gehören zum Normalbetrieb. Figma lädt beim Wiederverbinden
einen frischen Zustand und wendet lokale Offline-Änderungen erneut darauf an.
Fluid dokumentiert Reconnect- und Offline-Übergänge als notwendigen Teil des
Anwendungsdesigns. Dazu werden stabile Operationskennungen, ein bekannter
Bestätigungsstand und deterministische Wiederanwendung benötigt.

### 3.7 Flüchtige Presence getrennt von dauerhaftem Inhalt

Cursor, Auswahl, Tippstatus und Teilnehmeranwesenheit besitzen eine andere
Lebensdauer als Dokumentänderungen. Fluid führt dafür gesonderte Presence-APIs,
deren Sitzungsdaten nach Ende der Sitzung verschwinden. Canva skalierte
Mauszeiger zunächst über WebSockets und Redis Pub/Sub und untersuchte für den
latenzsensitiven flüchtigen Pfad WebRTC.

Presence benötigt daher nicht dieselbe Persistenz-, Replay- oder
Aufbewahrungsgarantie wie ein veröffentlichter Inhalt.

### 3.8 Transport ist nicht das Konsistenzmodell

WebSockets, WebRTC, Relay und Pub/Sub transportieren Nachrichten, bestimmen
aber nicht allein deren fachliche Gültigkeit. Figma nutzt WebSockets zu einem
autoritativen Dienst. Canva verwendet unterschiedliche Transporte je nach
Lebensdauer und Skalierungsziel. Fluid trennt Client Runtime, Container und
Fluid Service.

### 3.9 Entwurf und Veröffentlichung sind verschiedene Zustände

Confluence unterscheidet den gemeinsamen Entwurf von der veröffentlichten
Seitenversion. Microsoft weist für aktuelle Fluid-Integrationen ausdrücklich
darauf hin, dauerhafte Dateien und audit-, retention-, such- oder
reportingrelevante Geschäftsergebnisse in einem dafür vorgesehenen dauerhaften
Modell zu speichern. Figma trennt kollaborative Dateidaten von Kommentaren,
Benutzern, Teams und Projekten in PostgreSQL.

### 3.10 Externe Änderungen benötigen denselben Synchronisationsweg

Confluence propagiert Änderungen außerhalb des Editors über einen definierten
Synchrony-Aufruf. Direkte Änderungen, die diesen Weg umgehen, können verloren
gehen oder einen abweichenden Zustand erzeugen. Ein Collaboration-System kann
daher nicht zuverlässig parallel zu einem unkoordinierten zweiten Schreibpfad
betrieben werden.

### 3.11 Autorisierung am Sitzungs- und Operationsrand

Der Zutritt zu einem Synchronisationsraum wird authentifiziert und begrenzt.
Microsofts Relay-Vertrag verwendet dienstspezifische Tokens; Confluence bindet
die WebSocket-Sitzung an einen Seitenbezug und ein JWT. Diese Eintrittsprüfung
beantwortet allein noch nicht, ob jede spätere Operation unter inzwischen
geänderten Rechten zulässig bleibt.

### 3.12 Degradierter Betrieb ist ein eigener Zustand

Confluence Data Center kann bei längerer Nichtverfügbarkeit von Synchrony in
einen begrenzten Einzelbearbeitungsmodus wechseln. Collaboration-Ausfall wird
damit nicht als stiller Wechsel zu unkoordinierten Mehrfachschreibvorgängen
behandelt.

## 4. Abgleich mit dem sichtbaren WERK-Core

| Mechanismus | WERK-Status | Sichtbarer Bezug | Abgrenzung |
|---|---|---|---|
| begrenzter Synchronisationsraum je Arbeitskopie | **nicht vorhanden** | Dokumente und Versionen sind modelliert | kein ausführbarer Raum-, Teilnehmer- oder Sitzungsvertrag sichtbar |
| geordneter Collaboration-Operationsstrom | **nicht vorhanden** | Outbox und Kafka ordnen Domain-Events partitionsbezogen | Domain-Events sind Fakten nach fachlicher Mutation, keine Merge-Operationen einer Arbeitskopie |
| optimistische lokale Operationen | **nicht vorhanden** | Clientarchitektur beschreibt spätere Offline-Projektionen | kein Collaboration-Protokoll oder Bestätigungsstand sichtbar |
| datenmodellabhängige Merge-Regeln | **nicht vorhanden** | Dokumentmodell und Ressourcentypen sind typisiert | keine Text-, Baum-, Listen- oder Property-Merge-Semantik vorhanden |
| Schema- und Protokollkompatibilität | **teilweise vorhanden** | versionierte APIs, Events, Ressourcen und Verträge | keine Laufzeitkompatibilität kollaborativer Operationen definiert |
| Journal plus Snapshot/Summary | **teilweise vorhanden** | PostgreSQL, Outbox, unveränderliche Dokumentversionen | kein Journal oder Snapshot einer veränderlichen Arbeitskopie vorhanden |
| Reconnect, Replay und Offline-Rebase | **nicht vorhanden** | spätere Offline-Fähigkeit ist dokumentiert | Algorithmus, Cursor und Konfliktbehandlung fehlen |
| flüchtige Presence | **teilweise vorhanden** | austauschbarer `RealtimePort` ist konzeptionell beschrieben | keine Presence-Semantik oder Runtime sichtbar |
| Transportabstraktion | **teilweise vorhanden** | `RealtimePort`, HTTP und Kafka-Grenzen | Collaboration-Transport und Backpressure sind nicht festgelegt |
| Entwurf versus veröffentlichte Version | **teilweise vorhanden** | unveränderliche veröffentlichte Dokumentversionen; Collaboration-Arbeitskopien sind vorgesehen | Arbeitskopien und ihr Übergang zur Version sind noch nicht implementiert |
| definierter Pfad für externe Änderungen | **nicht vorhanden** | Modulgrenzen verbieten fremde Tabellenschreibzugriffe | kein Synchronisationsgateway für eine aktive Arbeitskopie vorhanden |
| Tenant- und serverseitige Autorisierung | **vorhanden** | Actor, Tenant-Kontext, ResourceRef, Permission und RLS | Collaboration-spezifische Wiederprüfung bei Rechteentzug fehlt |
| atomare dauerhafte Veröffentlichung mit Audit/Outbox | **teilweise vorhanden** | Plattformmuster und Dokument-Auditbasis existieren | ein Dokument-Producer und Collaboration-Publish-Pfad fehlen |
| degradierter Collaboration-Betrieb | **nicht vorhanden** | allgemeine Readiness- und Infrastrukturgrenzen existieren | kein expliziter Collaboration-Ausfallzustand definiert |

## 5. Konfliktbehaftete Direktübernahmen

Die folgenden Punkte wären nur bei unveränderter Direktübernahme
konfliktbehaftet; daraus wird keine Entscheidung über eine angepasste Nutzung
abgeleitet.

| Direkt übernommene Annahme | Konflikt mit bestätigtem WERK-Vertrag |
|---|---|
| Fluid- oder Relay-Zustand ist alleinige fachliche Wahrheit | PostgreSQL ist die fachliche Wahrheit; veröffentlichte Version, Audit und Outbox müssen dauerhaft und atomar sein |
| Merge-Logik im Client entscheidet fachliche Berechtigung | jede mandantenbezogene Operation benötigt serverseitige Berechtigungsprüfung |
| Redis/Valkey Pub/Sub oder Presence hält verbindliche Änderungen | Valkey ist austauschbare Infrastruktur und nie einzige Wahrheit |
| P2P-Übertragung ersetzt den autorisierten Serverpfad | Policy, Tenant-Prüfung, Audit und Freigaben dürfen nicht umgangen werden |
| ein generischer Merge-Vertrag gilt unterschiedslos für alle Fachobjekte | Module besitzen ihre Fachlogik und Datenhoheit; Core-Verträge bleiben fachneutral und versioniert |
| Sitzungszutritt erteilt dauerhaft alle Schreibrechte | Rechte, Kontoart, Tenant, Ressource und gegebenenfalls Processing-Policy müssen weiterhin fail-closed geprüft werden |

## 6. Verdichtetes Ergebnis

Die untersuchten Systeme konvergieren nicht auf einen bestimmten Algorithmus.
Sie konvergieren auf technische Trennungen:

1. dauerhafter veröffentlichter Zustand und veränderliche Zusammenarbeit,
2. Inhaltsoperationen und flüchtige Presence,
3. Transport und Konsistenzmodell,
4. lokaler optimistischer Zustand und global bestätigte Ordnung,
5. Operationsjournal und verdichteter Snapshot,
6. generische Laufzeitmechanik und datentypabhängige Merge-Regeln,
7. Sitzungszutritt und fortlaufende Operationsautorisierung,
8. normaler Echtzeitbetrieb und ausdrücklich degradierter Betrieb.

WERK besitzt bereits starke Grundlagen für dauerhafte Wahrheit, Tenant-Grenze,
Autorisierung, Audit, Outbox, Dokumentversionen und austauschbare flüchtige
Infrastruktur. Im sichtbaren Core fehlen dagegen die eigentlichen Mechanismen
für Collaboration-Räume, Operationskonvergenz, Merge-Semantik, Reconnect,
Presence und degradierten Collaboration-Betrieb. Diese Feststellung legt weder
eine Core-Erweiterung noch eine Technologieauswahl fest.

## 7. Quellen und Evidenzgrenze

- [Fluid Framework: Architektur](https://fluidframework.com/docs/concepts/architecture)
- [Fluid Framework: SharedTree](https://fluidframework.com/docs/data-structures/tree)
- [Microsoft: Fluid mit SharePoint Embedded](https://learn.microsoft.com/en-us/sharepoint/dev/embedded/build/fluid-framework)
- [Microsoft: Azure Fluid Relay – Überblick](https://learn.microsoft.com/en-us/azure/azure-fluid-relay/overview/overview)
- [Figma: Funktionsweise der Multiplayer-Technik](https://www.figma.com/blog/how-figmas-multiplayer-technology-works/)
- [Figma: zuverlässiger Multiplayer mit Write-ahead-Log](https://www.figma.com/blog/making-multiplayer-more-reliable/)
- [Canva: Echtzeit-Mauszeiger und skalierte Presence](https://www.canva.dev/blog/engineering/realtime-mouse-pointers/)
- [Canva: reaktive Echtzeitdienste für Canva Live](https://www.canva.dev/blog/engineering/lessons-learnt-from-building-reactive-microservices-for-canva-live/)
- [Atlassian: Collaborative Editing und Synchrony](https://developer.atlassian.com/cloud/confluence/collaborative-editing/)
- [Atlassian: Collaborative Editing in Confluence Data Center](https://developer.atlassian.com/server/confluence/collaborative-editing-for-confluence-server/)
- [Google Research: Collaboration in the Cloud at Google](https://research.google/pubs/collaboration-in-the-cloud-at-google/)

Die Quellen beschreiben öffentlich sichtbare Architekturteile, nicht die
vollständigen heutigen Produktionssysteme. Wo ein Unternehmen keine aktuelle
interne Mechanik offenlegt, enthält diese Inventur keine technische Behauptung
aus inoffiziellen Rekonstruktionen.
