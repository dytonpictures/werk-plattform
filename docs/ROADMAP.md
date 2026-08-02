# WERK – Umsetzungsroadmap

**Status:** Startplanung  
**Stand:** 2026-07-29  
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

- Go- und Frontend-Workspace mit nativer lokaler Entwicklungsumgebung und
  getrennten Prozessen für API, Worker und Migration anlegen.
- CI für Formatierung, Unit-Tests, API-Vertragsprüfung, Datenbankmigrationen und
  native Release-Builds einrichten.
- Standard für Konfiguration, Secrets, strukturierte Logs, Request-/Correlation-
  IDs und Fehler nach RFC 9457 festlegen.
- ADRs für Mandant/RLS, Kontoarten, API-/Event-Versionierung, Outbox/Queue,
  Object Storage, Plugins, KI, Backup, native Clients und das spätere
  domänengebundene Platform-Witness-Modell erstellen.
- Health-, Readiness- und Metrik-Endpunkte sowie ein Restore-Test-Skript liefern.

**Abnahme:** Eine neue Instanz startet per dokumentiertem Befehl, führt
Migrationen aus, besteht Healthchecks und kann aus einem verschlüsselten Backup
wiederhergestellt werden.

**Umsetzungsstand 2026-07-26:** Workspace, zentrale `.env`, native Startskripte,
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
vollständigen CI-, Migrations- und Restore-Prüfungen native Linux-Artefakte mit
Prüfsummen und signierten Herkunftsnachweisen. Automatisches Deployment,
formale Kanal-Promotion und ein
zugesagter Security-Supportzeitraum bleiben Ausbaustufen der Produktreife;
Änderungen vorbehalten.

**Administrative Betriebsbeobachtung 2026-07-29:** Das Admin-Portal besitzt
eine durch Admin-Session, Audience und Berechtigung geschützte,
installationsweite Betriebsübersicht.
Sie liest ausschließlich eine bereinigte PostgreSQL-Projektion aus API- und
PostgreSQL-Zustand, Worker-Heartbeat, Domain-Outbox, Security-Audit-Export und
angewendetem Migrationsstand; jeder Abruf wird auditiert. Die Admin-Runtime
erhält keinen Zugriff auf Heartbeat-Zeilen, Queue-Payloads, Fehlertexte, Leases,
Hostdaten oder Prozesskoordinaten. Der Worker schreibt und entfernt seine
Beobachtung ausschließlich über eng begrenzte `SECURITY DEFINER`-Funktionen;
PostgreSQL bestimmt sämtliche Zeitpunkte und begrenzt die Heartbeat-TTL
serverseitig auf höchstens zehn Minuten. Die Worker-Runtime besitzt keinen
direkten Tabellenzugriff, und die persistierte Build-Kennung folgt derselben
128-Zeichen-Grenze wie der öffentliche Betriebsvertrag. Neustart, Update,
Rollback und Hostdiagnose sind bewusst nicht ausführbar. Ein späterer
Ausführungsweg benötigt einen getrennten minimal privilegierten Ops-Agenten
samt Re-Authentifizierung, versioniertem Command-Vertrag, Idempotenz,
signierten Releases, Audit und Rollback. Diese Grenze ist in
[`ADR-029`](adr/ADR-029-administrative-beobachtung-und-ausfuehrungsgrenze.md)
festgelegt.

## Phase 1 – Sicherheits- und Organisationskern

**Zweck:** Eindeutige Unternehmensgrenzen und eine nicht umgehbare Trennung von
Arbeits- und Administrationsebene etablieren.

- Tenant, Organisationseinheit, Person, Organisation und Membership umsetzen.
- Getrennte `work`-, `admin`- und `service`-Konten sowie tenantgebundene,
  nicht interaktive `agent`-Principals mit getrennten Authentifizierungsarten,
  Audiences und API-Grenzen implementieren.
- Core Identity als interne Identitätsquelle mit Passwort-/Session-Vertrag,
  optionaler, selbst gestarteter MFA-Verstärkung für Administrationskonten und
  optionalen Provider-Adaptern ergänzen.
- Berechtigungsregistrierung, Rollen, Scopes, Policy-Prüfung und PostgreSQL RLS
  mit Non-Owner-App-Rolle implementieren.
- Admin-Portal und Workspace als getrennte Frontend-Oberflächen bereitstellen.
- Audit-Protokoll für sicherheits- und fachlich relevante Aktionen liefern.

**Abnahme:** Ein Arbeitskonto kann keine `/admin/v1`-Aktion ausführen; ein
Admin-Konto kann keinen Workspace- oder Fachendpunkt ausführen. Tenant-übergreifende
Zugriffe scheitern in Anwendung und Datenbank.

**Umsetzungsstand 2026-07-29:** Getrennte Owner-, Migrator-, Identity-, Work-,
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
Scope-Invarianten in Anwendung und Datenbank. Ein gültig angemeldeter und
berechtigter Admin kann
über `/admin/v1/work-users` ein Work-Konto samt Person, Membership,
tenantgebundener Workspace-Rolle und einer ausdrücklich gewählten
Bereitstellung atomar anlegen, ohne fachliche Benutzerrechte zu erhalten. Ein
Startpasswort erzwingt den Erstwechsel nur bei explizitem Flag; alternativ
aktiviert ein Work-Benutzer sein zunächst deaktiviertes Konto über einen
einmaligen, nur als Digest gespeicherten Einladungslink mit selbst gewähltem
Passwort. Admins ohne Faktor erhalten eine reguläre Single-Factor-Admin-Sitzung
und eine nicht blockierende Empfehlung zur selbst gestarteten MFA-Einrichtung.
Multi-Factor-Admin-Sitzungen bleiben ebenso zulässig; eine unbekannte
Assurance wird fail-closed abgewiesen. Aktiviertes TOTP bleibt bei späteren
Passwortanmeldungen verpflichtend. Berechtigungen, Ressourcenbindung,
Processing-Policy, Tenant-Kontext, CSRF, Audit und atomare Outbox bleiben von
dieser Zugangsregel unabhängig verbindlich.
Login, MFA, Session-Ausstellung, Logout, Passwortwechsel, Einladungseinlösung
und Provisionierung erzeugen transaktionale Security-/Domain-Audit-
beziehungsweise Outbox-Einträge. Mandanten und hierarchische
Organisationseinheiten können über getrennt autorisierte Admin-Verträge
aufgelistet und angelegt werden; jede Mutation benötigt einen expliziten
Tenant-Kontext und schreibt Audit plus Outbox atomar. Das sichtbare
Single-Company-Profil bezeichnet den bestätigten Tenant als Unternehmen. Nur
wenn insgesamt genau ein Verzeichniseintrag vorhanden und aktiv ist, wird er
automatisch gewählt und der Unternehmenswähler ausgeblendet; eine Neuanlage ist ausschließlich beim
bestätigten Leerbestand sichtbar, abweichender oder unbekannter Bestand wird
defensiv behandelt. Core, Datenbank, API, RLS und Audit behalten den technischen
Tenant-Vertrag; eine zusätzliche harte serverseitige Mengeninvariante bleibt
offen. Die allgemeine Rollen- und
Zuweisungsverwaltung ist für tenantgebundene Work-Rollen verfügbar: frei
verwaltbare Rollen verwenden ausschließlich registrierte Work-Berechtigungen,
Systemrollen bleiben unveränderlich und aktive Zuweisungen werden atomar mit
Audit und Outbox ersetzt. Dieser UI-/API-Vertrag verwaltet ausschließlich
`scope_type='tenant'`; vorhandene Organisations- oder Ressourcen-Zuweisungen
werden nicht eingelesen, beendet oder auf Tenant-Reichweite erweitert. Die Workspace-API verwendet
`core.workspace.access` praktisch, übernimmt den Tenant ausschließlich aus der
Work-Session und lädt Mandant, Organisationseinheit und Membership über die
Work-Runtime mit RLS. Die Benutzeroberfläche stellt diesen bestätigten Kontext
als Unternehmen dar; Admin-Sessions und tenantfremde Konten scheitern. Mandanten,
Organisationseinheiten und frei verwaltbare Work-Rollen besitzen vollständige
`PUT`-Änderungsverträge mit starkem `If-Match`, atomarer Versionserhöhung,
Audit und Outbox. Hierarchiezyklen und das Archivieren einer Einheit mit
aktiven Kindern, aktiven `governing_unit_id`-gebundenen Access-Gruppen oder
aktiven beziehungsweise geplanten Membership-, `GroupMembership`-,
`AppEntitlement`- oder Organisationsrollenkanten werden mit HTTP 409 abgelehnt.
Der Integritätscheck umfasst Work- und Service-Rollenzuweisungen, ohne
Service-Zuweisungen im Admin-Reader offenzulegen. Hierarchiemutationen werden
zuerst pro Tenant serialisiert und sperren anschließend Pfade und Kanten
geordnet.
Auch ein Umhängen, das
die Nachfahrenreichweite aktiver oder geplanter `include_descendants`-Kanten
auf einem geänderten Vorfahrenpfad verändern würde, antwortet mit HTTP 409;
abgelaufene, widerrufene und nur exakt gebundene Kanten erzeugen diesen
Vererbungskonflikt nicht. Das Deaktivieren nur der umgebenden App, Gruppe oder
Rolle hebt eine aktive durable Kante nicht auf. Systemrollen bleiben auch auf Datenbankebene
unveränderlich. Create, Umhängen und Reaktivieren teilen mit dem Resolver eine
Maximaltiefe von 64 Ebenen; tiefere Ergebnishierarchien werden mit
`organizational-unit-depth-limit-exceeded` abgelehnt. Künftige Writer für
App-, Gruppen- und Organisationsrollenkanten müssen die vorhandene
Tenant-vor-Pfad-vor-Kante-Sperrreihenfolge übernehmen. Suspendierte oder archivierte
Mandanten sperren tenantgebundene Work-Sessions bei der Actor-Auflösung. Eine
konsolidierte,
session-, audience- und berechtigungsgeschützte Audit-Timeline stellt
Security-Ereignisse
installationsweit mit Tenant-Kennzeichnung, begrenzter Cursor-Paginierung und
ohne freie Detail- oder Session-Rohdaten bereit. Jeder Abruf wird atomar selbst
auditiert. Konkrete externe Provider-Adapter werden erst nach diesem Kern
ergänzt.

**Abschlussnachweis 2026-07-25:** Phase 1 erfüllt die oben definierte Abnahme
und ist abgeschlossen. Ein gemeinsamer HTTP-Abnahmetest hält fest, dass ein
Work-Actor an `/admin/v1` und ein Admin-Actor am Workspace jeweils scheitert.
Die isolierten PostgreSQL-Integrationstests bestätigen zusätzlich getrennte
Laufzeitrollen, wiederholbare Migrationen, `FORCE ROW LEVEL SECURITY`,
Anwendungs- und Datenbankablehnung tenantfremder Zugriffe sowie atomare Audit-
und Outbox-Schreibvorgänge. Ein vollständiger Wegwerf-Stack wurde mit echtem
Work- und Admin-Login, Work-Passwortrotation, tenantgebundenem Workspace und
beiden negativen API-Grenzen geprüft. Der verschlüsselte Backup-/Restore-Test
und der native Ubuntu-24.04-First-Run waren ebenfalls erfolgreich. Die unten
genannten Provider-, Credential-Lebenszyklus- und App-Verwaltungs-Ausbaustufen
erweitern den Kern später, öffnen aber die Phase-1-Abnahme nicht erneut.
Passkeys sind nun zusätzlich als vollständiger WebAuthn-Lauf für Work- und
Administrationskonten vorhanden: re-authentifizierte Registrierung,
phishing-resistente Anmeldung, verschlüsselte Credential- und Zeremoniedaten,
RP-/Origin-Bindung, verpflichtende Benutzerverifikation, Signaturzähler,
Session-Rotation, Audit und passende Browser-Oberflächen. Der isolierte
Integrationstest verwendet eine echte EC2-Attestation und Assertion und lehnt
Fremd-Origin sowie Ceremony-Replay ab.

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
weiterhin keine Kontoart, Tenant-Zuordnung oder Audience liefern. WebAuthn ist
über den lokalen Credential-Adapter und den versionierten HTTP-Vertrag
funktionsfähig. Faktorwiderruf,
beaufsichtigte Wiederherstellung und Recovery-Code-Erneuerung benötigen noch
eigene aktionsgebundene, re-authentifizierte Verwaltungsabläufe. In Produktion
erzwingt die Konfigurationsvalidierung eine verfügbare MFA-Funktion, einen
32-Byte-Verschlüsselungsschlüssel und explizite erlaubte Origins; die
WebAuthn-Konfiguration bindet Origins zusätzlich an eine feste RP-ID. Diese
Konfiguration ist keine globale Einschreibungs- oder Zugangsbedingung.

**Admin-Assurance 2026-07-29:** Eine interaktive `admin`-Session mit
Admin-Audience, ohne Tenant und mit bekannter Assurance `single-factor` oder
`multi-factor` darf die Administrationsebene betreten. `unknown` wird
fail-closed abgewiesen. MFA ist eine selbst gestartete, nicht blockierende
Empfehlung. Kritische Tenant- und Work-Account-Mutationen verlangen inzwischen
eine ausdrückliche, aktions- und ressourcengebundene Re-Authentifizierung. Das
fünf Minuten gültige Einmalticket ist an aktuelle Admin-Session,
Sicherheitsgeneration, Permission und Zielressource gebunden. Grundvertrag und
Umsetzung sind in [`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md)
und [`ADR-036`](adr/ADR-036-aktionsgebundene-admin-reauthentifizierung.md)
festgelegt.

**Tenant- und Provider-Härtung 2026-07-29:** Die im Administrationsverzeichnis
sichtbare Tenant-Zuordnung eines Work-Kontos ist eine feste Sicherheitsgrenze.
Der neue, explizit tenantadressierte Änderungsvertrag erlaubt ausschließlich
den Lebenszyklus `active` ↔ `disabled`; er verschiebt weder Konto noch Person,
Login oder Membership in einen anderen Tenant. Eine wirkliche Statusänderung
benötigt ein starkes `If-Match`, erhöht Konto- und Sessiongeneration und schreibt
Audit plus Outbox atomar, sodass bestehende Sitzungen ungültig werden.

Passwort-, MFA-, Passkey- und API-Key-Abschlüsse prüfen unmittelbar vor
Session-Ausstellung, Assurance-Erhöhung oder Credential-Mutation das exakt
verwendete aktive Credential sowie den dazu gespeicherten aktiven Provider mit
passender Providerart und die konkrete aktive Konto-Provider-Bindung. Ändert
sich eine dieser drei Grundlagen, scheitert der Vorgang unter derselben
allgemeinen Authentifizierungsantwort fail-closed. Ausstehende MFA-Challenges
sind dafür an die `credential_id` des ersten Faktors gebunden; ein
Credential-Austausch kann die begonnene Zeremonie nicht übernehmen.

Eine separate, durch Admin-Session und Permission geschützte Übersicht der Anmeldeanbieter
zeigt höchstens 100 nicht geheime Metadatensätze und kennzeichnet weitere
Ergebnisse mit `truncated`.
`provider_key` ist auf 120 Zeichen, ein vorhandener getrimmter `issuer` auf
2048 Zeichen begrenzt. Die lokale Sicht enthält Anmeldemethoden, MFA-Status,
WebAuthn-RP-ID, RP-Name und nur die Anzahl konfigurierter Origins; Secretwerte
und konkrete Originwerte werden nicht ausgegeben. Der isolierte OIDC-v1-Adapter
prüft inzwischen Authorization Code, PKCE, State, Nonce, Discovery, Token und
exakte Providerbindung. Sein verschlüsselter, PostgreSQL-basierter
Einmal-Ceremony-Port ist vorhanden. Typisierte Providerkonfiguration,
Secret-Port, Kontenauflösung, HTTP-Laufzeitverdrahtung sowie SAML und LDAP sind
weiter offen und werden ausdrücklich als nicht verfügbar ausgewiesen; ein
Registry-Eintrag oder der Adapter allein aktiviert keinen externen Anmeldeweg.

**Organisations- und App-Zugriffsgrundlage 2026-07-29:** Hierarchische
Organisationseinheiten bleiben die inneren Unternehmensschalen eines Tenants.
Tenantgebundene Access-Gruppen bilden querliegende Kanten aus Work-Konten und
Organisationseinheiten. Plattformregistrierte Apps können tenantbezogen
aktiviert und über ein separates App-Entitlement ausdrücklich für Konto,
Einheit oder Gruppe geöffnet werden. Zusammengesetzte Tenant-Fremdschlüssel,
RLS und ein purer Go-Entscheidungsvertrag sind vorhanden. Die pure
`ResolveActorCoordinates`-Auflösung bindet den Snapshot an genau einen
tenantgebundenen `work`-Actor, berücksichtigt alle aktiven
`OrganizationalMembership`s, verfolgt vollständige aktive Elternpfade und löst
die wirksamen Access-Gruppen deterministisch auf. Jede organisatorische
Membership wiederholt dabei die geprüfte Account-Bindung; die resultierenden
Koordinaten sind außerhalb des Core-Pakets nicht frei konstruierbar und an Actor
sowie Prüfzeitpunkt gebunden. Der öffentliche Gate-Vertrag löst sie innerhalb
derselben Entscheidung auf und validiert den gesamten Entitlement-Snapshot vor
einem Match. Die spätere Runtime muss dennoch für jede Prüfung einen frischen
PostgreSQL-Snapshot und eine servereigene Zeit liefern; der pure Requestwert ist
keine cachebare Autoritätsaussage. Strukturell ungültige, unterbrochene referenzierte Pfade oder
tenantfremde Eingaben werden fail-closed abgelehnt. Die Organisationsmutation
schützt direkte und geplante Kanten beim Archivieren sowie die
Nachfahrenreichweite beim Umhängen mit den
oben beschriebenen 409-Konflikten. Ein Entitlement erteilt keine Rolle oder
Fachberechtigung. PostgreSQL-Store, Runtime-Adapter, Verwaltungs-API und UI für
die App-Verwaltung, delegierte Gruppenverwaltung und der erste
Fachapp-Verbraucher bleiben offen; Änderungen vorbehalten.

## Phase 2 – Gemeinsame Daten- und Ereignisplattform

**Zweck:** Zusammenhänge, Änderungen und historische Entscheidungen über alle
späteren Fachmodule hinweg verständlich machen.

**Async-Grundlage 2026-07-20:** Transactional-Outbox-Schema, versionierte
Event-Verträge, PostgreSQL-Leasing, partitionsgeordnete parallele Worker,
idempotente Consumer-Receipts, Retry/Backoff und Dead-Letter-Zustand sind als
globale Grundlage vorhanden. Fachliche Event-Registrierung, Replay-Verwaltung,
Metriken und der optionale Valkey-Adapter bleiben Teil des weiteren Phase-2-
Ausbaus.

**Cache-Grundlage 2026-08-01:** Ein herstellerneutraler `CachePort` und der
erste optionale Valkey-Adapter sind vorhanden. Identity nutzt ihn zunächst nur
für kurzlebige, gehashte Negativhinweise zu eindeutig ungültigen Sessions;
PostgreSQL bleibt für Session-, Rollen-, Sperr- und Widerrufsentscheidungen
maßgeblich. Positive Claims-Caches, Metriken und Lasttests bleiben offen.
Ein Session-Retention-/Anonymisierungsvertrag und ein berechtigungsgetrennter
Identity-Maintenance-Pfad bleiben ebenfalls offen; der allgemeine Worker erhält
dafür keinen direkten Zugriff auf Identity-Tabellen. Der noch nicht freigegebene
Vorschlag steht in [`ADR-040`](adr/ADR-040-identity-retention-und-session-anonymisierung.md).

**Ressourcenbasis 2026-07-21:** Das globale Modul- und Ressourcentypregister
sowie die Permission-Ressourcentyp-Bindung sind als erster Teil umgesetzt.
Ein verpflichtendes Datenprofil je Ressourcentyp klassifiziert zusätzlich
personenbezogene Daten und Vertraulichkeit; fehlt es, scheitert die
Autorisierungsauflösung fail-closed. Zusätzlich besitzt jede aktuelle
Permission-Ressourcentyp-Kombination eine serverseitige Processing-Policy. Die
gemeinsame Core-Entscheidung prüft Actor, Permission, Ressource, Datenprofil,
Processing-Policy und Grant. Das betreiberseitig freigegebene
Verarbeitungsverzeichnis sowie weitere Business-Objektregistrierungen,
Relationen, Policy-Facts und Suchprojektionen bleiben offen. Änderungen
vorbehalten.

**Business-Objektprojektion 2026-07-28:** Ein erster ausführbarer,
tenantgebundener `BusinessObjectView` ist vorhanden. Der versionierte Vertrag
erlaubt aktuell ausschließlich die ownergebundene, minimierte Workspace-
Projektion, persistiert sie mit `FORCE ROW LEVEL SECURITY` und löst sie über eine
versionierte Work-Query mit serverseitiger Berechtigungsprüfung auf. Fehlende,
tenantfremde und nicht sichtbare Objekte werden gleich behandelt. Core-Modell,
Store, Migration, HTTP-/OpenAPI-Vertrag sowie Unit-, Vertrags- und isolierte
PostgreSQL-Integrationstests belegen diesen ersten Schnitt. Weitere
Ressourcentypen, Relationen, Suche und ein allgemeiner Projektionsbetrieb bleiben
offen.

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

- Die vorhandene Ressourcenregistrierung und den ersten `BusinessObjectView`
  um weitere ownerseitige Projektionen für Navigation, Suche und autorisierte
  Kontextansichten erweitern.
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
