# WERK – Inventur der Core- und Plattformdienste

**Stand:** 29.07.2026  
**Status:** Analyse des sichtbaren Repository-Stands  
**Geltungsbereich:** Core- und Plattformdienste aus Vision, Datenmodell und
Roadmap; keine späteren Fachanwendungen

## 1. Bewertungsmodell

Die Bewertung misst nicht Quelltextmenge, sondern den nachweisbaren Weg eines
Dienstes vom Vertrag bis zum Betrieb. Sie ist eine Arbeitsmetrik und keine
Behauptung über einen mathematisch exakten Fertigstellungsgrad.

| Bereich | Gewicht | Vollständiger Nachweis |
|---|---:|---|
| Architektur und Datenhoheit | 15 % | Owner, Grenzen und Abhängigkeiten sind widerspruchsfrei dokumentiert |
| öffentlicher/versionierter Vertrag | 15 % | Typen, API, Ressourcen, Permissions und Events sind soweit erforderlich festgelegt |
| Core- und Application-Code | 15 % | ausführbarer Dienstpfad statt ausschließlich Typen oder Interfaces |
| Persistenz und Adapter | 15 % | Migration, Runtime-Rolle und realer Adapter sind vorhanden |
| Sicherheit und Atomizität | 15 % | Tenant, Policy, Audit und Outbox sind im Mutationspfad geschlossen |
| automatisierte Nachweise | 15 % | Unit-, Integrations- und negative Sicherheitstests decken den Dienstpfad |
| Betrieb und Wiederherstellung | 10 % | Konfiguration, Metriken, Ausfallverhalten, Backup/Restore und Upgrade sind benannt oder geprüft |
| **Gesamt** | **100 %** | Dokumentation 30 %, Code und ausführbare Nachweise 70 % |

Ein ausschließlich geplanter oder dokumentierter Dienst bleibt nach der
bestätigten Bewertungsregel unter 50 %. Prozentwerte sind auf fünf Punkte
gerundet. „Fehlt“ bedeutet, dass kein dienstspezifischer ausführbarer Kern
sichtbar ist; allgemeine Plattformgrundlagen können trotzdem existieren.

## 2. Gesamtübersicht

| Roadmap | Dienst | Reife | Einordnung | Zentrale Evidenz oder Lücke |
|---|---|---:|---|---|
| 0 | Konfiguration und `.env` | 80 % | über 50 % | Validierung, Tests, Startintegration; produktive Secret-Ablösung offen |
| 0 | Migration | 85 % | über 50 % | 43 fortlaufende Migrationen, Sperre, Strukturtests und ausführbarer Wegwerf-Datenbanklauf |
| 0 | Lokales Betriebswerkzeug | 65 % | über 50 % | `werkctl version`, read-only Betriebsinventur und öffentlicher API-Status mit stabilen JSON-/Exitverträgen sowie bestehender Migratorpfad; Service-, Update- und Pairing-Runner fehlen |
| 0 | Native API-/Worker-Prozesse | 75 % | über 50 % | getrennte Commands und Units; Heartbeat nur über Security-Definer-Verträge mit DB-Uhr und begrenzter TTL, vollständige Produktionsprofile offen |
| 0/1 | Administrative Betriebsbeobachtung | 65 % | über 50 % | Durch gültige Admin-Session und Permission geschützte, auditierte Summary für Komponenten, Queues und Migrationen sowie expierende Kafka-Laufzeitbeobachtung; generische Statushistorie und eigener Ops-Executor noch nicht vorhanden |
| 0 | Native TLS-/mTLS-Transportgrenze | 70 % | über 50 % | Runtime und Sicherheitstests; PKI-Lifecycle offen |
| 0 | Backup und Restore | 65 % | über 50 % | verschlüsselter Pfad und Restore-Vertrag; WAL/PITR und Off-Site offen |
| 0 | Release und Linux-Paketierung | 65 % | über 50 % | Paketartefakte und Tests; Kanal-Promotion und Supportvertrag offen |
| 1 | Tenancy und Organisation | 90 % | über 50 % | Modell, RLS und Admin-API; sichtbares Single-Company-Unternehmensprofil bei unverändertem Tenant-Vertrag sowie versionierte, atomare Mutationen mit 409-Schutz für direkte/geplante Kanten und veränderte Nachfahrenreichweite |
| 1 | Party und Membership | 70 % | über 50 % | Modell, Persistenz und Provisionierung; allgemeine Verwaltungsoberfläche begrenzt |
| 1 | Core Identity | 90 % | über 50 % | Passwort, TOTP, Passkeys, Sessions, API-Keys, bekannte Single-/Multi-Factor-Admin-Assurance mit fail-closed `unknown`, nicht blockierende MFA-Empfehlung, atomare Einmal-Aktivierung und sichere Einladungs-Neuausgabe; externe Adapter, eigenständiger Widerruf, produktive Zustellung sowie vollständige Recovery-/Credential-Lebenszyklen offen |
| 1 | Autorisierung und RBAC | 75 % | über 50 % | Policy, Registry-Bindung, RLS und reale Verbraucher; Policy-Facts offen |
| 1/2 | Organisationsgruppen und App-Zugriff | 45 % | **unter 50 %** | Modell, Migration sowie pure Gate- und tenant-/work-gebundene Koordinatenauflösung mit vollständigen aktiven Elternpfaden, Gruppenauflösung und fail-closed Prüfungen; PostgreSQL-Koordinatenstore, App-Verwaltungs-API/-UI und erster Fachapp-Verbraucher fehlen |
| 1/2 | Ressourcen- und Compliance-Registry | 65 % | über 50 % | persistente Registrierungen, fail-closed Policy-Nutzung und erster gebundener BusinessObjectView-Vertrag; Governance offen |
| 1/2 | Audit | 75 % | über 50 % | Security- und Business-Grundlage, Timeline, Export und auditierte Admin-Beobachtung; Retention/Reconciliation offen |
| 2 | Transactional Outbox und Worker | 75 % | über 50 % | Leasing, Partitionierung, Retry, Dead Letter, Receipts und betriebliche Zustandsprojektion; Scheduler getrennt offen |
| 2 | Kafka-Event-/Audit-/Log-Export | 60 % | über 50 % | Adapter, getrennte fail-fast Topic-Prüfung und verlustgezählter Log-Shutdown; Clusterbetrieb, vollständige ACL-/Retention-Betriebsprüfung und Lag-Alarmierung offen |
| 2 | Service-/Provider-Registry | 45 % | **unter 50 %** | globaler Vertrag, Migration und pure Auflösung; Runtime-Reader, Verwaltung und Domänenverbraucher fehlen weiterhin |
| 2 | Certificate-, Key- und Secret-Service | 30 % | **unter 50 %** | typisierte Ports und Unit-Tests; Registry-Reader, Provideradapter und Verbrauch fehlen |
| 2 | Business-Objektprojektionen | 55 % | über 50 % | Core-Modell, Migration 36, ownergebundener Store und versionierte Work-Query für die erste Workspace-Projektion; weitere Producer offen |
| 2 | Business-Relationen | 10 % | **fehlt** | konzeptionell beschrieben; kein ausführbarer Vertrag |
| 2 | Decision Records | 10 % | **fehlt** | konzeptionell beschrieben; kein ausführbarer Vertrag |
| 2 | Suche und Projektionen | 10 % | **fehlt** | Ziel und Sicherheitsgrenze dokumentiert; kein Index-/Rebuild-/Löschpfad |
| 2 | Cache-/Realtime-Infrastruktur | 25 % | **unter 50 %** | neutraler Cache-Port, optionaler Valkey-Adapter und sicherer PostgreSQL-Fallback für negative Sessionhinweise; Realtime, Metriken und produktiver Adaptertest fehlen |
| 3 | Documents Application Service | 45 % | **unter 50 %** | Modell, Persistenz, Sichtbarkeit und Lese-API; Schreib-/Publish-Pfad fehlt |
| 3 | Storage und Object-Provider | 30 % | **unter 50 %** | Blobmodell und Migration; kein produktiver Byte-, Provider- oder Reconciliation-Pfad |
| 3 | Job- und Scheduler-Service | 15 % | **fehlt** | Outbox-Worker als Grundlage; kein allgemeiner Job-/Zeitvertrag |
| 3 | Aufgaben und Inbox | 10 % | **fehlt** | Datenmodell und Roadmap; kein Codepfad |
| 3 | Workflow Runtime | 10 % | **fehlt** | konzeptionelles Modell; kein Codepfad |
| 3 | Approval Checkpoints und JIT | 25 % | **unter 50 %** | ADR und ausführlicher Vertrag; keine Persistenz oder Runtime |
| 3 | Benachrichtigungen | 5 % | **fehlt** | nur Zielmodell; kein gemeinsamer Zustellvertrag |
| 3 | Collaboration und Sync | 20 % | **fehlt** | Arbeitskopien und Grenzen dokumentiert; keine Runtime-Mechanismen |
| 5 | Integrationen und Webhooks | 10 % | **fehlt** | Zielmodell; keine Provider-/Zustellruntime |
| 5 | Plugin Runtime | 20 % | **fehlt** | ADR, Capability-Grenzen und Zielmodell; keine Sandbox oder Lifecycle-Runtime |
| 5 | AI Advisor Runtime | 15 % | **fehlt** | Akteur-, Daten- und Toolgrenzen dokumentiert; keine Runtime |
| 6 | Platform Witness und HA-Koordination | 25 % | **unter 50 %** | ADRs, Typen und lokale Liveness; Lease, Fencing, Replikation und Drills fehlen |
| 6 | Native Work-Clients | 20 % | **fehlt** | ADR und Clientarchitektur; keine Kotlin-/Compose-Codebasis |

## 3. Fehlende Dienste

Im Sinne der Bewertungsregel fehlt ein ausführbarer dienstspezifischer
End-to-End-Pfad für:

1. Business-Relationen,
2. Decision Records,
3. Suche und Suchprojektionen,
4. Realtime-Infrastruktur jenseits des ersten ausführbaren Cachepfads,
5. Job- und Scheduler-Service,
6. Aufgaben und Inbox,
7. Workflow Runtime,
8. Benachrichtigungen,
9. Collaboration und Sync,
10. Integrationen und Webhooks,
11. Plugin Runtime,
12. AI Advisor Runtime,
13. native Work-Clients.

„Fehlt“ heißt nicht, dass keine Architekturarbeit existiert. Mehrere dieser
Dienste besitzen bereits verbindliche Grenzen, aber noch keine ausführbare
Dienstkette.

`BusinessObjectView` gehört nicht mehr in diese Liste: Der erste ausführbare
Schnitt für die tenantgebundene Workspace-Ressource besitzt Core-Modell,
Persistenz, RLS, Berechtigungsprüfung und eine versionierte Work-Query. Die
Breite des Dienstes bleibt wegen des einzelnen registrierten Producers dennoch
begrenzt.

## 4. Vorhandene Dienste unter 50 %

| Dienst | Reife | Kleinste sichtbare Ausbaugrenze |
|---|---:|---|
| Organisationsgruppen und App-Zugriff | 45 % | PostgreSQL-Koordinatenstore und autorisierte App-Verwaltungs-API plus erster echter App-Zugriffsverbraucher |
| Service-/Provider-Registry | 45 % | datenbankgebundener Runtime-Reader plus genau ein typisierter Domänenverbraucher |
| Documents Application Service | 45 % | atomare Dokumentmutation mit Autorisierung, Audit und Outbox |
| Certificate-, Key- und Secret-Service | 30 % | Registry-gebundener Reader und erster begrenzter TLS-Verbraucher |
| Storage und Object-Provider | 30 % | sicherer Upload zu einem selbst gehosteten Provider mit Reconciliation |
| Approval Checkpoints und JIT | 25 % | tenantgesicherter Checkpoint mit einer idempotenten Entscheidung |
| Platform Witness und HA-Koordination | 25 % | noch kein kleiner produktiver Schnitt ohne Replikations-/Fencing-Gesamtvertrag |

Die neue Übersicht der Anmeldeanbieter ändert die 45-%-Bewertung der globalen
Service-/Provider-Registry nicht. Sie ist eine sichere, read-only Sicht auf die
Identity-eigenen Provider und Laufzeitfunktionen. Sie konsumiert weder den
globalen Service-/Capability-Vertrag noch aktiviert sie einen OIDC-, SAML- oder
LDAP-Adapter.

## 5. Verhältnis zur Codebasis

Der sichtbare Code konzentriert sich auf das Fundament:

- 15 Core-Packages besitzen jeweils mindestens einen Unit-Test; neu sichtbar
  sind insbesondere `businessobject` und `operations`,
- 16 Plattform-Packages decken Datenbank, HTTP, Identity, Audit, Outbox, Kafka,
  Migration, Konfiguration, TLS, Business-Objektprojektionen, Betriebszustand
  und weitere Stores ab,
- 43 additive Migrationen reichen von Tenancy und Identity über WebAuthn und
  `BusinessObjectView` bis zur administrativen Core-Beobachtung; die letzten
  fünf schließen Provider-/Session-Gates (`000038`), den privilegienarmen
  Heartbeat-Ausführungsvertrag (`000039`), den ausdrücklich gewährten
  Identity-Zugriff auf die Sicherheitsfunktionen (`000040`) und die
  Build-Version-Angleichung (`000041`) sowie den digestgebundenen initialen
  Work-Konto-Aktivierungsvertrag (`000042`),
- der OpenAPI-Vertrag veröffentlicht Identity-, Admin-, Workspace-, Audit- und
  Dokument-Leseendpunkte sowie die Business-Objekt-Query, Provider-Übersicht,
  Betriebsübersicht, den Work-Kontostatus-, Einladungsaktivierungs- und
  Einladungs-Neuausgabevertrag,
- es existiert weiterhin kein Package für Business Relations, Decisions,
  Search, Jobs, Workflows, Notifications, Plugins, AI oder Collaboration.

`go.mod` und `go.sum` sind vorhanden. Go-Paketauflösung und automatisierte Tests
sind in diesem Arbeitsstand ausführbar; Unit- und Vertragstests wurden
erfolgreich ausgeführt. PostgreSQL-Integrationen bleiben zusätzlich an
ausdrücklich konfigurierte Wegwerf-Instanzen gebunden. Die Prozentwerte stützen
sich damit auf aktuellen Quellcode, vorhandene negative Sicherheits- und
Integrationstests sowie ausführbare Prüfläufe, nicht nur auf frühere
Dokumentationsaussagen.

## 6. Frühester offener Dienst nach Roadmap

Die erste noch nicht umgesetzte Dienstgrenze in Roadmap-Phase 2 ist die
`BusinessRelation`. Der erste `BusinessObjectView` folgt dem vorhandenen
Modul-/Ressourcentypregister inzwischen als ausführbare, autorisierte gemeinsame
Sicht. Die Relation ist der nächste offene Vertrag vor Decision Records, Suche
und den Phase-3-Arbeitsdiensten.

Ein kleinster End-to-End-Schnitt müsste mindestens nachweisen:

```text
registrierter, versionierter Relationstyp mit erlaubten Quell- und Zieltypen
  -> zwei explizite tenantgebundene ResourceRefs desselben Tenants
  -> genau ein Owner sowie fachliche Gültigkeit und Aufzeichnungshistorie
  -> atomare Mutation mit Autorisierung, Audit und Outbox
  -> PostgreSQL-RLS und versionierte, serverseitig autorisierte Query
  -> negative Cross-Tenant-, Permission- und Fremdtyp-Tests
  -> definierte Ablösung statt stiller historischer Überschreibung
```

Dieser Relationsschnitt ist noch nicht implementiert. Vor einem Schema- und
API-Entwurf sind erster Relationstyp, Owner, Kardinalität und fachlicher
Verbraucher zu bestätigen; diese Entscheidungen würden sonst eine langfristige
Core-Grenze vorwegnehmen.

## 7. Reihenfolge nach bestätigten Abhängigkeiten

Ohne die offenen Architekturentscheidungen vorwegzunehmen, ergibt sich aus der
Roadmap folgende Abhängigkeitsfolge:

1. Business-Relationen auf der vorhandenen autorisierten Objekt- und
   Ressourcenbasis,
2. Decision Records und Approval-Verknüpfung,
3. Suchprojektionen auf autorisierten Objektansichten,
4. Phase-3-Dienste für Jobs, Dokumentmutation, Aufgaben und Workflows,
5. Collaboration erst auf einem vollständigen Dokument-Publish-Pfad.

Die Reihenfolge ist eine Lesart der bestätigten Roadmap, keine Freigabe aller
genannten Implementierungen.
