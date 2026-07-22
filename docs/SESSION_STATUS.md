# WERK – Übergabestatus

Stand: 2026-07-21

## Ziel

WERK ist ein selbst hostbares Unternehmensbetriebssystem. Core Identity ist die
interne Identitäts- und Zugriffsschicht. `work`, `admin` und `service` bleiben
strikt getrennt; `agent` bleibt ein tenantgebundener, nicht interaktiver
Principal auf der technischen `service`-Zugriffsebene.

## Aktueller Entwicklungsstand

- Dashboard läuft unter `http://127.0.0.1:3000`.
- API läuft unter `http://127.0.0.1:8081`.
- Native Infrastruktur: PostgreSQL `55432`, Valkey `56379`.
- `make dev` startet Migration, API, Worker und Dashboard dauerhaft.
- Login-Routen sind erreichbar:
  - `POST /api/v1/auth/login`
  - `GET /api/v1/auth/session`
  - `POST /api/v1/auth/logout`
  - `POST /api/v1/auth/password`
- Die Login-Oberfläche ist neutral und enthält keine User-/Admin-Auswahl.
- Der PostgreSQL-Auth-Service ist über die getrennte Non-Owner-Rolle
  `werk_identity_runtime` verdrahtet.
- Der Entwicklungs-Bootstrap legt `admin@werk.local` atomar und einmalig an.
- Im Entwicklungsprofil wird `dev-worker@werk.local` idempotent als getrenntes
  Work-Konto mit eigenem Tenant, Team und Basisrolle angelegt. Sein temporäres
  Passwort lautet `werk-worker-development` und wird beim Neustart nie
  zurückgesetzt.
- Passwörter werden mit Argon2id und zufälligem Salt gehasht; Sessiontokens
  liegen ausschließlich als SHA-256-Hash in PostgreSQL.
- Login, Sessionauflösung, Passwortwechsel und Logout wurden gegen den laufenden
  Container-Stack geprüft.
- TOTP-MFA für Admin-Konten ist als vollständiger Enrollment- und Loginablauf
  vorhanden. Recovery-Codes sind einmalig verwendbar und werden nur gehasht
  gespeichert; TOTP-Secrets liegen AES-256-GCM-verschlüsselt vor.
- Im Entwicklungsprofil ist Admin-MFA standardmäßig aktiv, damit die
  Verwaltungsoberflächen ohne Abschwächung der Access-Plane-Regeln nutzbar sind.
- Schreibende Cookie-Aufrufe sind durch Origin-/Fetch-Metadata-Prüfung,
  Double-Submit-CSRF und `SameSite=Strict` geschützt. Passwort- und MFA-Versuche
  besitzen persistente Versuchslimits.
- Login-Erfolg, Login-Ablehnung/Drosselung, MFA-Schritte, Passwortwechsel und
  Logout werden mit Request-/Correlation-ID protokolliert. Session-Ausstellung,
  Passwortänderung und Session-Widerruf sind jeweils mit ihrem Erfolgs-Audit
  atomar.
- Berechtigungsregistrierung, Rollen, Zuweisungen und serverseitige
  Scope-/Tenant-/Access-Plane-Policy sind vorhanden.
- Module, Ressourcentypen mit expliziter Installations-/Tenant-Grenze und die
  erlaubten Permission-Ressourcentyp-Zuordnungen sind installationsweit in
  PostgreSQL registriert. Der Go-Policy-Kern verwendet einen serverbestimmten
  Plattformkontext und typisierte Ressourcenreferenzen; fehlende oder
  deaktivierte Bindungen werden geschlossen abgelehnt.
- Jeder autorisierbare Ressourcentyp besitzt zusätzlich ein aktives Datenprofil
  für Personenbezug, Vertraulichkeit und die Pflicht eines
  Verarbeitungskontexts. Das Autorisierungs-Query lehnt fehlende oder inaktive
  Profile geschlossen ab. Jede Permission-Ressourcentyp-Bindung besitzt zudem
  eine serverseitige Processing-Policy. Der Core wertet Actor, Permission,
  Ressource, Datenprofil, Processing-Policy und Grants als eine fail-closed
  Entscheidung aus. Ein betreiberseitig freigegebenes Verzeichnis von
  Verarbeitungstätigkeiten ist bewusst noch nicht vorgetäuscht.
- Tenant-App-Installationen, Access-Gruppen und explizite App-Entitlements sind
  als tenantgesicherte PostgreSQL-Verträge und pure Go-Zugriffsentscheidung
  vorhanden. Eine Freischaltung kann ein Work-Konto, eine Organisationseinheit
  mit optionalen Nachfahren oder eine querliegende Access-Gruppe adressieren.
  Sie öffnet nur die App-Tür und erteilt weder Rolle noch Fachberechtigung.
- Ein MFA-bestätigter Installationsadministrator kann über
  `POST /admin/v1/work-users` ein Work-Konto mit Person, Membership,
  temporärem Passwort und tenantgebundener Workspace-Rolle anlegen.
- `GET/POST /admin/v1/tenants` und
  `GET/POST /admin/v1/tenants/{tenantId}/organizational-units` stellen die
  installationsweite Mandanten- und tenantgebundene Organisationsverwaltung
  bereit. Erzeugung, Audit und Outbox-Ereignis bilden jeweils einen Commit.
- Die Admin-Oberfläche bietet Formulare und eine Organisationsübersicht für
  diese Verträge.
- Tenantgebundene Work-Rollen können über `GET/POST /admin/v1/work-roles` aus
  dem registrierten Work-Berechtigungskatalog angelegt und über
  `PUT /admin/v1/work-users/{accountId}/roles` einem Arbeitskonto zugewiesen
  werden. Systemrollen bleiben unveränderlich; Access-Plane- und Tenant-Mischung
  scheitern in Service, Triggern und RLS.
- Die Admin-Oberfläche enthält dafür eine Rollenliste, den geschützten
  Berechtigungskatalog und eine Rollenzuweisung direkt an den Benutzerkonten.
- `GET /api/v1/workspace` verwendet `core.workspace.access` praktisch. Der
  Tenant stammt ausschließlich aus der Work-Session; der Arbeitskontext wird
  über `werk_work_runtime`, eine Read-only-Tenant-Transaktion und RLS geladen.
  Admin-Sessions, fremde Accounts und Sessions mit offenem Erst-Passwortwechsel
  werden abgewiesen.
- Die Benutzerseite zeigt den serverseitig bestätigten Mandanten, die aktive
  Organisationseinheit, Mitgliedschaft und Workspace-Berechtigung. Geplante
  Aufgaben-, Inbox- und Dokumentfunktionen sind klar als spätere Phasen
  gekennzeichnet und werden nicht mit erfundenen Daten dargestellt.
- Bestehende Mandanten, Organisationseinheiten und frei verwaltbare Work-Rollen
  können über versionierte `PUT`-Verträge geändert werden. Ein starker
  `If-Match`-Entity-Tag verhindert verlorene Updates; jede erfolgreiche Änderung
  erhöht die Version und schreibt Audit plus Outbox atomar.
- Suspendierte oder archivierte Mandanten sperren tenantgebundene Work-Sessions
  bei der nächsten Actor-Auflösung. Organisationseinheiten bleiben
  tenantgebunden und zyklenfrei; Einheiten mit aktiven Kindern oder Memberships
  können nicht archiviert werden. Systemrollen sind in Anwendung, RLS und
  Datenbanktriggern unveränderlich.
- `GET /admin/v1/security-audit` liefert eine installationsweite, optional nach
  Tenant, Ereignistyp und Ergebnis gefilterte Security-Timeline mit begrenzter
  Cursor-Paginierung. Zugriff erfordert MFA und
  `core.audit.security-event.read`; freie Detail-JSON- und Session-Rohdaten
  werden nicht ausgeliefert. Jeder erfolgreiche Abruf wird im selben Commit als
  `core.audit.security-events-listed.v1` protokolliert.
- Die Admin-Oberfläche enthält dafür eine eigene Ansicht „Audit-Protokoll“ mit
  Tenant-/Ergebnis-/Ereignisfilter, Statusübersicht, Nachladen und kopierbaren
  Request-/Korrelationskennungen.

## Identitätsmodell

- Interne Quelle: `Core Identity`.
- Externe OIDC/SAML/LDAP-Systeme sind nur optionale spätere Adapter.
- Provider-Nachweise enthalten ausschließlich Provider-Subject,
  Authentifizierungsmethode, Zeitpunkt und Assurance. Kontoart, Tenant und
  Audience werden anschließend aus einer internen Bindung aufgelöst.
- Kontoarten und Audiences sind registrierte Datenbankverträge. Konten können
  mehrere widerruf- und rotierbare Credentials besitzen.
- Tenantgebundene Agenten sind als nicht interaktive Principals vorbereitet.
  API-Schlüssel werden getrennt gehasht aufgelöst und können ein über alle
  Prozesse derselben autoritativen PostgreSQL-Datenbank atomar gezähltes
  Nutzungslimit besitzen; Browser-Sessions entstehen dabei nicht. Zwei
  getrennte Datenbankkopien sind von dieser Konsistenzzusage nicht umfasst.
- Dev-Admin ist vorgesehen als `admin@werk.local` mit temporärem Passwort `werk-development`.
- Das Dev-Passwort muss beim ersten Login geändert werden.
- Produktion darf kein festes Default-Passwort verwenden.

## Cliententscheidung

- Die responsive Weboberfläche bleibt ein eigenständiger Client.
- Installierbare Smartphone-, Tablet- und spätere Desktop-Clients verwenden
  Kotlin Multiplatform und Compose Multiplatform gemäß ADR-013.
- Der erste native Client gehört ausschließlich zur `work`-Zugriffsebene. Eine
  spätere native Administration benötigt ein getrenntes Artefakt, eine getrennte
  Audience und eine getrennte Sessionablage.
- PWA und WebView-Shell gelten nicht als nativer WERK-Client.
- Native Authentifizierung und Offline-Synchronisation benötigen vor ihrer
  Implementierung jeweils ein konkretisierendes Sicherheits-ADR.

## Bereits vorhanden

- Access-Plane-Vertrag und Session-Auflösung
- Party-/Person-/Organisation-/Membership-Modell
- Account-/Credential-/Session-Tabellen
- Bootstrap-Singleton und `must_change_password`
- RLS- und Tenant-Grenzen
- Backup-/Restore-Profile
- ADR-010 und Bootstrap-Dokumentation
- ADR-011 zur getrennten Core-Identity-Runtime
- ADR-013 und die dokumentierte Kotlin-/Compose-Multiplatform-Clientarchitektur
- ADR-014 zu Principals, Providern, mehreren Credentials und Audiences
- ADR-015 zur späteren Active/Passive-Identity-Autorität mit Witness, Lease,
  Autoritätsgeneration und Fencing
- Persistenter Credential- und Session-Adapter
- RBAC-/Policy-Fundament und Admin-Provisioning für Work-Konten
- Plattformweites Modul-/Ressourcentypregister und fail-closed `ResourceRef`-
  Autorisierungsvertrag gemäß ADR-016
- EU-Compliance- und Datenverarbeitungsgrundlage mit verpflichtendem
  Ressourcendatenprofil und Permission-Processing-Policy gemäß ADR-017
- Organisationskoordinaten, Access-Gruppen und explizite App-Entitlements gemäß
  ADR-018
- Transaktionale Outbox und partitionierte Worker-Grundlage

## Mehrinstanzziel – geplant, noch nicht implementiert

- Mehrere Core-/Identity-Prozesse an derselben PostgreSQL-Wahrheit teilen
  bereits Sperren, Widerrufe und atomare Sicherheitszähler.
- Eine zweite Instanz mit eigener Datenbankkopie bleibt eine Active/Passive-
  Replik desselben Identity-Realms und keine unabhängige Identity-Quelle.
- Automatischer Failover benötigt einen unabhängigen, QDevice-artigen Identity
  Witness. Ein Healthcheck ist nur ein Signal und darf keine Schreibhoheit
  vergeben.
- Die Reserve darf erst nach abgelaufener Lease, exklusiv vergebener höherer
  Autoritätsgeneration, erfüllter Replikationsschranke und Fencing der alten
  Hauptinstanz übernehmen.
- Ohne erreichbaren Witness bleibt ein Wechsel manuell und fail-closed. Der
  Witness speichert keine Konten, Credentials, Schlüssel, Tenants oder
  Fachdaten.
- Das Einzelinstanzprofil benötigt heute keine Witness-Infrastruktur. Umsetzung
  und Failover-Drills gehören in die spätere HA-/Produktreifephase.

## Nächster zwingender Schritt

Phase 1 wird als Nächstes mit den noch offenen Identity-Lebenszyklusabläufen
weiter gehärtet. Die Phase-2-Ressourcenregistrierung besitzt nun ein globales
Typregister und verpflichtende Datenprofile; als nächste Schicht folgen
Business-Objektprojektionen, Policy-Facts und gemeinsame Objektansichten. Das
serverseitige Permission-Processing-Register ist als strukturelle Policybasis
vorhanden. Das betreiberseitig freigegebene Verzeichnis von
Verarbeitungstätigkeiten wird mit Retention-, Audit- und Löschverträgen separat
konkretisiert.

Für die App-Zugriffsgrundlage folgen Verwaltungs-API, Administrationsoberfläche,
delegierte Zuständigkeiten und die Kopplung an den ersten realen
Fachapp-Endpunkt. Bis dahin ist der Vertrag vorhanden, aber nicht als fertige
Benutzerfunktion ausgewiesen.

WebAuthn, beaufsichtigte Kontowiederherstellung, Faktorwiderruf und das Erneuern
von Recovery-Codes bleiben weitere Identity-Härtungen. Sie dürfen die
Work-/Admin-Trennung nicht aufweichen.

## Prüfungen

Go-Tests, Vet, Formatierung, JavaScript-Syntaxprüfung, OpenAPI-Struktur,
Compose-Struktur, Migration-Wiederholung, Rollen-/RLS-/RBAC-Integration,
Mandanten- und Organisationseinheiten-Erzeugung und -Änderung einschließlich
Versionskonflikt, Hierarchiezyklus und Statusgrenzen, Work-Account-
Provisionierung, Work-Rollenerzeugung, -Änderung und -Zuweisung einschließlich
Systemrollen- und Cross-Tenant-Ablehnung sowie der echte Passwortwechsel-/TOTP-/
Recovery-End-to-End-Ablauf, Organisations-/App-Zugriffsconstraints und
Cross-Tenant-App-Kanten laufen erfolgreich. Der Workspace-Vertrag ist
zusätzlich gegen Work-RLS, fremde Account-/Tenant-Kombinationen, Admin-Akteure,
den offenen Erst-Passwortwechsel und suspendierte Mandanten geprüft.

Planungsstand 2026-07-21; weitere Policy-, App- und Ressourcenverträge bleiben
änderbar. Sicherheitsgrenzen, fail-closed Verhalten und Owner-Datenhoheit sind
davon nicht ausgenommen.
