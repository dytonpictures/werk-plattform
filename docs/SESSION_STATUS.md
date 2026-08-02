# WERK – Übergabestatus

Stand: 2026-08-01

## Ziel

WERK ist ein selbst hostbares Unternehmensbetriebssystem. Core Identity ist die
interne Identitäts- und Zugriffsschicht. `work`, `admin` und `service` bleiben
strikt getrennt; `agent` bleibt ein tenantgebundener, nicht interaktiver
Principal auf der technischen `service`-Zugriffsebene.

## Aktueller Entwicklungsstand

- API und eingebettetes Dashboard laufen gemeinsam unter
  `http://127.0.0.1:3000`.
- `.env` ist die zentrale lokale Konfigurations- und Secret-Datei; getrennte
  Datenbank-URLs erhalten die Rollen- und Kontoartgrenzen.
- `sh scripts/start.sh` validiert auf Debian/Ubuntu `amd64` die Konfiguration,
  migriert und startet die API. Der Worker bleibt bei
  deaktiviertem Kafka optional.
- Login-Routen sind erreichbar:
  - `POST /api/v1/auth/login`
  - `GET /api/v1/auth/session`
  - `POST /api/v1/auth/logout`
  - `POST /api/v1/auth/password`
  - `POST /api/v1/auth/invitations/initial/accept`
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
- Login, Sessionauflösung, Passwortwechsel und Logout wurden gegen den nativen
  Laufzeitpfad geprüft.
- TOTP-MFA für Admin-Konten ist als vollständiger Enrollment- und Loginablauf
  vorhanden. Recovery-Codes sind einmalig verwendbar und werden nur gehasht
  gespeichert; TOTP-Secrets liegen AES-256-GCM-verschlüsselt vor.
- Passkey-Anmeldungen starten loginlos mit discoverable Credentials. Eine
  getrennte anonyme Authentication-Ceremony enthält vor dem signierten
  WebAuthn-Nachweis weder Account, Tenant noch Audience, wird atomar konsumiert
  und besitzt Peer-, Prozess-, Bestands- und Cleanup-Grenzen. Erst Credential-ID
  und opaker User-Handle lösen das aktive Core-Identity-Konto serverseitig auf.
- Die Profiloberfläche listet ausschließlich eigene aktive Passkeys und
  Sessions. Eigene Passkeys können nach Passwortbestätigung mit atomarer
  Sessionrotation widerrufen werden; einzelne eigene Sessions lassen sich
  gezielt beenden. Credential- und Sessiongeheimnisse werden nicht ausgegeben.
- Eine interaktive Admin-Session mit Admin-Audience, ohne Tenant und mit
  bekannter Assurance `single-factor` oder `multi-factor` darf die normal
  autorisierten Admin-Funktionen nutzen. `unknown` wird fail-closed abgewiesen.
- Ein Admin ohne aktiven Faktor erhält eine nicht blockierende Empfehlung zur
  selbst gestarteten MFA-Einrichtung. Die Session-Antwort verwendet dafür
  `mfa_enrollment_recommended`; das veraltete
  `mfa_enrollment_required` ist keine Zugangsbedingung.
- Im Entwicklungsprofil ist die MFA-Funktion standardmäßig verfügbar. Das ist
  weder eine globale Einschreibungs- noch eine globale Admin-Zugangspflicht.
  Nach aktivierter TOTP-Einrichtung bleibt der zweite Faktor bei späteren
  Passwortanmeldungen verpflichtend. Die Admin-Assurance-Grenze steht in
  [`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md).
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
  Sie öffnet nur die App-Tür und erteilt weder Rolle noch Fachberechtigung. Der
  pure serverseitige `ResolveActorCoordinates`-Vertrag bindet die Auflösung an
  einen tenantgebundenen `work`-Actor, berücksichtigt alle aktiven
  `OrganizationalMembership`s, vollständige aktive Elternpfade und wirksame
  Access-Gruppen und lehnt organisatorische Memberships ohne identische
  Account-Bindung sowie strukturell ungültige oder tenantfremde Snapshots
  fail-closed ab. Die resultierenden Koordinaten sind außerhalb des Core-Pakets
  nicht frei konstruierbar, an Actor und Prüfzeitpunkt gebunden und werden vom
  öffentlichen `EvaluationRequest` innerhalb derselben Entscheidung aufgelöst.
  Der vollständige Entitlement-Snapshot wird vor einem Match validiert. Der
  pure Wert kann seine eigene Datenfrische nicht beweisen; PostgreSQL-Store und
  Runtime müssen später pro Prüfung einen neuen Snapshot mit servereigener Zeit
  liefern. PostgreSQL-Store,
  Runtime-Adapter, Verwaltungs-API, UI und ein echter Fachapp-Verbraucher für
  diese App-Verwaltung bleiben offen.
- Ein gültig angemeldeter und berechtigter Installationsadministrator kann über
  `POST /admin/v1/work-users` ein Work-Konto mit Person, Membership,
  tenantgebundener Workspace-Rolle und ausdrücklich gewählter Bereitstellung
  anlegen. Beim Startpasswort ist der Erstwechsel ein explizites Boolean. Der
  alternative Einladungsweg erzeugt ein deaktiviertes Konto ohne Credential;
  der Empfänger setzt sein Passwort selbst über einen ablaufenden Einmal-Link.
- Einladungstokens werden als 32 zufällige Bytes erzeugt und nur als
  SHA-256-Digest gespeichert. Aktivierung, erstes Credential, Tokenverbrauch,
  Sessiongeneration, Security-Audit und Outbox sind atomar. Die
  Erstellungsantwort kann einmalig einen lokalen E-Mail-Entwurf vorbereiten,
  behauptet aber ohne Notification-Provider keinen Versand und persistiert
  keine Empfängeradresse.
- Ein gültig angemeldeter und berechtigter Admin kann einen noch offenen oder
  bereits abgelaufenen Aktivierungslink in einem aktiven Mandanten neu
  ausgeben. Der alte Link wird
  im selben Commit widerrufen; neuer Digest, Audit und Outbox werden atomar
  gespeichert. Roh-Token und Empfängeradresse bleiben auf die nicht cachebare
  Einmalantwort begrenzt.
- Der öffentliche Aktivierungsweg lehnt unbekannte, abgelaufene, verbrauchte,
  widerrufene und ersetzte Tokens generisch ab. Pro direktem Peer gelten zwölf
  Versuche pro Minute; pro Prozess laufen höchstens zwei zugehörige
  Argon2id-Hashvorgänge gleichzeitig.
- `GET/POST /admin/v1/tenants` und
  `GET/POST /admin/v1/tenants/{tenantId}/organizational-units` stellen die
  installationsweite Mandanten- und tenantgebundene Organisationsverwaltung
  bereit. Erzeugung, Audit und Outbox-Ereignis bilden jeweils einen Commit.
- Die Admin-Oberfläche stellt einen bestätigten Tenant sichtbar als
  **Unternehmen** dar. Besteht das geladene Verzeichnis aus genau diesem einen
  aktiven Eintrag, wählt sie ihn automatisch und blendet den
  Unternehmenswähler aus. Eine Unternehmensanlage erscheint ausschließlich
  nach einem erfolgreich bestätigten leeren Verzeichnis; bei mehreren oder
  unbekannten Einträgen greift ein defensiver Auswahl- beziehungsweise
  Fehlerzustand. Bereiche, Abteilungen, Standorte und Teams erscheinen darunter
  als Organisationseinheiten. Die technischen Verträge in Core, Datenbank,
  API, RLS und Audit verwenden unverändert `tenant` und `tenant_id`; eine harte
  serverseitige Single-Company-Mengeninvariante ist noch offen.
- Tenantgebundene Work-Rollen können über `GET/POST /admin/v1/work-roles` aus
  dem registrierten Work-Berechtigungskatalog angelegt und über
  `PUT /admin/v1/work-users/{accountId}/roles` einem Arbeitskonto zugewiesen
  werden. Dieser Verwaltungsvertrag liest und ersetzt ausschließlich
  Zuweisungen mit `scope_type='tenant'`; Organisations- und Ressourcen-Scopes
  werden weder vorselektiert, widerrufen noch stillschweigend tenantweit
  gemacht. Systemrollen bleiben unveränderlich; Access-Plane- und
  Tenant-Mischung scheitern in Service, Triggern und RLS.
- Die Admin-Oberfläche enthält dafür eine Rollenliste, den geschützten
  Berechtigungskatalog und eine Rollenzuweisung direkt an den Benutzerkonten.
- `GET /api/v1/workspace` verwendet `core.workspace.access` praktisch. Der
  Tenant stammt ausschließlich aus der Work-Session; der Arbeitskontext wird
  über `werk_work_runtime`, eine Read-only-Tenant-Transaktion und RLS geladen.
  Admin-Sessions, fremde Accounts und Sessions mit offenem Erst-Passwortwechsel
  werden abgewiesen.
- Die Benutzerseite zeigt das serverseitig bestätigte Unternehmen, die aktive
  Organisationseinheit, Mitgliedschaft und Workspace-Berechtigung. Geplante
  Aufgaben-, Inbox- und Dokumentfunktionen sind klar als spätere Phasen
  gekennzeichnet und werden nicht mit erfundenen Daten dargestellt.
- Bestehende Mandanten, Organisationseinheiten und frei verwaltbare Work-Rollen
  können über versionierte `PUT`-Verträge geändert werden. Ein starker
  `If-Match`-Entity-Tag verhindert verlorene Updates; jede erfolgreiche Änderung
  erhöht die Version und schreibt Audit plus Outbox atomar.
- Suspendierte oder archivierte Mandanten sperren tenantgebundene Work-Sessions
  bei der nächsten Actor-Auflösung. Organisationseinheiten bleiben
  tenantgebunden und zyklenfrei. Eine Einheit kann nicht archiviert werden,
  solange aktive Kinder, aktive `governing_unit_id`-gebundene Access-Gruppen
  oder aktive beziehungsweise geplante Membership-, `GroupMembership`-,
  `AppEntitlement`- oder organisationsgebundene Rollenkanten bestehen. Beim
  Umhängen wird zusätzlich die geänderte Vorfahrenmenge geprüft: Würde sich dadurch die
  Nachfahrenreichweite einer aktiven oder geplanten
  `include_descendants`-Gruppenmitgliedschaft oder App-Freischaltung ändern,
  antwortet der Vertrag mit HTTP 409. Beim Archivieren werden abgelaufene
  Kanten, widerrufene `GroupMembership`s und `AppEntitlement`s sowie
  deaktivierte `governing_unit_id`-gebundene Access-Gruppen ignoriert. Beim
  Umhängen erzeugen abgelaufene, widerrufene oder nur exakt gebundene Kanten
  und unveränderte gemeinsame Vorfahren keinen Vererbungskonflikt. Das
  Deaktivieren einer umgebenden App, Gruppe oder Rolle entfernt eine weiterhin
  aktive Kante dagegen nicht. Hierarchiemutationen werden pro Tenant
  serialisiert; danach werden relevante Hierarchie- und Kantenzeilen geordnet
  gesperrt. Ein schmaler, RLS-geschützter Integritätsvertrag berücksichtigt auch
  organisationsgebundene Service-Rollenzuweisungen, ohne sie dem Admin-Reader
  offenzulegen. Künftige Writer für
  App-, Gruppen- und Organisationsrollenkanten müssen dieselbe
  Tenant-vor-Pfad-vor-Kante-Sperrreihenfolge einhalten. Create, Umhängen und Reaktivieren
  lehnen Ergebnishierarchien über der gemeinsamen Grenze von 64 Ebenen mit
  `organizational-unit-depth-limit-exceeded` ab. Systemrollen sind in
  Anwendung, RLS und Datenbanktriggern unveränderlich.
- `GET /admin/v1/security-audit` liefert eine installationsweite, optional nach
  Tenant, Ereignistyp und Ergebnis gefilterte Security-Timeline mit begrenzter
  Cursor-Paginierung. Zugriff erfordert eine gültige Admin-Session und
  `core.audit.security-event.read`; freie Detail-JSON- und Session-Rohdaten
  werden nicht ausgeliefert. Jeder erfolgreiche Abruf wird im selben Commit als
  `core.audit.security-events-listed.v1` protokolliert.
- Die Admin-Oberfläche enthält dafür eine eigene Ansicht „Audit-Protokoll“ mit
  Unternehmens-, Ergebnis- und Ereignisfilter, Statusübersicht, Nachladen und
  kopierbaren Request-/Korrelationskennungen. Der zugrunde liegende technische
  Filter bleibt `tenant_id`.

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
  ADR-018 einschließlich purer `ResolveActorCoordinates`-Auflösung
- Transaktionale Outbox und partitionierte Worker-Grundlage
- Das lokale Betriebswerkzeug `werkctl` mit `version`, read-only `doctor` und
  `status` sowie dem bestehenden mutierenden `migrate`. Start, Update und
  Hybrid-Cloud-Pairing sind noch nicht ausführbar und bleiben hinter der
  vorgeschlagenen typisierten Runnergrenze aus ADR-031.

## Mehrinstanzziel – geplant, noch nicht implementiert

- Mehrere Core-/Identity-Prozesse an derselben PostgreSQL-Wahrheit teilen
  bereits Sperren, Widerrufe und atomare Sicherheitszähler.
- Eine zweite Instanz mit eigener Datenbankkopie bleibt eine Active/Passive-
  Replik desselben Identity-Realms und keine unabhängige Identity-Quelle.
- Automatischer Failover benötigt die Domain `identity-control` eines
  unabhängigen, QDevice-artigen Platform Witness. Ein Healthcheck ist nur ein
  Signal und darf keine Schreibhoheit vergeben.
- Die Reserve darf erst nach abgelaufener Lease, exklusiv vergebener höherer
  Autoritätsgeneration, erfüllter Replikationsschranke und Fencing der alten
  Hauptinstanz übernehmen.
- Ohne erreichbaren Witness bleibt ein Wechsel manuell und fail-closed. Der
  Witness speichert keine Konten, Credentials, Schlüssel, Tenants oder
  Fachdaten.
- Das Einzelinstanzprofil benötigt heute keine Witness-Infrastruktur. ADR-022
  trennt `single`, `dual-cloud` und `hybrid` von der Authority-Koordination;
  Umsetzung und Failover-Drills gehören in die spätere HA-/Produktreifephase.

## Nächster zwingender Schritt

Phase 1 wird als Nächstes mit den noch offenen Identity-Lebenszyklusabläufen
weiter gehärtet. Die Phase-2-Ressourcenregistrierung besitzt nun ein globales
Typregister und verpflichtende Datenprofile; als nächste Schicht folgen
Business-Objektprojektionen, Policy-Facts und gemeinsame Objektansichten. Das
serverseitige Permission-Processing-Register ist als strukturelle Policybasis
vorhanden. Das betreiberseitig freigegebene Verzeichnis von
Verarbeitungstätigkeiten wird mit Retention-, Audit- und Löschverträgen separat
konkretisiert.

Für die App-Zugriffsgrundlage folgen PostgreSQL-Store und Runtime-Adapter für
die serverseitige Koordinatenauflösung, Verwaltungs-API,
Administrationsoberfläche, delegierte Zuständigkeiten und die Kopplung an den
ersten realen Fachapp-Endpunkt. Bis dahin sind pure Verträge vorhanden, aber
nicht als fertige Benutzerfunktion ausgewiesen.

Beaufsichtigte Kontowiederherstellung, Faktorwiderruf und das Erneuern von
Recovery-Codes bleiben weitere Identity-Härtungen. Sie dürfen die
Work-/Admin-Trennung nicht aufweichen. WebAuthn ist mit Registrierung, Anmeldung,
verschlüsselter Persistenz, Audit und Browser-Oberflächen umgesetzt.

## Prüfungen

Go-Tests, Race Detector, Vet, Formatierung, JavaScript-Syntaxprüfung,
OpenAPI-Struktur, native Start- und Paketstruktur, Migration-Wiederholung,
Rollen-/RLS-/RBAC-Integration,
Mandanten- und Organisationseinheiten-Erzeugung und -Änderung einschließlich
Versionskonflikt, Hierarchiezyklus und Statusgrenzen, Work-Account-
Provisionierung, Work-Rollenerzeugung, -Änderung und -Zuweisung einschließlich
Systemrollen- und Cross-Tenant-Ablehnung sowie der echte Passwortwechsel-/TOTP-/
Recovery-/Passkey-End-to-End-Ablauf mit EC2-Attestation, Fremd-Origin- und
Replay-Ablehnung, Organisations-/App-Zugriffsconstraints und
Cross-Tenant-App-Kanten laufen erfolgreich. Die Organisationsmutation ist
zusätzlich gegen aktive und geplante direkte Kanten sowie gegen Änderungen der
vererbten Nachfahrenreichweite beim Umhängen geprüft; beide Konfliktarten
werden als HTTP 409 abgebildet. Der Workspace-Vertrag ist
zusätzlich gegen Work-RLS, fremde Account-/Tenant-Kombinationen, Admin-Akteure,
den offenen Erst-Passwortwechsel und suspendierte Mandanten geprüft.

Der initiale Einladungsweg ist zusätzlich gegen direkte Admin-/Identity-
Umgehung, parallele Einlösung, Ablauf während einer Sperrwartezeit, Neuausgabe,
alten Token, suspendierte Mandanten sowie Token-/Empfänger-Leaks in Audit und
Outbox auf einer frischen PostgreSQL-Datenbank mit allen 43 Migrationen geprüft.
Die `werkctl`-Diagnose-, Status-, Redaktions- und Exitcode-Verträge laufen auch
unter dem Race Detector erfolgreich.

Planungsstand 2026-07-29; weitere Policy-, App- und Ressourcenverträge bleiben
änderbar. Sicherheitsgrenzen, fail-closed Verhalten und Owner-Datenhoheit sind
davon nicht ausgenommen.
