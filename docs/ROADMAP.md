# WERK – Umsetzungsroadmap

**Status:** Startplanung  
**Ziel:** Einen sicheren, selbst hostbaren Plattformkern liefern, auf dem
Fachanwendungen ohne parallele Grundstrukturen wachsen können.

Diese Roadmap folgt [Vision](vision.md) und
[Datenmodell](DATENMODELL.md). Sie priorisiert irreversible Grundlagen vor
fachlicher Breite: Identität, Datenhoheit, Audit, Ereignisse, Historie und
Betrieb werden vor CRM, HRM oder Finance umgesetzt.

## Leitlinie für jede Phase

- Go-Modularmonolith, PostgreSQL als fachliche Wahrheit, React/TypeScript-Frontend
  und versionierte HTTP/JSON-Business-API.
- Die eigenständige Weboberfläche und spätere Kotlin-/Compose-Multiplatform-
  Clients verwenden dieselben versionierten Verträge und Sicherheitsgrenzen.
  Native Clients erhalten weder Sonder-APIs noch direkten Datenzugriff.
- Ein Feature gilt erst als fertig, wenn es Tenant-Kontext, Autorisierung, Audit,
  API-Vertrag, Tests und Betriebsverhalten berücksichtigt.
- Valkey ist Infrastruktur hinter Core-Ports, niemals die alleinige Quelle
  fachlicher Wahrheit.
- Admin- und Arbeitsbereich bleiben technisch, organisatorisch und in den APIs
  getrennt.

## Phase 0 – Architektur und Lieferfähigkeit

**Zweck:** Ein reproduzierbares Projekt schaffen, bevor Produktfunktionen entstehen.

- Go- und Frontend-Workspace mit lokaler Entwicklungsumgebung, Docker-Compose-
  Profil und getrennten Diensten für App, PostgreSQL und Valkey anlegen.
- CI für Formatierung, Unit-Tests, API-Vertragsprüfung, Datenbankmigrationen und
  Container-Build einrichten.
- Standard für Konfiguration, Secrets, strukturierte Logs, Request-/Correlation-
  IDs und Fehler nach RFC 9457 festlegen.
- ADRs für Mandant/RLS, Kontoarten, API-/Event-Versionierung, Outbox/Queue,
  Object Storage, Plugins, KI, Backup, native Clients und das spätere
  domänengebundene Platform-Witness-Modell erstellen.
- Health-, Readiness- und Metrik-Endpunkte sowie ein Restore-Test-Skript liefern.

**Abnahme:** Eine neue Instanz startet per dokumentiertem Befehl, führt
Migrationen aus, besteht Healthchecks und kann aus einem verschlüsselten Backup
wiederhergestellt werden.

**Umsetzungsstand 2026-07-19:** Workspace, getrennte Images, lokale Netze,
Health/Readiness, interne Metriken, strukturierte HTTP-Logs, Request- und
Correlation-IDs, Problem Details, Konfigurationsvalidierung, OpenAPI-Grundvertrag,
Migrationssperre und CI-Basis sind vorhanden. Ein eigener, RLS-vollständiger
Backup-Leser, ausschließlich verschlüsselte `age`-Streams und ein automatischer
Restore-Test für Daten, Migrationen, Grants und Tenant-Isolation sind vorhanden.
Die ADRs für Outbox/Ereignisse, Object Storage, Erweiterungen/Plugins und KI
liegen ebenfalls vor. Die spätere native Clientstrategie ist mit ADR-013 auf
Kotlin Multiplatform und Compose Multiplatform begrenzt. Damit ist Phase 0 für
das definierte Single-Host-Startprofil abgeschlossen. Off-Site-/WAL-/PITR-
Sicherungen und der betriebliche
Restore-Drill bleiben bewusst spätere Betriebsreife-Ausbaustufen.

Das spätere Active/Passive-Identity-Modell ist mit ADR-015 und ADR-022 auf eine
einzige schreibende Autorität, die Domain `identity-control` eines unabhängigen
QDevice-artigen Platform Witness, Lease, Autoritätsgeneration und Fencing
festgelegt, aber noch nicht implementiert.

Eine erste SemVer-gesteuerte Release-Pipeline veröffentlicht nach den
vollständigen CI-, Migrations- und Restore-Prüfungen Linux-Artefakte und
getrennte Multi-Arch-Images mit Prüfsummen, SBOM und signierten
Herkunftsnachweisen. Automatisches Deployment, formale Kanal-Promotion und ein
zugesagter Security-Supportzeitraum bleiben Ausbaustufen der Produktreife;
Änderungen vorbehalten.

## Phase 1 – Sicherheits- und Organisationskern

**Zweck:** Eindeutige Unternehmensgrenzen und eine nicht umgehbare Trennung von
Arbeits- und Administrationsebene etablieren.

- Tenant, Organisationseinheit, Person, Organisation und Membership umsetzen.
- Getrennte `work`-, `admin`- und `service`-Konten sowie tenantgebundene,
  nicht interaktive `agent`-Principals mit getrennten Authentifizierungsarten,
  Audiences und API-Grenzen implementieren.
- Core Identity als interne Identitätsquelle mit Passwort-/Session-Vertrag,
  MFA für Admin-Sitzungen und optionalen Provider-Adaptern ergänzen.
- Berechtigungsregistrierung, Rollen, Scopes, Policy-Prüfung und PostgreSQL RLS
  mit Non-Owner-App-Rolle implementieren.
- Admin-Portal und Workspace als getrennte Frontend-Oberflächen bereitstellen.
- Audit-Protokoll für sicherheits- und fachlich relevante Aktionen liefern.

**Abnahme:** Ein Arbeitskonto kann keine `/admin/v1`-Aktion ausführen; ein
Admin-Konto kann keinen Workspace- oder Fachendpunkt ausführen. Tenant-übergreifende
Zugriffe scheitern in Anwendung und Datenbank.

**Umsetzungsstand 2026-07-21:** Getrennte Owner-, Migrator-, Identity-, Work-,
Admin-, Service-, Worker- und Backup-Datenbankrollen, `FORCE ROW LEVEL SECURITY`,
restriktive Tenant-Gates, ein transaktionsgebundener Go-Datenbankzugriff,
tenantgebundene Party-/Person-/Organisations-/Membership-Tabellen, persistente
Account-/Credential-/Session-Tabellen, ein providerunabhängiger
Authentication-Vertrag, Argon2id-Passwort-Hashing, atomarer Dev-Bootstrap,
persistente Session-Ausstellung und -Auflösung, Passwortwechsel, Logout sowie
geschützte HTTP-API-Grenzen sind vorhanden. TOTP-MFA für Administrationskonten
ist mit verschlüsselten Secrets, Recovery-Codes, kurzlebiger Login-Challenge,
Session-Assurance, persistentem Brute-Force-Schutz, Origin-/Fetch-Metadata- und
CSRF-Prüfung ausführbar. Das RBAC-Fundament registriert versionierte
Berechtigungen, Rollen und Zuweisungen mit Access-Plane-, Tenant- und
Scope-Invarianten in Anwendung und Datenbank. Ein MFA-autorisierter Admin kann
über `/admin/v1/work-users` ein Work-Konto samt Person, Membership,
temporärem Passwort und tenantgebundener Workspace-Rolle atomar anlegen, ohne
fachliche Benutzerrechte zu erhalten. Login, MFA, Session-Ausstellung, Logout,
Passwortwechsel und Provisionierung erzeugen transaktionale Security-/Domain-
Audit- beziehungsweise Outbox-Einträge. Mandanten und hierarchische
Organisationseinheiten können über getrennt autorisierte Admin-Verträge
aufgelistet und angelegt werden; jede Mutation benötigt einen expliziten
Tenant-Kontext und schreibt Audit plus Outbox atomar. Die allgemeine Rollen- und
Zuweisungsverwaltung ist für tenantgebundene Work-Rollen verfügbar: frei
verwaltbare Rollen verwenden ausschließlich registrierte Work-Berechtigungen,
Systemrollen bleiben unveränderlich und aktive Zuweisungen werden atomar mit
Audit und Outbox ersetzt. Die Workspace-API verwendet
`core.workspace.access` praktisch, übernimmt den Tenant ausschließlich aus der
Work-Session und lädt Mandant, Organisationseinheit und Membership über die
Work-Runtime mit RLS. Die Benutzeroberfläche stellt diesen bestätigten Kontext
dar; Admin-Sessions und tenantfremde Konten scheitern. Mandanten,
Organisationseinheiten und frei verwaltbare Work-Rollen besitzen vollständige
`PUT`-Änderungsverträge mit starkem `If-Match`, atomarer Versionserhöhung,
Audit und Outbox. Hierarchiezyklen und das Archivieren noch verwendeter
Organisationseinheiten werden abgelehnt; Systemrollen bleiben auch auf
Datenbankebene unveränderlich. Suspendierte oder archivierte Mandanten sperren
tenantgebundene Work-Sessions bei der Actor-Auflösung. Eine konsolidierte,
MFA- und berechtigungsgeschützte Audit-Timeline stellt Security-Ereignisse
installationsweit mit Tenant-Kennzeichnung, begrenzter Cursor-Paginierung und
ohne freie Detail- oder Session-Rohdaten bereit. Jeder Abruf wird atomar selbst
auditiert. Konkrete externe Provider-Adapter werden erst nach diesem Kern
ergänzt.

Der plattformweite Ressourcen- und Autorisierungsvertrag registriert außerdem
Core-/App-Namensräume, Ressourcentypen mit expliziter Installations- oder
Tenant-Grenze und die zulässige Zuordnung jeder Berechtigung zu ihren
Ressourcentypen. Der Go-Kern leitet den Plattformkontext aus dem Actor ab und
entscheidet mit einer typisierten `ResourceRef` fail-closed. Die bestehenden
Admin- und Workspace-Verbraucher verwenden diese Basis. Konfigurierbare
Policy-Facts und App-Manifeste sind noch offen; Änderungen vorbehalten.

Der Identity-Vertrag trennt registrierte Kontoarten, Providerbindungen,
mehrfache Credentials, Audiences und Authentifizierungsmethoden. Persistente,
tenantgebundene Agenten verwenden den technischen API-Bereich; API-Schlüssel
besitzen Ablauf, Widerrufsstatus und ein über alle Prozesse derselben
autoritativen PostgreSQL-Datenbank atomar gezähltes Nutzungslimit. Das ist noch
keine Konsistenzzusage für zwei getrennte Datenbankkopien. Provider dürfen
weiterhin keine Kontoart, Tenant-Zuordnung oder Audience liefern. WebAuthn
bleibt als Daten- und Domänenvertrag vorbereitet. Faktorwiderruf,
beaufsichtigte Wiederherstellung und Recovery-Code-Erneuerung benötigen noch
eigene re-authentifizierte Verwaltungsabläufe. In Produktion erzwingt die
Konfigurationsvalidierung aktiviertes MFA, einen 32-Byte-Verschlüsselungsschlüssel
und explizite erlaubte Origins.

**Organisations- und App-Zugriffsgrundlage 2026-07-21:** Hierarchische
Organisationseinheiten bleiben die inneren Unternehmensschalen eines Tenants.
Tenantgebundene Access-Gruppen bilden querliegende Kanten aus Work-Konten und
Organisationseinheiten. Plattformregistrierte Apps können tenantbezogen
aktiviert und über ein separates App-Entitlement ausdrücklich für Konto,
Einheit oder Gruppe geöffnet werden. Zusammengesetzte Tenant-Fremdschlüssel,
RLS und ein purer Go-Entscheidungsvertrag sind vorhanden. Ein Entitlement
erteilt keine Rolle oder Fachberechtigung. Verwaltungs-API, UI, delegierte
Gruppenverwaltung und der erste Fachapp-Verbraucher bleiben offen; Änderungen
vorbehalten.

## Phase 2 – Gemeinsame Daten- und Ereignisplattform

**Zweck:** Zusammenhänge, Änderungen und historische Entscheidungen über alle
späteren Fachmodule hinweg verständlich machen.

**Async-Grundlage 2026-07-20:** Transactional-Outbox-Schema, versionierte
Event-Verträge, PostgreSQL-Leasing, partitionsgeordnete parallele Worker,
idempotente Consumer-Receipts, Retry/Backoff und Dead-Letter-Zustand sind als
globale Grundlage vorhanden. Fachliche Event-Registrierung, Replay-Verwaltung,
Metriken und der optionale Valkey-Adapter bleiben Teil des weiteren Phase-2-
Ausbaus.

**Ressourcenbasis 2026-07-21:** Das globale Modul- und Ressourcentypregister
sowie die Permission-Ressourcentyp-Bindung sind als erster Teil umgesetzt.
Ein verpflichtendes Datenprofil je Ressourcentyp klassifiziert zusätzlich
personenbezogene Daten und Vertraulichkeit; fehlt es, scheitert die
Autorisierungsauflösung fail-closed. Zusätzlich besitzt jede aktuelle
Permission-Ressourcentyp-Kombination eine serverseitige Processing-Policy. Die
gemeinsame Core-Entscheidung prüft Actor, Permission, Ressource, Datenprofil,
Processing-Policy und Grant. Das betreiberseitig freigegebene
Verarbeitungsverzeichnis sowie einzelne Business-Objektregistrierungen,
`BusinessObjectView`, Relationen, Policy-Facts und Suchprojektionen bleiben
offen. Änderungen vorbehalten.

**Service-/Provider-Registry 2026-07-22:** Der globale, versionierte
Metadatenvertrag trennt logische Dienste, technische Capabilities,
service-spezifische Providerinstanzen und deren ausdrückliche Bindungen. Die
Auflösung ist provider-ID-, registry-vertrags-, dienst-, capability-, boundary-
und tenantgenau und erhält diese Koordinaten gemeinsam im Ergebnis;
automatische Providerwahl, Health-Failover, Providerkonfiguration und Geheimnisse sind kein
Teil der Registry. Das Go- und PostgreSQL-Fundament ist ownerverwaltet und noch
an keinen bestehenden Identity-, Storage-, Kafka- oder Certificate-Adapter
gekoppelt. Vor einer solchen Kopplung wird genau ein Domänenverbraucher mit
Audit-, Outbox-, RLS- und Betriebsvertrag integriert; Änderungen vorbehalten.

**Secure-Material-Vertrag 2026-07-22:** Certificate-, Signaturschlüssel- und
Secretzugriff besitzen getrennte typisierte Go-Ports auf der Registry-Grammatik.
Anfragen führen Provider-/Binding-Revision als Diagnosekoordinaten mit und sind
an exakte Materialversion, Zweck und Operationsgrenze gebunden. Vor
sicherheitsrelevanter Nutzung wird der vollständige Registry-Vertrag erneut
aufgelöst; private Schlüssel werden nicht exportiert und eine allgemeine
`secret.read`-Fähigkeit ist ausgeschlossen. Datenbank-Seeds,
Runtime-Reader und Adapter bleiben bis zum ersten eng begrenzten nativen
TLS-Verbraucher bewusst offen; Änderungen vorbehalten.

- Ressourcenregistrierung und `BusinessObjectView` für Navigation, Suche und
  autorisierte Kontextansichten implementieren.
- `BusinessRelation` mit registrierten Relationstypen, Owner, Gültigkeitszeit und
  Historie implementieren.
- `DecisionRecord` als wiederverwendbare Capability mit Evidenzen,
  Policy-Version, Entscheidern und Gültigkeitszeit implementieren.
- Versionierte Verträge für globale `ApprovalCheckpoint`s und deren Ereignisse
  registrieren; Kontoart und Tenant-Grenzen im Vertrag explizit machen.
- Versionierte Domain-Events, Transactional Outbox, idempotente Consumer,
  Retries und Dead-Letter-Handling implementieren.
- Kafka als mitgelieferten Distributionspfad für versionierte Domain-Events,
  minimierte Security-Audits und strukturierte Betriebslogs betreiben;
  gemeinsames Tagging, getrennte Topics und at-least-once-Deduplizierung
  verbindlich testen.
- Globalen Suchindex auf Basis berechtigungsgeprüfter Objektprojektionen liefern.
- Valkey-Adapter für `CachePort`, `SessionPort`, `RealtimePort` und `QueuePort`
  ergänzen; degradiertes Verhalten und Wiederanlaufverhalten testen.

**Abnahme:** Ein Fachmodul kann ein Objekt, eine Relation und ein versioniertes
Ereignis veröffentlichen. Nach Neustart oder Valkey-Ausfall bleiben Fachdaten und
Audit vollständig, Consumer können Ereignisse ohne doppelte Wirkung nachholen.

**Umsetzungsstand 2026-07-22:** Transactional Outbox, partitionierte Worker-
Leases, Consumer-Receipts, Retry und Dead Letter sind implementiert. Das
Single-Host- und native Entwicklungsprofil bringt einen persistenten Kafka-
KRaft-Knoten mit. Domain-Events werden mit versioniertem Envelope und
konservativem Tagging publiziert; Security-Audits besitzen eine atomare,
minimierte Export-Queue und Betriebslogs einen nicht blockierenden, geschwärzten
Kafka-Pfad. Mehrhost-Cluster, Schema-Registry, SIEM-Verbraucher und
betriebliche Lag-Alerts bleiben weitere Betriebsreife; Änderungen vorbehalten.

## Phase 3 – Arbeit koordinieren

**Zweck:** Den universellen Arbeitsfluss von WERK bereitstellen, ohne bereits eine
bestimmte Fachanwendung zu erzwingen.

- Dokumente, Versionen, Object Storage, Klassifikation und Aufbewahrungsregeln
  implementieren.
- Aufgaben, Zuweisungen, Fristen, Benachrichtigungspräferenzen und Echtzeit-
  Aktualisierung implementieren.
- Versionierte Workflow-Definitionen, Instanzen, Freigaben, Eskalationen und
  Entscheidungsprotokolle implementieren. Fachliche Kontrollpunkte verwenden
  ausschließlich `work`-Konten und unterstützen Policy-gesteuerte
  Re-Authentifizierung, Mehrpersonenregeln sowie kurzlebige,
  ressourcengebundene Just-in-Time-Grants.
- Formulare, Kommentare und Akten als Capabilities auf Core-Diensten aufbauen.
- Workspace mit globaler Suche, Inbox, Aufgaben, Benachrichtigungen und
  Ressourcenverweisen liefern.

**Dokument-/Storage-Fundament 2026-07-22:** Core Documents und Core Storage sind
als getrennte logische Dienste im modularen Monolithen festgelegt. Der erste
inaktive Fundamentschnitt ist implementiert: veröffentlichte unveränderliche
Versionen, tenantgebundene versiegelte Blobs mit fail-closed `unknown`- und
`missing`-Zuständen, opake Locations, Klassifikationshistorie,
Ressourcen-/Permission-Registrierungen und `FORCE RLS`. Die fachliche
Auditbasis trennt inzwischen `initiated_by` und `executed_by`, bindet die
tenantgebundene Ressource und prüft den Policy-/Processing-Snapshot
serverseitig. Noch kein Dokument-Application-Service erzeugt diese Einträge;
er muss erfolgreiche Mutation, Audit und Outbox später atomar verbinden. Ein
öffentlicher Bytepfad wird erst mit diesem Producer, Einmaltickets,
Quarantäneprüfung, S3-Adapter und koordiniertem
PostgreSQL-/Object-Store-Restore freigegeben.
Die Work-UI folgt demselben Vertrag schrittweise mit Dokumentliste,
Detail-/Versionsansicht, Klassifikation und anschließend sicheren
Upload-/Downloadzuständen; sie erhält keinen direkten Storage-Zugriff.
Collaboration und Sync folgen danach als Arbeitskopien auf diesem Vertrag;
Änderungen vorbehalten.

**Abnahme:** Ein generischer Vorgang kann Dokumente enthalten, Beziehungen zu
anderen Objekten haben, einen Workflow starten, Aufgaben erzeugen, eine
begründete Entscheidung erhalten und revisionssicher abgeschlossen werden.

## Phase 4 – Erster fachlicher End-to-End-Pilot

**Zweck:** Den Core an einem echten Unternehmensablauf beweisen, ohne ihn für ein
Fachmodul zu verbiegen.

- Einen realen Pilotprozess auswählen und als eigenständiges App-Modul umsetzen.
- Das Modul verwendet ausschließlich Core-APIs für Identität, Rechte, Dokumente,
  Aufgaben, Workflows, Entscheidungen, Suche, Audit und Events.
- Fachobjekte, Fachregeln, API, UI und Reporting-Projektionen bleiben Eigentum
  des Pilotmoduls.
- Der Pilotvertrag bleibt clientneutral. Eine klar begrenzte Work-Funktion kann
  als erster Android-/iOS-Client-Slice erprobt werden, sobald ein separates
  Native-Authentifizierungs-ADR angenommen ist; dies ist keine Voraussetzung
  für die fachliche Phase-4-Abnahme.
- Migrations-, Import- und Schulungsablauf für eine getrennte Testinstanz liefern.
- Nutzung, Fehler, Durchlaufzeiten und manuelle Umgehungen im Pilot messen.

**Abnahme:** Der ausgewählte Ablauf kann von einem berechtigten Arbeitskonto
vollständig in WERK ausgeführt werden. Es gibt keine Tabellenkopplung oder
Parallelimplementierung einer Core-Fähigkeit.

## Phase 5 – Kontrollierte Erweiterbarkeit und KI

**Zweck:** Erweiterungen und KI integrieren, ohne die Sicherheits- oder
Verantwortungsgrenzen zu lockern.

- Plugin-Manifest, Signaturprüfung, Capability-Registrierung und widerrufbare
  `PluginCapabilityGrant`s implementieren.
- Zunächst einen isolierten Ausführungsweg wählen: WASM für begrenzte Logik,
  Sidecar für Integrationen; keine native Go-Plugin-ABI.
- Webhooks, API-Tokens, Service-Subjekte, Import/Export und Integration-Runs
  bereitstellen.
- `AiAgent`, `AiRun` und `AiActionProposal` implementieren.
- KI-Read-Tools, Datenklassifikation, Redaction, Tool-Policies, menschliche
  Freigaben und deterministische Ausführung implementieren.

**Abnahme:** Ein Plugin kann ausschließlich erteilte Fähigkeiten nutzen. Eine
KI kann eine Aktion erklären und vorschlagen, aber keine schreibende Aktion ohne
gültige Policy und gegebenenfalls Freigabe ausführen.

## Phase 6 – Fachanwendungen und Produktreife

**Zweck:** Den bewährten Kern schrittweise mit Fachanwendungen und belastbarem
Betrieb erweitern.

- Fachanwendungen in priorisierter Unternehmensreihenfolge entwickeln, etwa CRM,
  Projekte, Tickets, Assets/CMDB, HRM, Finance, BI und BCP.
- Für jedes Modul Ressourcen, Beziehungen, Events, Berechtigungen,
  Aufbewahrungsklassen und Integrationsverträge registrieren.
- Reporting-Projektionen und BI-Exports aufbauen, ohne operative Fachmodelle zu
  duplizieren.
- Produktiv-, Vorschau- und Testkanal, signierte Releases, SBOM, Restore-Drills,
  SLOs, Runbooks und Upgradepfade etablieren.
- Den nativen `work`-Client nach ADR-013 zuerst für Android und iOS/iPadOS
  produktionsreif liefern; Windows, macOS und Linux folgen nach erfolgreichem
  Mobile-Pilot mit signiertem Packaging und kontrolliertem Updatekanal.
- Native Offline-Funktionen nur pro Use Case mit verschlüsselter Projektion,
  Idempotenz, Konfliktbehandlung und erneuter serverseitiger Policy-Prüfung
  freigeben.
- Optional erst danach: Fleet und Marketplace.
- Das HA-/Mehrinstanz-Betriebsprofil als eigenen Abnahmeschnitt liefern:
  stabile Realm-/Instanzkennungen, replizierte PostgreSQL-Wahrheit,
  QDevice-artigen Platform Witness mit `identity-control`, exklusive Lease,
  monotone Autoritätsgeneration, Fencing sowie auditierte manuelle und
  automatische Promotion.
- Netztrennung, Witness-Ausfall, Replikationsverzug, Schlüsselrotation,
  Rückkehr der alten Hauptinstanz und Wiederherstellung in automatisierten
  Failover-Drills prüfen. Ohne Witness bleibt der Zwei-Instanz-Failover manuell
  und fail-closed.
- Die native TLS-/mTLS-Basis aus
  [`ADR-023`](adr/ADR-023-native-server-tls-und-transportidentitaet.md) um eine
  versionierte Instanz-/Realm-Zuordnung, Zertifikatsausstellung, Sperrung und
  Rotationstests für den echten Control-Plane-Transport ergänzen.

**Abnahme:** Mehrere Fachmodule verwenden nachweisbar denselben Core, Updates
sind wiederholbar und rücksicherbar, und die produktive Instanz erfüllt die
vereinbarten Wiederherstellungs- und Betriebsziele. Ein aktiviertes HA-Profil
weist zusätzlich nach, dass niemals zwei Identity-Autoritäten gleichzeitig
schreiben und ein Failover keine unbestätigten Widerrufe oder exakten
Sicherheitszähler stillschweigend verliert.

## Startreihenfolge nach dieser Roadmap

Der direkte Arbeitsbeginn erfolgt mit Phase 0, dann Phase 1. Phase 2 ist die
erste eigentliche Differenzierung von WERK: Objektgraph, Entscheidungen,
zeitbezogene Historie und Ereignisse schaffen die Grundlage für ein System, das
Zusammenhänge und Auswirkungen sichtbar machen kann. Phase 4 beginnt erst, wenn
ein konkreter Pilotprozess ausgewählt wurde; alle vorherigen Phasen sind davon
fachlich unabhängig.
