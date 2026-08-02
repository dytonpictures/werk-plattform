# WERK – Dokumentationsinventur

**Stand:** 27.07.2026  
**Art:** Bestandsaufnahme ohne Überarbeitung bestehender Dokumente  
**Bezugsstand:** 48 vorhandene Markdown-Dateien mit insgesamt 8.267 Zeilen

## 1. Zweck und Grenze

Diese Inventur erfasst den vorhandenen Dokumentationsbestand, seine grobe
Funktion und erkennbare Überschneidungen. Sie nimmt keine fachlichen
Entscheidungen vor und ändert keine bestehende Dokumentation.

**Nachtrag 29.07.2026:** Die Mengen-, Zeilen- und ADR-Angaben dieses Dokuments
bleiben als historische Momentaufnahme vom 27.07.2026 erhalten. Inzwischen sind
ADR-029 bis ADR-032 hinzugekommen. Das angenommene
[`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md)
ersetzt die globalen MFA-Zugangsbedingungen aus ADR-003 und ADR-030: Gültige
interaktive Admin-Sessions dürfen bekannte Single- oder Multi-Factor-Assurance
tragen, MFA bleibt eine nicht blockierende Empfehlung und `unknown` wird
fail-closed abgewiesen. Die übrigen Kontoart-, Audience-, Permission-, Tenant-,
CSRF-, Audit- und Outbox-Grenzen bleiben bestehen.

Die Bewertung folgt der im Agentenleitfaden festgelegten Reihenfolge:

1. Sicherheitsinvarianten und `DATENMODELL.md`,
2. `ROADMAP.md`,
3. keine ad-hoc fachliche Abkürzung.

`vision.md`, `DATENMODELL.md` und `ROADMAP.md` sind die ausdrücklich benannten
Architekturquellen. Nicht bestätigte Auslegungsfragen stehen separat in
[`OFFENE-FRAGEN-DOKUMENTATIONSINVENTUR.md`](OFFENE-FRAGEN-DOKUMENTATIONSINVENTUR.md).

## 2. Bestand

| Gruppe | Anzahl | Umfang | Funktion |
|---|---:|---:|---|
| Projektleitfaden | 1 | 79 Zeilen | Arbeits- und Sicherheitsregeln für Agenten |
| Einstieg | 1 | 109 Zeilen | Installation, Start, Prüfung und Architekturlinks |
| Architektur- und Fachdokumente unter `docs/` | 17 | 5.221 Zeilen | Zielbild, Datenmodell, Planung, Verträge und Status |
| Architecture Decision Records | 28 | 2.835 Zeilen | angenommene Architekturentscheidungen ADR-001 bis ADR-028 |
| Linux-Paketdokumentation | 1 | 23 Zeilen | Installation des Debian-/Ubuntu-Pakets |
| **Gesamt** | **48** | **8.267 Zeilen** | Stand vor dieser Inventur |

Der Ordner `wiki/` ist vorhanden, enthält zum Inventurzeitpunkt aber keine
Dateien.

## 3. Dokumentenlandkarte

### 3.1 Verbindliche Architekturquellen

| Datei | Ausgewiesene Rolle | Umfang |
|---|---|---:|
| `docs/vision.md` | Zielbild und Architekturvision, Version 2.2 | 833 Zeilen |
| `docs/DATENMODELL.md` | konzeptionelle Architekturgrundlage und Datenhoheit | 1.263 Zeilen |
| `docs/ROADMAP.md` | verbindliche Umsetzungsreihenfolge | 390 Zeilen |

### 3.2 Übergreifendes Projekt- und Statusbild

| Datei | Grobe Funktion | Umfang |
|---|---|---:|
| `docs/WERK_GESAMTPROJEKTZIEL.md` | Produktauftrag, Zielumfang, Sicherheit und Erfolgskriterien | 782 Zeilen |
| `docs/BACKEND-IMPLEMENTIERUNGSSTAND.md` | kompakter technischer Ist-Stand | 100 Zeilen |
| `docs/SESSION_STATUS.md` | ausführlicher Übergabe- und Entwicklungsstatus | 225 Zeilen |

### 3.3 Ausführbare oder fachnahe Verträge

| Datei | Gegenstand | Umfang |
|---|---|---:|
| `docs/API_GRUNDVERTRAG.md` | API-Bereiche, Identität, Fehler und Nebenläufigkeit | 117 Zeilen |
| `docs/AUTHORIZATION.md` | Autorisierung, Ressourcen und Kontoprovisionierung | 237 Zeilen |
| `docs/IDENTITY-MFA.md` | MFA-, Login- und Wiederherstellungsgrenzen | 154 Zeilen |
| `docs/ASYNC-RUNTIME.md` | Outbox, Worker, Idempotenz und Kafka-Export | 95 Zeilen |
| `docs/APPROVAL-CHECKPOINTS.md` | fachneutrale Freigabe- und JIT-Verträge | 201 Zeilen |
| `docs/DOCUMENT-STORAGE.md` | Dokument-, Blob-, Transfer- und Ausbauvertrag | 274 Zeilen |
| `docs/KI_DATENKLASSIFIKATION.md` | Datenklassen für künftige KI-Funktionen | 32 Zeilen |

### 3.4 Client, Oberfläche und Betrieb

| Datei | Gegenstand | Umfang |
|---|---|---:|
| `docs/CLIENT-ARCHITEKTUR.md` | Web- und native Clientgrenzen | 203 Zeilen |
| `docs/DESIGN-SYSTEM.md` | visuelle und sicherheitsbezogene UI-Grundlage | 91 Zeilen |
| `docs/BETRIEBSPROFIL.md` | natives Single-Host-Profil | 94 Zeilen |
| `docs/BOOTSTRAP.md` | initialer Administrator und Entwicklungszugang | 41 Zeilen |
| `README.md` | zentraler Einstieg für Start und Entwicklung | 109 Zeilen |
| `packaging/linux/README.md` | Paketinstallation auf Debian/Ubuntu | 23 Zeilen |

### 3.5 Architekturentscheidungen

Die ADR-Reihe ist lückenlos von ADR-001 bis ADR-028 vorhanden. Alle 28 ADRs
weisen einen angenommenen Status aus. Inhaltlich deckt sie folgende Cluster ab:

| Cluster | ADRs |
|---|---|
| Tenant, Kontoarten, Identity und Autorisierung | 001, 003, 004, 010–012, 014–018, 024 |
| Betrieb, Backup, Release, HA und Transport | 002, 005, 015, 019, 022, 023, 028 |
| Events und Infrastrukturverträge | 006, 020, 025, 027 |
| Dokumente und Storage | 007, 021, 026 |
| Erweiterungen und KI | 008, 009 |
| Native Clients | 013 |

## 4. Abgleich mit dem sichtbaren Repository

Die dokumentierte Plattformstruktur ist im sichtbaren Codebestand grundsätzlich
wiederzufinden:

- getrennte Prozesse unter `cmd/api`, `cmd/worker`, `cmd/migrate` und
  `cmd/werkctl`,
- Core-Grenzen unter anderem für Identity, Tenancy, Party, Authorization,
  Resources, Audit, Events, Documents, Storage und Provider Registry,
- Plattformadapter für Datenbank, HTTP, Migration, Outbox, Kafka,
  Transport-Security und spezialisierte Stores,
- 35 fortlaufende, unveränderte Migrationsdateien,
- getrennte API-Präfixe `/api/v1`, `/admin/v1` und `/service/v1` im
  maschinenlesbaren OpenAPI-Vertrag,
- eingebettetes Dashboard sowie native Linux-Paket- und Startartefakte.

Dieser Abgleich bestätigt nur sichtbare Struktur und benannte Verträge. Er ist
kein vollständiger Verhaltens-, Sicherheits- oder Testnachweis.

## 5. Auffälligkeiten der Inventur

Die folgenden Punkte sind Beobachtungen, noch keine entschiedenen Fehler oder
Änderungsaufträge:

1. `WERK_GESAMTPROJEKTZIEL.md`, `vision.md` und `DATENMODELL.md` überschneiden
   sich stark bei Produktbild, Architektur und Sicherheitsregeln. Nur die drei
   in `AGENTS.md` genannten Quellen besitzen eine ausdrücklich definierte
   Konfliktreihenfolge; die Rolle des Gesamtprojektziels darin ist nicht
   festgelegt.
2. `ROADMAP.md` trägt weiterhin den Status „Startplanung“, enthält zugleich
   datierte Abschlussnachweise für Phase 0 und Phase 1 sowie fortgeschrittene
   Umsetzungsstände für spätere Grundlagen.
3. `ROADMAP.md`, `SESSION_STATUS.md` und
   `BACKEND-IMPLEMENTIERUNGSSTAND.md` dokumentieren parallel den Ist-Stand.
   Ein eindeutiger Owner für den jeweils aktuellen Status ist nicht
   ausgewiesen.
4. Das Gesamtprojektziel verwendet eine eigene fünfstufige
   Entwicklungsreihenfolge, während die verbindliche Roadmap sieben
   nummerierte Schritte von Phase 0 bis Phase 6 verwendet. Ob beide Ebenen
   bewusst verschieden sind, ist nicht ausdrücklich erläutert.
5. Status- und Metadaten sind uneinheitlich: Einige Dokumente führen Status,
   Version und Datum, andere nur einzelne Angaben oder keine Metadaten.
6. `packaging/linux/README.md` ist englisch, während die übrige
   Projektdokumentation überwiegend deutsch ist.
7. Der leere Ordner `wiki/` hat im sichtbaren Dokumentationsmodell keine
   erklärte Funktion.
8. Mehrere Dokumente mischen Zielvertrag, Implementierungsstand und offene
   Ausbaustufen. Dadurch ist Aktualität häufig nur abschnittsweise und nicht
   für das gesamte Dokument erkennbar.

## 6. Noch nicht ausgeführte Vertiefung

Nicht Bestandteil dieser ersten Inventur sind:

- semantischer Satz-für-Satz-Konfliktvergleich aller Dokumente,
- vollständige Verifikation jeder Implementierungsbehauptung gegen Code und
  Tests,
- Prüfung externer Links und externer Normreferenzen,
- sprachliche, redaktionelle oder terminologische Überarbeitung,
- Änderung einer bestehenden Datei oder Festlegung einer neuen
  Dokumentationshierarchie.

Diese Punkte benötigen zuerst bestätigte Antworten auf die zurückgestellten
Grundsatzfragen.
