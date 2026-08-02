# WERK – Autorisierung und Kontoprovisionierung

Stand: 2026-07-29

## Sicherheitsgrenze

WERK entscheidet Zugriffe serverseitig aus sieben voneinander unabhängigen
Bestandteilen:

1. Die Session gehört exakt zu einer Access Plane (`work`, `admin` oder
   `service`).
2. Eine aktive Rollenzuweisung gehört derselben Access Plane an.
3. Rolle und registrierte Berechtigung gehören derselben Access Plane an.
4. Die Berechtigung ist ausdrücklich für den aktiven, registrierten
   Ressourcentyp zugelassen.
5. Der Ressourcentyp besitzt ein aktives, versioniertes Datenprofil.
6. Die Permission-Ressourcentyp-Bindung besitzt eine aktive Processing-Policy
   und liefert den erforderlichen serverseitigen Verarbeitungskontext.
7. Der Scope der Zuweisung deckt die angeforderte Ressource und ihren Tenant ab.

Ein `admin`-Konto ist deshalb kein stärkeres `work`-Konto. Es kann Identitäten
und Plattformkonfiguration verwalten, erhält aber dadurch keine fachliche
Entscheidungsbefugnis im Workspace.

Der zugrunde liegende Plattformvertrag ist in
[`ADR-016`](adr/ADR-016-plattformweiter-ressourcen-und-autorisierungsvertrag.md)
festgelegt.

## Admin-Session-Assurance

Die Administrationsebene akzeptiert eine interaktive Session ausschließlich
für die Kontoart `admin`, mit Admin-Audience und ohne Tenant. Eine bekannte
Assurance `single-factor` oder `multi-factor` ist zulässig; `unknown` wird
fail-closed abgewiesen. MFA ist ein selbst gestarteter, nicht blockierender
Verstärkungsweg und keine globale Vorbedingung für Admin-Berechtigungen.

Diese Eintrittsgrenze ersetzt keine Autorisierung. Permission,
Ressourcenreferenz, Scope, Datenprofil, Processing-Policy und bei
tenantgebundenen Operationen der explizite Tenant werden weiterhin
serverseitig geprüft. CSRF, Audit und atomare Outbox bleiben ebenfalls
verbindlich. Eine künftige besonders sensible Aktion darf nur dann zusätzliche
Assurance verlangen, wenn ihr versionierter Vertrag eine aktions- und
ressourcengebundene Re-Authentifizierung, Just-in-time-Freigabe oder
Mehrpersonenfreigabe ausdrücklich definiert. Diese Grenze steht in
[`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md).

## Plattformkontext und Ressourcenreferenz

Der Server leitet aus dem authentifizierten Actor einen `PlatformContext` mit
Actor-ID, Kontoart, Access Plane und gegebenenfalls Tenant ab. Werte aus Query,
Header oder Body dürfen diesen Kontext nicht ersetzen.

Jedes Autorisierungsziel besitzt zusätzlich eine `ResourceRef` aus Grenze,
registriertem `kind`, stabiler ID und bei tenantgebundenen Ressourcen zwingend
dem Tenant. Ein fehlender Tenant ist kein Platzhalter für alle Tenants, sondern
nur bei ausdrücklich installationsgebundenen Verwaltungsressourcen gültig.

PostgreSQL registriert Module, Ressourcentypen und die erlaubte Zuordnung einer
Berechtigung zu Ressourcentypen. Eine fehlende, deaktivierte oder fremde
Registrierung führt unabhängig von einer vorhandenen Rolle zu `deny`.

Zusätzlich benötigt jeder Ressourcentyp ein aktives, versioniertes Datenprofil
und jede Permission-Ressourcentyp-Bindung eine aktive Processing-Policy. Fehlt
einer dieser Verträge, scheitert dieselbe Auflösung fail-closed. Ein Datenprofil
klassifiziert personenbezogene Daten und Vertraulichkeit; es erteilt keine
Rechtsgrundlage. Die Processing-Policy liefert einen ausschließlich
serverseitig aufgelösten `ProcessingContext` und kann Verarbeitung auch für
Aktionen verlangen, deren Kontrollressource selbst keine personenbezogenen
Daten trägt. Das spätere Betreiberregister muss die referenzierte
Verarbeitungstätigkeit tatsächlich freigeben. Die Grenze beschreibt
[`ADR-017`](adr/ADR-017-eu-compliance-und-datenverarbeitungsgrundlage.md).

## Scopes

- `installation`: technische Administration ausschließlich für registrierte
  installationsgebundene Kontrollressourcen.
- `tenant`: gilt ausschließlich für den exakt zugewiesenen Tenant.
- `organizational-unit`: gilt für die konkrete Organisationseinheit im Tenant.
- `resource`: gilt für eine konkrete registrierte Ressource im Tenant.

Tenantgebundene Scopes werden sowohl im Go-Policy-Kern als auch durch
PostgreSQL-Invarianten und RLS abgesichert. Fehlender Tenant-Kontext ist keine
globale Freigabe, sondern eine Ablehnung.

Administratives Verwalten eines Mandanten verwendet eine installationsgebundene
Kontrollrepräsentation des Mandanten. Der anschließende Datenzugriff bleibt
trotzdem tenantexplizit und RLS-begrenzt. Damit wird aus einer Admin-Rolle kein
fachlicher Generalschlüssel.

## Work-Konto anlegen

`POST /admin/v1/work-users` erfordert eine gültige Admin-Session und die
Installationsberechtigung `core.identity.work-account.create`. Der Vorgang legt
in einer Admin-Datenbanktransaktion Person, Membership, Work-Konto,
tenantgebundene Workspace-Rolle und Zuweisung an. Die Bereitstellungsart ist
ausdrücklich Teil des Vertrags:

- `initial-password` erzeugt das Password-Credential sofort. Das verpflichtend
  vorhandene Boolean `require_password_change` bestimmt, ob nur der erste Login
  bis zum Passwortwechsel zugelassen wird. Ohne diese Pflicht kennt der Admin
  das weiterverwendete Startpasswort; die Oberfläche kennzeichnet dieses Risiko.
- `invitation-link` erzeugt zunächst ein deaktiviertes Work-Konto ohne
  Credential. Nur die einmalige, ablaufende Aktivierung kann es freischalten.
  Die allgemeine Statusmutation darf einen offenen Einladungszustand nicht
  umgehen.

Beide Wege enthalten im Erstellungscommit das Audit-Ereignis
`identity.work-account.created.v1` und den Outbox-Eintrag
`core.identity.work-account-created.v1`. Die Einladungseinlösung speichert
Credential, Tokenverbrauch, Kontoaktivierung, Sessiongeneration,
`identity.work-account-invitation.accepted.v1` und
`core.identity.work-account-invitation-accepted.v1` wiederum atomar. Roh-Token,
Empfängeradresse und Link werden weder in PostgreSQL, Audit noch Outbox
persistiert. Ein Teilzustand ohne Audit oder Rollenbindung kann damit nicht
sichtbar werden.

Für ein noch nicht aktiviertes Konto ersetzt
`POST /admin/v1/tenants/{tenantId}/work-users/{accountId}/invitation` eine
offene oder abgelaufene initiale Einladung. Der durch eine gültige
Admin-Session und die bestehende Permission geschützte Vertrag nutzt keine
zusätzliche Sonderberechtigung, sondern
`core.identity.work-account.update` auf dem adressierten Work-Konto. Innerhalb
derselben Tenant-Transaktion widerruft Core Identity den alten Link, speichert
nur den Digest des neuen Tokens und erzeugt
`identity.work-account-invitation.reissued.v1` sowie
`core.identity.work-account-invitation-reissued.v1`. Token und
Empfängeradresse fehlen in beiden Aufzeichnungen; die Adresse dient nur dem
einmaligen lokalen Browserentwurf.

## Mandanten und Organisationseinheiten verwalten

Mandantenlisten sind installationsweite, explizit autorisierte Read-only-
Abfragen. Das Anlegen eines Mandanten verwendet bereits dessen neue ID als
Transaktionskontext; Tenant, Security-Audit und
`core.tenancy.tenant-created.v1` werden gemeinsam sichtbar.

Organisationseinheiten werden ausschließlich innerhalb eines expliziten
Tenant-Kontexts gelesen und geschrieben. Eine Elternreferenz muss auf eine
aktive Einheit desselben Mandanten zeigen. PostgreSQL RLS verhindert zusätzlich,
dass die Admin-Runtime über den Transaktions-Tenant hinaus schreibt. Die
Admin-Runtime besitzt keine Löschberechtigung für Mandanten oder
Organisationseinheiten.

`PUT /admin/v1/tenants/{tenantId}` und
`PUT /admin/v1/tenants/{tenantId}/organizational-units/{unitId}` ersetzen die
änderbaren Felder nur bei passendem starkem `If-Match`-Entity-Tag. Erfolgreiche
Änderungen erhöhen `version` und schreiben Audit sowie Outbox im selben Commit.
Eine suspendierte oder archivierte Tenant-Grenze macht tenantgebundene Work-
Sessions bei der nächsten Actor-Auflösung unwirksam. Beim Umhängen von
Organisationseinheiten werden fremde oder inaktive Eltern und Hierarchiezyklen
abgelehnt. Eine Einheit mit aktiven Untereinheiten oder aktiven beziehungsweise
geplanten Membership-, Access-Gruppen-, Gruppenmitgliedschafts-,
App-Entitlement- oder organisationsbezogenen Rollen-Kanten kann nicht
archiviert werden. Der stabile Fehlercode lautet
`organizational-unit-referenced`. Beim Umhängen blockiert
`organizational-unit-inherited-access-conflict` Änderungen, die den
Geltungsbereich einer aktiven oder geplanten `include_descendants`-Kante aus
App-Entitlement oder Access-Gruppe verändern. Exakte sowie abgelaufene oder
widerrufene Kanten verhindern das Umhängen nicht. Das bloße Deaktivieren einer
umgebenden App-Installation, Access-Gruppe oder Rolle entfernt eine weiterhin
aktive durable Kante nicht; sie muss selbst beendet oder widerrufen werden.
Create, Umhängen und Reaktivieren prüfen außerdem den vollständigen daraus
entstehenden Teilbaum gegen die gemeinsame Grenze von 64 Ebenen. Der stabile
409-Code bei Überschreitung lautet
`organizational-unit-depth-limit-exceeded`.

## Work-Rollen verwalten und zuweisen

`GET/POST /admin/v1/work-roles` liest beziehungsweise erstellt Rollen immer in
einem expliziten Tenant-Kontext. Frei verwaltbare Rollen gehören ausschließlich
zur Access Plane `work` und dürfen nur aktive, registrierte Work-Berechtigungen
enthalten. Admin- und Service-Berechtigungen werden serverseitig ausgeschlossen;
Systemrollen sind als solche gekennzeichnet und über diesen Vertrag nicht
veränderbar.

`PUT /admin/v1/work-roles/{roleId}` ersetzt Anzeigename, Status und
Berechtigungsmenge einer frei verwaltbaren Rolle mit `If-Match`-Versionsschutz.
Der technische Rollen-Key bleibt stabil. Änderungen schreiben Audit und Outbox
atomar. Systemrollen sind sowohl in der Application-Schicht als auch durch
PostgreSQL-Trigger und eingeschränkte RLS-Policies gegen Änderungen der
Admin-Runtime geschützt.

`PUT /admin/v1/work-users/{accountId}/roles` ersetzt ausschließlich die aktuell
wirksamen Work-Rollen mit `scope_type='tenant'` eines Arbeitskontos. Die dazu
verwendete Benutzerprojektion enthält ebenfalls nur diesen Scope. Bestehende
Organisations- oder Ressourcen-Zuweisungen werden weder widerrufen noch durch
Vorbelegung tenantweit gemacht. Konto, Rollen und Scope müssen demselben
Mandanten angehören. Datenbanktrigger, RLS und eine Eindeutigkeitsregel für
aktive Zuweisungen sichern diese Grenze zusätzlich. Rollenerzeugung und
Zuweisungswechsel schreiben jeweils Security-Audit und Outbox-Ereignis atomar.
Die Verwaltung einer Work-Rolle erlaubt dem Admin weder eine Work-Session noch
fachliche Entscheidungen im Namen des Kontos.

## Organisationskoordinaten und App-Freischaltung

Organisationseinheiten bilden innerhalb eines Tenants Bereiche, Abteilungen
und Teams. Access-Gruppen ergänzen diese Hierarchie um tenantgebundene,
abteilungsübergreifende Kanten. Sie enthalten im ersten Vertrag ausschließlich
Work-Konten und Organisationseinheiten; verschachtelte Gruppen bleiben
ausgeschlossen.

Eine Plattformregistrierung `app.*` muss zunächst im Tenant aktiv installiert
sein. Danach öffnet ein zeitlich gültiges `AppEntitlement` die App ausdrücklich
für ein Work-Konto, eine Organisationseinheit – optional einschließlich ihrer
Nachfahren – oder eine Access-Gruppe. Fehlt Installation oder Entitlement, gilt
`deny`.

Das Entitlement ist keine Rolle und enthält keine Fachberechtigung. Nach dem
App-Gate folgen weiterhin Rolle, Permission, Ressourcenreferenz,
Processing-Policy und die Fachregel der Owner-App. Die Organisation- und
Gruppenkoordinaten werden serverseitig ermittelt; Query, Header und Body dürfen
sie nicht erweitern. Der Basisvertrag steht in
[`ADR-018`](adr/ADR-018-organisationskoordinaten-und-app-entitlements.md).

Tabellen, Tenant-Fremdschlüssel, RLS und der pure Go-Entscheidungsvertrag sind
implementiert. Verwaltungs-API, UI, Delegation und ein produktiver
Fachapp-Endpunkt folgen erst mit einem konkreten Verbraucher; Änderungen
vorbehalten.

## Workspace-Zugriff

`GET /api/v1/workspace` ist der erste praktische Verbraucher einer
tenantgebundenen Work-Berechtigung. Der Endpunkt akzeptiert ausschließlich eine
gültige Session der Access Plane `work`, übernimmt den Tenant unveränderlich aus
dieser Session und prüft `core.workspace.access` gegen genau diesen
Tenant-Scope. Eine Tenant-ID aus Query, Header oder Request-Body wird nicht
akzeptiert.

Erst nach der Policy-Entscheidung wird der Arbeitskontext über die separate
Non-Owner-Rolle `werk_work_runtime` innerhalb einer Read-only-
Tenant-Transaktion geladen. RLS begrenzt Tenant, Konto, Party, Membership und
Organisationseinheit zusätzlich. Admin-Sessions, fremde Work-Konten und
Sessions mit noch offenem Erst-Passwortwechsel werden abgelehnt.

Die versionierte Antwort enthält außerdem `capabilities.documents`. Dieser
Wert wird serverseitig über `core.documents.document.list` gegen die virtuelle,
tenantgebundene Dokumentsammlung geprüft. Clients dürfen damit den Einstieg in
Core Documents anzeigen oder ausblenden; die Berechtigungsprüfung jedes
Dokumentendpunkts bleibt davon unabhängig verbindlich.

## Dokument- und Storage-Zugriff

Dokumentzugriffe verwenden die Plattform-Policy als äußeres Gate, nicht als
universelle Dokumentregel. Core Identity liefert Actor, Kontoart, Access Plane,
Tenant und Assurance. Die zentrale Entscheidung prüft Ressourcentyp,
Permission, Scope, Datenprofil und Processing-Policy. Danach darf Core Documents
zusätzliche lokale Bedingungen wie fachliche Verknüpfung, Klassifikation,
Dokumentstatus oder Einzelfreigabe verlangen. Eine Plattformablehnung kann dort
nur weiter eingeschränkt und nie in eine Freigabe umgewandelt werden.

Core Storage wertet keine Work-Rollen und keine App-Entitlements aus. Es nimmt
nur eine bereits autorisierte, exakt begrenzte interne Storage-Operation an.
Bei einer späteren Prozessgrenze authentifiziert sich der Service als eigener
Principal; eine kurzlebige Delegation hält Work-Auslöser und Service-Ausführer
getrennt. Ein Transfer-Ticket ist kein Generalschlüssel und ersetzt weder die
erneute Prüfung vor Veröffentlichung noch die dokumentlokale Regel.

Admin-Konten dürfen Providerkonfiguration und Betriebszustand verwalten, aber
keine Tenant-Dokumente oder Blobinhalte lesen. Agents, Plugins und KI erhalten
keine Storage-Credentials oder direkte Blob-Ressourcen. Der Vertrag steht in
[`ADR-021`](adr/ADR-021-interner-dokument-blob-und-transfervertrag.md); Änderungen
vorbehalten.

## Authentifizierungs-Audit

Erfolgreiche Session-Ausstellung, Passwortänderung und Logout werden in derselben
Transaktion wie die jeweilige Zustandsänderung protokolliert. Abgelehnte oder
gedrosselte Logins erhalten ebenfalls Security-Audit-Einträge, geben nach außen
aber immer dieselbe generische Fehlermeldung zurück. Audit-Details enthalten
keine Passwörter, MFA-Codes, Sessiontokens oder eingegebenen Login-Namen.
Bei Passwort-Logins werden das Zurücksetzen eines erfolgreichen Throttle-Zustands
und die Session- beziehungsweise MFA-Ausstellung in einer Schreibtransaktion
gespeichert. Bei ungültigen Zugangsdaten werden Throttle-Fortschritt und
Denied-Audit ebenfalls atomar geschrieben. Dadurch entstehen keine getrennten
Commit-Runden für logisch zusammengehörige Authentifizierungsfolgen.

## Audit-Protokoll lesen

`GET /admin/v1/security-audit` benötigt eine gültige Admin-Session, einen
Installationsscope und `core.audit.security-event.read`. Die Berechtigung ist
getrennt von Benutzer-, Rollen- und Mandantenverwaltung und als `high`
klassifiziert. Die Installation-Administrator-Systemrolle erhält sie explizit.
Die Klassifikation `high` allein löst keine implizite globale
Re-Authentifizierung aus; eine solche Anforderung müsste für den Endpunkt
aktionsgebunden versioniert werden.

Die Timeline ist auf 100 Einträge je Cursor-Seite begrenzt und kann nach Tenant,
exaktem Ereignistyp und Ergebnis gefiltert werden. Der API-Vertrag gibt weder
das freie interne `details`-Objekt noch Session-IDs aus. Jeder erfolgreiche
Abruf erzeugt atomar `core.audit.security-events-listed.v1` mit globalem Kontext.
Eine eigene RLS-Policy erlaubt der Admin-Runtime in einer tenantlosen
Installationstransaktion ausschließlich genau dieses Audit-Ereignis; beliebige
globale Writes bleiben abgewiesen.

## Erweiterungsregel

Neue Core- oder Fachberechtigungen werden versioniert registriert, einer Access
Plane und mindestens einem registrierten Ressourcentyp zugeordnet. HTTP-Routen
dürfen Rollen niemals direkt vergleichen; sie lösen zuerst den Actor aus der
Session auf und prüfen dann eine registrierte Berechtigung gegen die konkrete
Ressourcenreferenz. Fachliche Freigaben bleiben Aufgaben von `work`-Konten und
verwenden später Approval Checkpoints statt Admin-Rechte.

Planungsstand: Registry, `ResourceRef`, Ressourcendatenprofile,
Permission-Processing-Policies und die gemeinsame fail-closed `Decision` sind
implementiert. Der `ProcessingContext` ist an die serverseitige Permission-
Ressourcen-Policy gebunden, aber noch nicht an ein betreiberseitig freigegebenes
Verarbeitungsverzeichnis. Policy-Facts, konfigurierbare Vererbungsregeln und
öffentliche App-Registrierung und die Laufzeitkopplung des App-Gates folgen
versioniert; Änderungen vorbehalten.
