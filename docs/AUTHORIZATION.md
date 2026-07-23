# WERK – Autorisierung und Kontoprovisionierung

Stand: 2026-07-21

## Sicherheitsgrenze

WERK entscheidet Zugriffe serverseitig aus fünf voneinander unabhängigen
Bestandteilen:

1. Die Session gehört exakt zu einer Access Plane (`work`, `admin` oder
   `service`).
2. Eine aktive Rollenzuweisung gehört derselben Access Plane an.
3. Rolle und registrierte Berechtigung gehören derselben Access Plane an.
4. Die Berechtigung ist ausdrücklich für den aktiven, registrierten
   Ressourcentyp zugelassen.
5. Der Scope der Zuweisung deckt die angeforderte Ressource und ihren Tenant ab.

Ein `admin`-Konto ist deshalb kein stärkeres `work`-Konto. Es kann Identitäten
und Plattformkonfiguration verwalten, erhält aber dadurch keine fachliche
Entscheidungsbefugnis im Workspace.

Der zugrunde liegende Plattformvertrag ist in
[`ADR-016`](adr/ADR-016-plattformweiter-ressourcen-und-autorisierungsvertrag.md)
festgelegt.

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

`POST /admin/v1/work-users` erfordert eine gültige Admin-Session mit MFA und die
Installationsberechtigung `core.identity.work-account.create`. Der Vorgang legt
in einer Admin-Datenbanktransaktion Person, Membership, Work-Konto,
Password-Credential, tenantgebundene Workspace-Rolle und Zuweisung an.

Das temporäre Passwort muss beim ersten Login geändert werden. Derselbe Commit
enthält das Audit-Ereignis `identity.work-account.created.v1` und den
Outbox-Eintrag `core.identity.work-account-created.v1`. Ein Teilzustand ohne
Audit oder Rollenbindung kann damit nicht sichtbar werden.

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
abgelehnt. Eine Einheit mit aktiven Untereinheiten oder aktuell wirksamen
Memberships kann nicht archiviert werden.

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

`PUT /admin/v1/work-users/{accountId}/roles` ersetzt die aktuell wirksamen
tenantgebundenen Work-Rollen eines Arbeitskontos. Konto, Rollen und Scope müssen
demselben Mandanten angehören. Datenbanktrigger, RLS und eine Eindeutigkeitsregel
für aktive Zuweisungen sichern diese Grenze zusätzlich. Rollenerzeugung und
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

## Audit-Protokoll lesen

`GET /admin/v1/security-audit` benötigt eine MFA-bestätigte Admin-Session, einen
Installationsscope und `core.audit.security-event.read`. Die Berechtigung ist
getrennt von Benutzer-, Rollen- und Mandantenverwaltung und als `high`
klassifiziert. Die Installation-Administrator-Systemrolle erhält sie explizit.

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
