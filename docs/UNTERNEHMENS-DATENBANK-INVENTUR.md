# WERK – Inventur zu Unternehmensdatenbanken, Gruppen und Benutzerzuordnung

**Stand:** 29.07.2026  
**Status:** Architekturanalyse, noch kein angenommener Zielvertrag  
**Gegenstand:** Einzelunternehmen, Unternehmensgruppen, verwaltete
PostgreSQL-Datenbanken und unternehmensbezogene Benutzerzuordnung  
**Nicht enthalten:** angenommene ADR-Entscheidung, produktiver
Provisionierungsauftrag oder Änderung bestehender Daten

## 1. Anlass und bestätigte Arbeitsrichtung

WERK soll nicht als Single-Company-Produkt festgelegt werden. Eine Installation
soll sowohl ein einzelnes Unternehmen als auch eine Unternehmensgruppe mit
mehreren getrennten Unternehmen verwalten können. Als zu untersuchende
Arbeitsrichtung erhält jedes Unternehmen eine eigene logisch verwaltete
PostgreSQL-Datenbank. Ob mehrere dieser Datenbanken auf demselben lokalen
PostgreSQL-Cluster, auf getrennten Clustern oder bei einem Cloud-Provider liegen,
ist eine austauschbare Betriebs- und Placemententscheidung.

Diese Richtung ist noch kein angenommener Architekturvertrag. Insbesondere Ort
und Reichweite der Identity-Wahrheit, das Gruppenmodell und der genaue
Löschvertrag sind noch zu bestätigen. Die bestehenden ADRs und Migrationen
werden deshalb durch diese Inventur nicht stillschweigend umgedeutet.

Unabhängig von der späteren physischen Verteilung bleiben folgende Regeln
verbindlich:

- PostgreSQL ist die fachliche Wahrheit.
- Jede unternehmensbezogene Operation trägt einen expliziten, serverseitig
  bestätigten Tenant-Kontext.
- `tenant_id`, RLS, Kontoart, Policy, Audit und Outbox bleiben auch innerhalb
  einer exklusiven Unternehmensdatenbank erhalten.
- Admin-, Work-, Service- und Agent-Zugriff bleiben getrennt.
- Eine Unternehmensgruppe erteilt niemals automatisch Zugriff auf die Daten
  ihrer Unternehmen.
- Fachänderung, lokales Audit und lokale Outbox werden in derselben
  Unternehmensdatenbank atomar gespeichert.

## 2. Heutiger Repository-Stand

Der aktuelle Code implementiert noch ein anderes physisches Modell:

- Alle 43 Migrationen bilden eine vollständige Installation in genau einer
  PostgreSQL-Datenbank.
- Mehrere Tenants sind durch `tenant_id`, zusammengesetzte Fremdschlüssel und
  `FORCE ROW LEVEL SECURITY` innerhalb dieser Datenbank getrennt.
- API, Identity, Administration und Worker verwenden je Prozessrolle genau
  einen statischen DSN und Pool.
- Tenant-Anlage, Work-Account, Person, Membership, Rolle, Audit, Outbox und
  Projektionen können heute bewusst in einer lokalen Transaktion verbunden
  werden.
- Es existieren weder Control-Plane-Datenbank, Company-zu-Datenbank-Directory,
  Company-DB-Router, Pool-Flotte noch Unternehmensgruppenmodell.
- `accounts` enthält installationsweite Admin- und tenantgebundene Work-/Service-
  Konten. Sessions, Providerbindungen und Einladungen referenzieren dieselbe
  lokale Kontotabelle.

Eine Unternehmensdatenbank kann deshalb nicht nur als zusätzlicher DSN an die
heutige Tenant-Anlage gehängt werden. Der Datenowner und die heute lokalen
Transaktionsgrenzen müssen vorher ausdrücklich neu geschnitten werden.

## 3. Begriffs- und Eigentumsgrenzen

Die folgenden Ebenen dürfen nicht vermischt werden:

| Ebene | Bedeutung | Sicherheitswirkung |
|---|---|---|
| Installation | ein WERK-Verwaltungs- und Identity-Rahmen | installationsweite Admin-, Release- und Betriebsgrenze |
| Unternehmensgruppe | optionaler Verwaltungsverbund mehrerer Unternehmen | keine automatische Fach- oder Datenberechtigung |
| Unternehmen / Tenant | eigenständige Daten- und Sicherheitswelt | expliziter Tenant-Kontext und genau eine autoritative Company-DB |
| Organisationseinheit | Standort, Bereich, Abteilung oder Team innerhalb eines Unternehmens | zusätzlicher lokaler Organisations- und Berechtigungsscope |
| Add-on-Installation | lokal aktiviertes Fachmodul eines Unternehmens | keine Rolle und keine Berechtigung ohne lokale Entitlements und Policies |

Eine operative Muttergesellschaft kann selbst ein Unternehmen mit eigener
Company-DB und zugleich Mitglied einer Unternehmensgruppe sein. Die Gruppe ist
nicht automatisch eine weitere Company-DB. Benötigt die Gruppe eigene operative
Fachdaten, wird dafür ein eigenes Unternehmen angelegt statt die Control Plane
zur Fachanwendung zu machen.

```text
WERK-Installation / Control Plane
├── Admin- und Betriebsgrenze
├── optionale Unternehmensgruppe
│   ├── Unternehmen A / Tenant A -> Company-DB A
│   │   └── Bereiche, Abteilungen, Teams und lokale Add-ons
│   └── Unternehmen B / Tenant B -> Company-DB B
│       └── Bereiche, Abteilungen, Teams und lokale Add-ons
└── alleinstehendes Unternehmen C / Tenant C -> Company-DB C
```

Der Einzelunternehmensfall ist keine zweite Architektur. Er ist derselbe Vertrag
mit genau einer registrierten Unternehmensdatenbank. Eine Komfortvorauswahl in
der Oberfläche darf daraus keine serverseitige Mengeninvariante machen.

## 4. Logisches Zielbild als Prüfmodell

### 4.1 Control Plane

Eine mögliche Control-Datenbank besitzt ausschließlich installations- und
flottenweite Wahrheit:

- Installation und Identity-Realm,
- Unternehmens- und Gruppenverzeichnis,
- opake Company-zu-Data-Plane-Bindung,
- gewünschter und beobachteter Lebenszykluszustand,
- Provisionierungs-, Migrations-, Backup-, Restore- und Löschoperationen,
- globale Admin-Identität und Admin-RBAC,
- gegebenenfalls globale Authentifizierungs-Principals,
- kanonische, versionierte Modul-, Permission-, Ressourcen- und
  Processing-Verträge,
- Control-Audit, Control-Outbox und minimale Betriebsprojektionen.

DSNs, Datenbankpasswörter und Cloud-Credentials gehören nicht in dieses
Directory. Es speichert nur opake Provider-, Placement- und Secret-Referenzen.

### 4.2 Company Data Plane

Eine Unternehmensdatenbank besitzt genau eine Unternehmenswahrheit:

- einen unveränderlichen Datenbankanker aus Company-/Tenant-ID, Realm-ID,
  Data-Plane-ID und Bindungsgeneration,
- lokale Personen, Organisationen, Organisationseinheiten und Memberships,
- tenantlokale Work-, Service- und Agent-Konten,
- lokale Rollen, Zuweisungen, Access-Gruppen und App-Entitlements,
- lokale Add-on-Installationen und Fachdaten,
- Dokument-, Workflow-, Aufgaben- und Storage-Metadaten,
- lokalen Business-/Security-Audit, Outbox und Consumer-Receipts,
- lokalen Core- und Add-on-Migrationsstand.

Globale Vertragskataloge dürfen versioniert in eine Company-DB projiziert
werden, wenn lokale Fremdschlüssel, Trigger oder Autorisierung sie benötigen.
Die Control Plane bleibt dabei kanonischer Owner; die lokale Kopie ist ein
verifizierter Release-Snapshot und keine konkurrierende Wahrheit.

### 4.3 Physische Platzierung

| Placement | Nutzen | Grenze |
|---|---|---|
| mehrere Company-DBs auf einem Cluster | kleinster Self-hosted-Einstieg, günstiger Betrieb | gemeinsamer Ausfall-, WAL-, Rollen- und Ressourcenbereich |
| Company-DBs auf mehreren Clustern | bessere Last- und Ausfalltrennung | mehr Provisionierung, Monitoring und Backupkoordination |
| eigener Managed-Cluster je Unternehmen | stärkste betriebliche Isolation | höhere Kosten und längere Bereitstellung |
| kundeneigene oder hybride PostgreSQL-Zelle | Daten verbleiben beim jeweiligen Betreiber | Pairing, Erreichbarkeit, Versionen und Supportgrenze werden Teil des Vertrags |

Die Anwendung soll ausschließlich eine `CompanyDatabaseBinding` auflösen. Der
Provider entscheidet über das Placement; Requestdaten dürfen niemals Host,
Datenbankname oder DSN bestimmen.

## 5. Was „atomar“ in diesem Modell bedeutet

Eine PostgreSQL-Transaktion kann nur in einer konkreten Datenbank die lokale
Wahrheit atomar ändern. `CREATE DATABASE` und `DROP DATABASE` laufen außerdem
nicht in einem Transaktionsblock. Control-DB, Company-DB und Provideraktion
können deshalb nicht als eine einzige gewöhnliche Transaktion behandelt werden.

Der Vertrag lautet stattdessen:

```text
Control-DB
  Command + gewünschter Zustand + Operation + Audit + Outbox atomar

Provisioner oder Company-DB
  idempotenter Schritt
  + lokale Änderung + lokales Audit + lokale Outbox atomar
  + dauerhaftes Receipt

Control-DB
  Receipt übernehmen + beobachteten Zustand fortschreiben atomar
```

Jeder Ablauf benötigt:

- eine stabile Operation-ID und einen Idempotency-Key,
- getrennten `desired_state` und `observed_state`,
- monotone Route-, Bindungs- und Security-Generationen,
- Lease, Retry und begrenztes Backoff,
- deduplizierende Command-Inbox und Applied-/Failed-Receipts,
- einen sicheren Zwischenzustand, der noch keinen Zugriff erlaubt,
- sichtbaren Teilfehler statt erfundener Gesamterfolge.

Verteilte PostgreSQL-2PC-Transaktionen sind kein notwendiger Startmechanismus.
PostgreSQL sieht `PREPARE TRANSACTION` für externe Transaction Manager vor und
warnt vor langlebigen vorbereiteten Transaktionen und ihren gehaltenen Locks.
Für WERK sind kleine lokale Transaktionen und nachvollziehbare Sagas die
einfachere, wiederanlaufbare Grundlage.

## 6. Lebenszyklus- und Fehlerpfade

| Aktion | Lokal atomar | Außerhalb der lokalen Transaktion | Sicherer Fehlerzustand |
|---|---|---|---|
| Gruppe anlegen | Gruppe, Audit und Outbox in der Control-DB | keiner | Gruppe fehlt vollständig oder ist vorhanden |
| Unternehmen vormerken | Company-ID, gewünschtes Placement, Operation, Audit und Outbox in der Control-DB | Provisionierungsjob | `pending` ohne Route |
| Datenbank anlegen | Datenbankanker, Tenant, Seeds und jede Migration in der Company-DB | `CREATE DATABASE`, Rollen und Credentials | isolierte `provisioning_failed`-DB ohne Route |
| Unternehmen aktivieren | lokaler Tenantstatus, Audit und Outbox; danach Control-Receipt | Readiness und Attestation | erst `active`, wenn beide Seiten bestätigt sind |
| Unternehmen aussetzen | Route und Generation zuerst in der Control-DB sperren; lokaler Status separat atomar | Pools schließen und Worker anhalten | sofort zentral gesperrt, lokal gegebenenfalls `reconciling` |
| Unternehmen umplatzieren | Restore und Prüfung in einer neuen Company-DB | Datenübertragung und Provideraktion | alte Route bleibt, bis neuer Datenbankanker bestätigt ist |
| Route umschalten | Binding und Generation in einer Control-Transaktion | alte Pools leeren und alte DB einzäunen | genau eine aktive Bindungsgeneration |
| Unternehmen migrieren | jede Migration in genau einer Company-DB unter Lock | Flottenplanung und Canary-Wellen | nur kompatible Versionen werden geroutet |
| Unternehmen sichern | konsistenter Snapshot der Company-DB | Verschlüsselung, Upload und Restore-Probe | fehlgeschlagener Lauf wird nicht als Backup veröffentlicht |
| Unternehmen wiederherstellen | Restore in eine frische DB und dortige Prüfung | Objektabgleich und späterer Route-Swap | alte DB bleibt bis zur bestätigten Umschaltung unangetastet |
| Unternehmen archivieren | Lebenszyklusstatus, Audit und Outbox | optionale Anzeige-/Retentionprojektion | Daten bleiben erhalten und nicht schreibbar |
| endgültige Löschung anfordern | Löschoperation, Sperre und Tombstone in der Control-DB | Retention, Legal Hold, Export und Deprovisionierung | dauerhaft ungeroutet; Teilschritte wiederholbar |
| Datenbank endgültig entfernen | nicht gemeinsam mit Control-DB atomar | Verbindungen drainen, Credentials widerrufen, Object Storage und DB löschen | `deletion_failed` bleibt gesperrt; ID wird nie wiederverwendet |

Das Entfernen eines Unternehmens aus einer Gruppe, das Archivieren eines
Unternehmens und das irreversible Löschen seiner Datenbank sind drei getrennte
Aktionen. Eine Gruppenlöschung darf niemals implizit alle Company-DBs löschen.

## 7. Einzelunternehmen und Unternehmensgruppe

### 7.1 Einzelunternehmen

- Die Installation besitzt genau eine aktive Company-Bindung.
- Der Server kann diesen Kontext nach erfolgreicher Prüfung vorauswählen.
- Die Unternehmensgrenze bleibt in Session, API, Datenbankanker, `tenant_id`
  und Audit sichtbar.
- Provisionierung, Migration, Backup und Restore verwenden bereits denselben
  Flottenvertrag wie später mehrere Unternehmen.
- Wird ein zweites Unternehmen angelegt, ist kein Profilwechsel und keine
  Datenmodellmigration erforderlich.

### 7.2 Unternehmensgruppe

- Die Gruppe referenziert mehrere Company-IDs, niemals deren lokale Tabellen.
- Jede Company bleibt alleinige Schreibwahrheit für Personen, OUs, Rollen,
  Add-ons und Fachdaten.
- Eine gruppenweite Aktion wird in unabhängige lokale Commands aufgefächert.
- Teilerfolg wird pro Unternehmen gezeigt und kann einzeln wiederholt werden.
- Eine Live-Gruppenansicht muss nicht erreichbare Company-DBs als `partial`
  ausweisen. Eine asynchrone Projektion zeigt Quelle, Generation, Schemaversion,
  Beobachtungszeit und Staleness.
- Gruppenweite Berichte oder Personensichten benötigen einen eigenen
  Datenverarbeitungs- und Berechtigungsvertrag. Sie folgen nicht automatisch
  aus der Gruppenmitgliedschaft.

## 8. Benutzerzuordnung

### 8.1 Objekte, die getrennt bleiben müssen

| Objekt | Scope | Bedeutung |
|---|---|---|
| `WorkPrincipal` | Identity-Realm / Control Plane | Authentifizierungsidentität mit Credential, Passkey, MFA und Providerbindung |
| `PrincipalCompanyLink` | Principal zu genau einer Company | serverseitige Routing- und Zutrittsbindung, aber keine Rolle |
| `WorkAccount` | genau eine Company-DB | lokaler Arbeitsakteur mit Status und Security-Generation |
| `Person` | genau eine Company-DB | lokaler Personen-/Party-Datensatz; keine globale PII-Zusammenführung |
| `Membership` | genau eine Company-DB und OU | lokale organisatorische Zuordnung |
| `RoleAssignment` | genau eine Company-DB | lokale Berechtigung; wird nicht über Unternehmen vereinigt |

Ein gemeinsamer Principal ist nur nötig, wenn wirklich ein Login mehrere
Unternehmen öffnen soll. Alternativ kann jede Company einen eigenen Identity-
Realm besitzen; dann muss der Company-Kontext bereits vor Passwort-, Passkey-
oder Providerprüfung feststehen. Beide Varianten sind technisch möglich, aber
sie erzeugen verschiedene Nutzer- und Migrationsverträge.

### 8.2 Ein Login für mehrere Unternehmen

Für einen gemeinsamen Login ist der kohärente Prüfweg:

```text
Credential / Passkey / externer Provider
  -> globalen WorkPrincipal authentifizieren
  -> aktive PrincipalCompanyLinks serverseitig laden
  -> bei genau einem Link serverseitig vorauswählen
  -> bei mehreren Links kurze LoginContinuation und Auswahl anzeigen
  -> ausgewählten opaken Link serverseitig auflösen
  -> Company-DB und lokalen WorkAccount prüfen
  -> neue, exakt companygebundene Work-Session ausstellen
```

Die Session bindet mindestens Principal, Company/Tenant, lokalen WorkAccount,
Principal-/Link-/Account-Generation und Datenbank-Bindungsgeneration. Ein
Company-Wechsel erzeugt eine neue Session; die Tenant-ID einer bestehenden
Session wird nie geändert. Eine Admin-Session bleibt tenantlos und kann nicht in
eine Work-Session umgeschaltet werden.

E-Mail-Adresse oder Anzeigename dürfen keine automatische Zusammenführung
zweier Personen oder Konten auslösen. Bei OIDC ist nur die geprüfte Kombination
aus Providerinstanz beziehungsweise Issuer und unveränderlichem Subject eine
stabile externe Bindung. Die Unternehmenszuordnung und Rollen stammen weiterhin
aus WERK.

### 8.3 Join, Leave und Wechsel

| Vorgang | Ablauf |
|---|---|
| neuer Principal tritt Company bei | globales Join-Intent und Einladung; lokaler zunächst deaktivierter Person-/Account-/Membership-/Rollen-Satz; Credential-Aktivierung; lokales Aktivieren; globaler Link zuletzt `active` |
| vorhandener Principal tritt weiterer Company bei | kein automatisches neues Passwort; authentifizierte Annahme des Links; separater lokaler Person- und WorkAccount-Datensatz |
| Company verlassen | Link zuerst sperren und Generation erhöhen; lokale Session-/Account-Generation erhöhen und Account deaktivieren; Link nach Receipt `left` |
| Abteilung wechseln | ausschließlich lokale Transaktion über alte/neue Membership, betroffene lokale Rollen/Entitlements, Audit und Outbox |
| in andere Company wechseln | Zielbeitritt und Quellaustritt als Saga; niemals `tenant_id` eines Accounts ändern und niemals Rollen oder Fachdaten automatisch kopieren |
| in mehreren Companies arbeiten | je Company eigener WorkAccount, lokale Rollen und getrennte Sessions; derselbe globale Principal ist nur die Anmeldeidentität |

Ein global deaktivierter Principal sperrt alle Company-Zugänge. Das Deaktivieren
eines `PrincipalCompanyLink` sperrt ausschließlich dieses Unternehmen. Ein lokal
fehlender, deaktivierter oder generationsfremder WorkAccount führt trotz
globalem Link immer zu `deny`.

### 8.4 Externe Provider und spätere Provisionierung

- Ein installationsweiter OIDC-Provider kann einen globalen Principal über
  seine stabile Provider-/Subject-Bindung authentifizieren.
- Ein companylokaler Provider darf nur einen bereits serverseitig an diese
  Company gebundenen Link öffnen.
- Providerclaims bestimmen weder Company/Tenant noch Rolle oder Membership.
- SCIM kann später Benutzer und Gruppen provisionieren, ersetzt aber nicht den
  WERK-Vertrag für Tenant-Zuordnung, Rollen und lokale Datenhoheit.
- Service Accounts und Agents bleiben je Company getrennte technische
  Principals; eine menschliche Multi-Company-Anmeldung darf nicht auf sie
  übertragen werden.

## 9. Add-ons und Organisationsverknüpfungen

Die bestehende OU-, Access-Group- und App-Entitlement-Grundlage lässt sich
vollständig companylokal weiterverwenden:

```text
Control Plane
  Add-on-Release + gewünschter Gruppenrollout + Fortschritt je Company

Company-DB
  lokaler Modulkatalog
  -> TenantAppInstallation
  -> lokale OU-/Access-Group-/Account-Entitlements
  -> lokale Rolle, Permission und Processing-Policy
  -> Fachaktion + Audit + Outbox
```

Ein Gruppenrollout erzeugt N lokale Installationsoperationen. Er erzeugt keine
gruppenweite OU-ID, Rolle oder Entitlement-Kante. Jede Company prüft ihre lokalen
Ziele und behält die Sperrreihenfolge Tenant -> Organisationspfad -> Kante. Ein
Add-on erhält weiterhin keinen direkten Datenbankzugriff.

## 10. Pragmatische Iterationen ohne Big Bang

### Iteration 0 – Entscheidung und Kompatibilitätsgrenze

- neues ADR für Control Plane, Company Data Plane, Datenhoheit, Identity und
  Saga-Grenze entwerfen,
- bisherige Single-Company-Aussagen ausdrücklich ablösen,
- keine vorhandene Migration verändern,
- bestehenden gemeinsamen Datenbestand noch nicht bewegen.

### Iteration 1 – Routingabstraktion im heutigen Betrieb

- `CompanyDirectory`, `CompanyDatabaseResolver` und rollengetrennte
  Transaktionsfactory einführen,
- aktuellen statischen Pool als Ein-Company-Adapter verwenden,
- unveränderlichen Data-Plane-Anker und Bindungsgeneration additiv ergänzen,
- jede Connection gegen erwartete Company, Rolle und Schemaversion attestieren.

### Iteration 2 – Lokale gemanagte zweite Company-DB

- schmalen, separaten Provisionierungsrunner einführen; API und heutiger
  Migrator erhalten kein `CREATEDB`,
- Control-Operation, Datenbankanlage, Rollen, Migration, Prüfung und Aktivierung
  idempotent verbinden,
- Pool-Flotte begrenzen und inaktive Pools schließen,
- zweite Company erst nach Cross-DB-, Fehlstart-, Restore- und
  Schemaversatztests freischalten.

### Iteration 3 – Benutzerzuordnung und Multi-Company-Login

- bestätigte Identity-Variante implementieren,
- bei gemeinsamem Login `WorkPrincipal`, `PrincipalCompanyLink` und lokale
  WorkAccounts additiv einführen,
- Sessiongenerationen und LoginContinuation prüfen,
- Join, Leave und später Transfer als wiederanlaufbare Sagas umsetzen.

### Iteration 4 – Unternehmensgruppen und Rollouts

- Gruppenverzeichnis und Company-Mitgliedschaften ergänzen,
- gruppenweite Add-on-, Migrations- und Benutzeroperationen in lokale Tasks
  auffächern,
- Teilfortschritt und Teilausfall im Betriebsdashboard anzeigen,
- keine gruppenweite Fachdatenprojektion ohne eigenen Vertrag einführen.

### Iteration 5 – Cloud, Hybrid und Placement

- lokalen PostgreSQL-Provisioner, externen Provider und Cloud-Provider hinter
  denselben Lifecycle-Port stellen,
- `werkctl`/Runner für Pairing, Status, kontrollierte Migration, Backup und
  Restore erweitern,
- Placement-Wechsel als Restore in eine neue Data Plane mit atomarem
  Route-Generation-Swap behandeln,
- dedizierte Cluster nur nach Isolation, Last oder Betreiberanforderung nutzen.

## 11. Erforderliche automatisierte Nachweise

Vor einer produktiven zweiten Company-DB sind mindestens erforderlich:

1. falsche Company-zu-Datenbank-Bindung wird vor der ersten Fachquery abgelehnt,
2. Request, Session oder Client kann keinen DSN und kein Placement behaupten,
3. Work-/Admin-/Service-Rollen bleiben pro Datenbank und API getrennt,
4. RLS und Tenant-Kontext bleiben in jeder Company-DB wirksam,
5. parallele Provisionierungsjobs erzeugen höchstens eine aktive Data Plane,
6. jeder Abbruchpunkt der Provisionierung ist idempotent wiederanlaufbar,
7. eine Company-DB kann ausfallen, ohne andere Companys oder die Control Plane
   als erfolgreich auszugeben,
8. Schema-Drift und inkompatible Versionen werden quarantänisiert,
9. Restore erhöht Binding-/Security-Generationen und reaktiviert keine alten
   Sessions, Rollen oder Ereignisse,
10. Join-/Leave-Saga erlaubt niemals einen global aktiven Link ohne gültigen
    lokalen Account,
11. ein Principal mit zwei Companies erhält pro Company getrennte Rollen,
    Memberships, App-Entitlements und Sessions,
12. Gruppenaktionen zeigen Partial Success und sind je Company wiederholbar,
13. Backup und Restore werden pro Company-DB samt Object-Storage-Manifest
    geprüft,
14. endgültige Löschung kann bei Retention oder Legal Hold nicht fortfahren,
15. bestehende Ein-Company-Installationen können ohne Datenverlust adoptiert
    oder über einen geprüften Export-/Importpfad migriert werden.

## 12. Auswirkungen auf den aktuellen Code

Direkt betroffen wären später mindestens:

- `internal/platform/database`: statischer Pool wird durch eine serverseitige,
  companygebundene Transaktionsfactory ergänzt,
- `internal/platform/config`: feste DSNs werden für Data Planes durch opake
  Providerbindungen ergänzt,
- `cmd/api`, `cmd/worker` und `cmd/migrate`: Composition Root, Worker-Fan-out und
  getrennte Control-/Company-Migrationsketten,
- `internal/platform/adminstore`: Tenant-Anlage wird zu einer asynchronen
  Company-Provisionierungsoperation,
- `internal/platform/identitystore`: Login, Providerbindung, Einladung, Session
  und Actor-Auflösung gemäß bestätigter Identity-Variante,
- `internal/platform/outbox`, `auditexport` und `operationsstore`: lokale Queues
  plus installationsweite Betriebsprojektion,
- `deploy/postgres` und Backup-Wrapper: Company-spezifische Rollen,
  Verbindungsgrenzen, Sicherung und Restore,
- `cmd/werkctl`: kontrollierter Provisionierungs-, Migrations-, Backup-,
  Restore-, Pairing- und Deprovisionierungsrunner.

Die vorhandenen 43 Migrationen bleiben unverändert. Eine Control-Plane-
Migrationslinie und additive Company-Data-Plane-Migrationen benötigen getrennte
Checksummen, Locks und Versionsstände.

## 13. Quellen und technische Plausibilisierung

- PostgreSQL ordnet Datenbanken als oberste SQL-Objektgrenze unterhalb eines
  Clusters ein. Eine Verbindung adressiert genau eine Datenbank; Rollen und
  weitere wenige Objekte sind dagegen clusterweit:
  <https://www.postgresql.org/docs/current/manage-ag-overview.html>
- `CREATE DATABASE` benötigt `CREATEDB` beziehungsweise Superuserrechte und kann
  nicht innerhalb eines Transaktionsblocks ausgeführt werden:
  <https://www.postgresql.org/docs/current/sql-createdatabase.html>
- `DROP DATABASE` ist nicht rückgängig zu machen, läuft nicht in einem
  Transaktionsblock und erfordert das Beenden beziehungsweise Fernhalten
  bestehender Verbindungen:
  <https://www.postgresql.org/docs/current/sql-dropdatabase.html>
- Ein `pg_dump` ist innerhalb einer Datenbank konsistent. `pg_dumpall` verarbeitet
  mehrere Datenbanken nacheinander; ihre Snapshots sind nicht untereinander
  synchronisiert:
  <https://www.postgresql.org/docs/current/backup-dump.html>
- PostgreSQL beschreibt 2PC als Mechanismus für einen externen Transaction
  Manager und warnt vor lange offenen vorbereiteten Transaktionen:
  <https://www.postgresql.org/docs/current/sql-prepare-transaction.html>
- CloudNativePG zeigt als ein mögliches, nicht vorgeschriebenes Providerbeispiel
  einen deklarativen `Database`-Lifecycle mit beobachtetem Zustand und getrenntem
  Reclaim-Verhalten:
  <https://cloudnative-pg.io/docs/devel/declarative_database_management/>
- OpenID Connect bindet eine externe Identität stabil an Subject und Issuer;
  E-Mail und Anzeigename sind keine belastbaren Identitätsschlüssel:
  <https://openid.net/specs/openid-connect-core-1_0-18.html>
- SCIM beschreibt auch einen Client mit Zugriff auf mehrere getrennte Tenants,
  schreibt die konkrete Tenant-Zuordnung und deren Autorisierung aber bewusst
  nicht vor:
  <https://www.rfc-editor.org/info/rfc7644/>

