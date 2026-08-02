# ADR-033 – Protokollfähige Identity-Provider und Registry-Kopplung

**Status:** Vorgeschlagen  
**Datum:** 2026-07-30

## Kontext

Core Identity besitzt bereits lokale Konten, Credentials, Sessions,
`identity_providers`, unveränderliche Provider-/Subject-Bindungen und den
providerneutralen `VerifiedIdentity`-Vertrag. OIDC-, SAML- und LDAP-Adapter sind
noch nicht implementiert. Ein vorhandener Metadatensatz aktiviert deshalb
heute ausdrücklich keinen externen Anmeldeweg.

Die globale Service-/Provider-Registry aus
[`ADR-025`](ADR-025-globale-service-und-provider-registry.md) beschreibt
logische Dienste, technische Capabilities, service-spezifische
Providerregistrierungen und deren ausdrückliche Bindungen. Sie ist kein
Konfigurations-, Secret-, Health- oder Routingdienst. Core Identity behält nach
[`ADR-010`](ADR-010-core-identity-als-interne-identitaetsquelle.md) und
[`ADR-014`](ADR-014-principals-provider-credentials-und-audiences.md) die
Hoheit über Konten, Anmeldeprovider, Subject-Bindungen, Kontoart, Tenant,
Audience, Berechtigungen und Audit.

Die Begriffe „LDAP“, „Active Directory“, „OIDC“, „SAML“, „Univention“ und
„Domain“ dürfen dabei nicht als austauschbare Produktfähigkeiten behandelt
werden:

- LDAP ist ein Verzeichniszugriffsprotokoll.
- OIDC und SAML dienen in diesem Schnitt der föderierten Anmeldung an WERK.
- Ein Produkt kann Verzeichnis, Anmeldeprovider und weitere Dienste zugleich
  anbieten, ohne dass eine einzelne Registrierung alle Fähigkeiten erhält.
- LDAP-, OIDC- oder SAML-Unterstützung weist weder einen Windows-Domänendienst
  noch einen funktionierenden Windows-Domänenbeitritt nach.
- „Domain“ bezeichnet in anderen WERK-ADRs außerdem abgegrenzte Authority-
  Bereiche. Eine spätere Geräteintegration muss deshalb ausdrücklich
  „Geräte- und Netzwerkdomäne“ heißen.

Ohne einen engeren Vertrag würde ein Produktname zum vermeintlichen
Superprovider werden und WERK Fähigkeiten anzeigen, die weder implementiert
noch geprüft sind.

## Entscheidung

### Getrennte logische Dienste

Der erste Vertrag verwendet zwei voneinander unabhängige Dienste unter der
Hoheit von `core.identity`:

```text
core.identity.service.directory                 contract_version=1
core.identity.service.login-federation          contract_version=1
```

`directory` liest begrenzte, externe Verzeichnisprojektionen für einen
ausdrücklichen Tenant-Kontext. `login-federation` führt eine interaktive
OIDC- oder SAML-Anmeldezeremonie aus und liefert höchstens einen geprüften
Identitätsnachweis an Core Identity. Ein Directory-Provider ist nicht
automatisch ein Login-Provider und ein Login-Provider ist nicht automatisch
ein Directory-Provider.

Ein physisches Produkt erhält pro Dienst eine eigene `ProviderRegistration`.
Eine Univention-Installation kann beispielsweise eine Registrierung für
Verzeichniszugriff und eine zweite Registrierung für Keycloak-basierte
Anmeldung besitzen. Nur eine spätere UCS-Installation mit tatsächlich
vorhandenem und geprüftem Samba/AD-Dienst könnte zusätzlich einen getrennten
Geräte-Domain-Provider erhalten. Nubus für Kubernetes erhält durch OpenLDAP
und Keycloak keine solche Fähigkeit.

### Directory-Service v1

Der Dienst registriert ausschließlich einzeln bindbare, tenantgebundene
Lese-Capabilities:

```text
core.identity.service.directory.capability.user.read
core.identity.service.directory.capability.group.read
core.identity.service.directory.capability.group-membership.read
core.identity.service.directory.capability.change-set.read
```

Alle vier Capabilities verwenden `operation_boundary=tenant`. Eine
installationsweit konfigurierte Directory-Verbindung darf sie für mehrere
Tenants ausführen, ohne den Tenant-Kontext der konkreten Operation aufzuheben.
Eine tenantkonfigurierte Verbindung darf ausschließlich Operationen desselben
Tenants bedienen.

Jede Capability ist optional. Ein Adapter, der Benutzer und Gruppen lesen kann,
aber keinen stabilen Änderungs-Cursor anbietet, erhält keine
`change-set.read`-Bindung. Ein `active`-Status des Providers ersetzt diese
Bindung und deren Laufzeitprüfung nicht.

Das normierte Ergebnis enthält nur begrenzte, explizit gemappte Attribute,
einen stabilen externen Objektbezeichner, Objekttyp, Quellrevision und
gegebenenfalls einen opaken Paging- oder Änderungs-Cursor. Ein externer
Objektbezeichner ist noch kein WERK-Account, keine `Party`, keine Membership und
keine Rolle. Cursor sind providerlokale, sensible Synchronisationsmetadaten und
werden weder als freie Clientwerte noch in Events ausgegeben.

Ein ehrlicher erster LDAP-/AD-Slice umfasst:

- TLS-validierte Verbindung ohne Schalter zum Überspringen der
  Zertifikats- oder Hostnamenprüfung,
- Authentifizierung des technischen Directory-Clients über eine sichere,
  Identity-eigene Materialreferenz,
- feste Suchbasis und serverseitig erlaubte Attributabbildung,
- begrenzte, paginierte Leseoperationen mit Deadline, Größen- und
  Parallelitätsgrenzen,
- stabile, getestete Fehlerabbildung ohne Rückgabe von DN, Filtern,
  Zugangsdaten oder unnötigen Personendaten,
- Integrations- und Negativtests gegen jedes ausdrücklich unterstützte
  Providerprofil.

Die Aussage „LDAP/AD-Verzeichniszugriff verfügbar“ ist erst zulässig, wenn der
konkrete Adapter, die Providerkonfiguration und mindestens eine Capability-
Bindung ausführbar geprüft wurden. Sie bedeutet noch keine Synchronisation,
Provisionierung oder Anmeldung.

### Login-Federation-Service v1

Der Dienst registriert zwei installationsgebundene Capabilities:

```text
core.identity.service.login-federation.capability.oidc.login
core.identity.service.login-federation.capability.saml.login
```

Beide verwenden in Version 1 `operation_boundary=installation`. Vor der
Identitätsprüfung existiert noch kein vertrauenswürdiger Tenant-Kontext. Wegen
der bestehenden Registry-Invariante dürfen deshalb in v1 nur
`config_scope=installation`-Provider diese Capabilities implementieren. Ein
späterer companylokaler Anmeldeprovider benötigt einen eigenen Vertrag, in dem
die Providerauswahl und der Tenant serverseitig vor der Zeremonie gebunden
werden; ein vom Client übermittelter Tenant bleibt unzulässig.

OIDC v1 umfasst genau:

- Authorization Code Flow mit PKCE,
- serverseitig erzeugten und kurzlebig gebundenen `state`- und `nonce`-Wert,
- exakt konfigurierte Issuer-, Redirect-URI- und Clientbindung,
- Signatur-, Zeit-, Audience- und Token-Typ-Prüfung,
- exakte Identitätsauflösung über die geprüfte Kombination aus Provider und
  unverändertem `issuer + sub`,
- Replay-Schutz, allgemeine externe Fehlerantwort und Security-Audit.

SAML v1 umfasst genau:

- serverseitig erzeugte AuthnRequest- und Relay-State-Bindung,
- festes IdP-Metadaten- und Zertifikatsvertrauen,
- Signatur-, Issuer-, Audience-, Destination-, Zeit- und
  `InResponseTo`-Prüfung,
- Replay-Schutz und exakte Auflösung eines stabilen, konfigurierten Subjects,
- allgemeine externe Fehlerantwort und Security-Audit.

Beide Adapter liefern ausschließlich den vorhandenen `VerifiedIdentity`-
Umfang: Provider-Key, Provider-Subject, Authentifizierungsmethode,
Assurance und serverbestätigten Zeitpunkt. Core Identity löst danach die
aktive, unveränderte Subject-Bindung auf und bestimmt Kontoart, Tenant,
Audience, Session und Berechtigungen. Claims oder Assertions dürfen diese
Werte weder setzen noch erweitern.

### Registry bleibt metadata-only

Die globale Registry speichert für diese Dienste weiterhin nur:

- Dienst- und Capability-Keys mit exakten Vertragsversionen,
- Provider-ID und service-spezifischen Provider-Key,
- den im Code bekannten Adapter-Key,
- Konfigurationsscope und gegebenenfalls Tenant-ID,
- Lifecycle, Registry-Version und Revision,
- ausdrückliche Provider-/Capability-Bindungen.

Nicht in die Registry gehören:

- URLs, Issuer-Metadaten, Redirect-URIs, LDAP-Basen oder Suchfilter,
- Client-IDs, Passwörter, Tokens, private oder öffentliche Schlüssel,
  Zertifikatsinhalte oder Secretwerte,
- OIDC-Claims-, SAML-Attribut- oder LDAP-Schema-Mappings,
- Subject-Bindungen, Konten, Tenant-Zuordnungen oder Rollen,
- State, Nonce, PKCE-Verifier, Relay State, Assertions oder Replaydaten,
- Paging-/Synchronisationscursor,
- Timeouts, Health, Readiness, Latenzen oder Fehlerraten.

Identity-eigene, providerlokale Konfiguration referenziert sichere Materialien
opak und versioniert. `identity_providers` und
`account_identity_bindings` bleiben die autoritative Anmeldeprovider- und
Subject-Wahrheit. Directory-Mappings, Importzustand und Cursor bleiben
ebenfalls Identity-eigene Daten und werden nicht in die globale Registry
verschoben. Eine Registry-Auflösung ist weder Berechtigung noch
Konfigurationsnachweis, Healthsignal oder Authority-Token.

### Identity-eigene Kopplungsobjekte

Die Kopplungsrichtung zeigt ausschließlich von Core Identity auf die globale
Registry. Der konzeptionelle v1-Vertrag unterscheidet zwei typisierte,
Identity-eigene Bindings:

```text
FederatedLoginProviderBinding
  identity_provider_key
  registry_provider_id
  registry_contract_version
  service_contract_version
  capability_key
  capability_version
  typed_configuration_ref
  status, revision

DirectorySourceBinding
  id, tenant_id
  registry_provider_id
  registry_contract_version
  service_contract_version
  typed_configuration_ref
  mapping_contract_version
  status, revision
```

Ein aktives `FederatedLoginProviderBinding` verweist auf genau eine OIDC- oder
SAML-Capability, deren Protokoll zur `provider_kind` des zugehörigen
`identity_providers`-Eintrags passen muss. Ein aktives
`DirectorySourceBinding` bindet eine Providerregistrierung und einen
serverbestätigten Tenant; jede Operation nennt zusätzlich die exakt benötigte
Directory-Capability. `typed_configuration_ref` adressiert eine
protokollspezifische, validierte Identity-Konfiguration und ist weder freie
JSON-Konfiguration noch Secretwert.

Provider-ID, Dienst- und Capability-Versionen werden bei jeder Nutzung gegen
die Registry frisch aufgelöst. Ein Binding kopiert keinen Registry-Lifecycle
und darf einen deaktivierten oder retired Vertrag nicht überstimmen. Die
globale Registry erhält keine Rückreferenz auf Identity-Provider, Directory-
Quellen, Subjects oder Konfiguration. Ob diese Kopplungsobjekte später als
Tabellen oder als gleichwertige typisierte Aggregate persistiert werden, wird
mit dem ersten Adapter festgelegt; ihre Ownership- und Prüfgrenze ist bereits
verbindlich.

### Typisierte Adapterkopplung

Adapter werden an der Composition Root des jeweiligen Dienstes typisiert
zusammengesetzt. Beispielhafte Adapter-Keys sind:

```text
core.identity.adapter.ldap-directory.v1
core.identity.adapter.univention-udm.v1
core.identity.adapter.oidc-login.v1
core.identity.adapter.saml-login.v1
```

Der Laufzeitweg ist immer explizit:

```text
serverseitig gewählte Provider-ID und Operation
  -> frische Registry-Auflösung für exakte Dienst-/Capability-Version
  -> Identity-eigene Providerkonfiguration und Materialreferenzen laden
  -> passenden typisierten Adapter ausführen
  -> Protokollergebnis vollständig prüfen und begrenzen
  -> Directory-Projektion zurückgeben
     oder VerifiedIdentity durch Core Identity auflösen
  -> Autorisierung, Audit und gegebenenfalls atomare Outbox getrennt anwenden
```

Es gibt keine globale Adapter-Map mit freien Typen, kein dynamisches Go-Plugin
und keine automatische Auswahl des ersten aktiven Providers.

## Ressourcen-, Berechtigungs- und Ereignisgrenze

Für eine spätere schreibende Verwaltungsoberfläche sind folgende präzise
Ressourcentypen vorgesehen:

| Ressourcentyp | Grenze | Datenprofil |
|---|---|---|
| `core.identity.login-provider` | `installation` | `none`, `confidential` |
| `core.identity.directory-provider` | `installation` | `none`, `confidential` |
| `core.identity.directory-import` | `tenant` | `personal`, `restricted`, Processing erforderlich |

Die vorgesehenen Berechtigungen sind:

```text
core.identity.login-provider.read          admin / high
core.identity.login-provider.configure     admin / critical
core.identity.directory-provider.read      admin / high
core.identity.directory-provider.configure admin / critical
core.identity.directory-import.read        work / high
core.identity.directory-import.execute     service / high
```

Die bestehende Berechtigung `core.identity.provider.read` bleibt für die
vorhandene schreibgeschützte Übersicht der Anmeldeprovider erhalten. Ihre Bedeutung wird
nicht auf Directory-, UCS- oder Geräte-Domain-Provider erweitert. Eine
technische Service-Capability aus der Provider-Registry ist niemals eine
RBAC-Berechtigung; jeder aufrufende Command benötigt weiterhin Actor,
passende Permission, Ressource, Tenant, Datenprofil und Processing-Policy.

OIDC- und SAML-Anmeldungen verwenden die bestehenden Security-Audit-Typen wie
`identity.login.succeeded.v1`, `identity.login.denied.v1` und
`identity.login.throttled.v1`. Auditdetails dürfen Methode und stabile
Provider-ID enthalten, aber keine Subjects, Claims, Assertions, Tokens,
Passwörter oder externen Fehlerrohtexte.

Ein reiner Directory-Lookup erzeugt kein Domain-Event. Sobald ein späterer,
persistenter und idempotenter Importlauf WERK-Zustand besitzt, verwendet er
tenantgebundene Ereignisse:

```text
core.identity.directory-import-started.v1
core.identity.directory-import-completed.v1
core.identity.directory-import-failed.v1
```

Die Payload enthält höchstens Run-ID, Provider-ID, Zählwerte, Ergebnisstatus
und einen Cursor-Digest. Für tatsächlich angelegte oder geänderte WERK-Konten
werden die bestehenden Account-Ereignisse wiederverwendet; es entsteht keine
zweite Directory-Account-Wahrheit. Installationsweite Provideränderungen
erzeugen zunächst Security-Audit. Ein installationsweiter Domain-Event wird
nicht vorgetäuscht, solange der vorhandene Eventvertrag einen Tenant verlangt.

## Sicherheitsinvarianten

1. Ein Providerergebnis bestimmt niemals Kontoart, Tenant, Audience, Rolle,
   Permission, Membership oder App-Entitlement.
2. Directory-Operationen tragen immer einen serverbestätigten Tenant-Kontext;
   ein fehlender Tenant bedeutet niemals „alle Unternehmen“.
3. Login-Federation v1 übernimmt keinen Tenant vom Browser oder Provider.
4. Provider, Capability, Binding und Konfiguration werden vor jeder
   sicherheitsrelevanten Nutzung frisch und fail-closed geprüft.
5. Deaktivierung von Provider, Subject-Bindung, Konto, Credential oder Tenant
   wirkt spätestens bei der nächsten Auflösung. Eine begonnene Zeremonie kann
   keinen ausgetauschten Nachweis übernehmen.
6. Protokoll- und Providerfehler werden nach außen vereinheitlicht. Weder
   Kontovorhandensein noch Binding-, Mapping- oder Konfigurationsdetails werden
   offengelegt.
7. Alle externen Antworten, Attribute, Metadaten und Fehlermengen besitzen
   Größen-, Zeit- und Parallelitätsgrenzen.
8. TLS-Vertrauen wird ausdrücklich konfiguriert und vollständig validiert;
   produktive Adapter besitzen keinen `insecure-skip-verify`-Pfad.
9. Providerkonfiguration und Secretmaterial werden weder in Logs, Events,
   allgemeinen Fehlern noch in der globalen Registry ausgegeben.
10. Ein physisches Produkt erhält nur die einzeln geprüften Capabilities seiner
    getrennten Providerregistrierungen.

## Bewusste Nicht-Ziele

- keine LDAP-Passwortanmeldung in diesem v1-Schnitt; der vorhandene
  Methodentyp `ldap-password` bleibt reserviert und benötigt einen eigenen
  Verifikations- und Throttlingvertrag,
- keine automatische Kontoanlage, Account-Verknüpfung oder Rollenvergabe aus
  OIDC-Claims, SAML-Attributen oder LDAP-Gruppen,
- kein Directory-Writeback, SCIM, Kennworthashimport oder automatische
  Löschung in v1,
- kein OIDC-/SAML-Single-Logout oder providerübergreifendes Sessionmanagement
  in v1,
- keine automatische Providerwahl, Priorität, Fallback, Lastverteilung oder
  Health-Promotion,
- keine allgemeine öffentliche Providerkonfigurations-API im ersten Slice,
- keine Speicherung freier Providerkonfiguration oder Secrets in PostgreSQL-
  Registrytabellen,
- kein Windows-Domänendienst, Domain Join, DNS-, Kerberos-, Netlogon-,
  Gruppenrichtlinien- oder Maschinenkontenvertrag,
- keine Behauptung, Nubus für Kubernetes sei durch LDAP/OIDC ein
  Active-Directory-Domain-Controller,
- kein Univention-, Active-Directory- oder anderer Produkt-Superprovider,
- keine Kopplung dieser Fähigkeiten an Docker, Kubernetes, Helm oder ein
  bestimmtes Deploymentprofil.

Ein späterer Geräte- und Netzwerkdomänendienst gehört nicht in Core Identity.
Er benötigt einen eigenen Owner und ein separates ADR, voraussichtlich unter
`core.fleet.service.device-domain`. Erst reale DNS-, Kerberos-,
Maschinenkonto- und Windows-Join-Tests gegen eine ausdrücklich unterstützte
UCS-/Samba-AD-Konfiguration dürfen eine Windows-Domain-Join-Capability
aktivieren.

## Phasenweise Umsetzung

### Phase A – Inaktive Vertragsbasis

- dieses ADR annehmen und die Begriffe in Produkttexten verwenden,
- Service-, Capability-, Ressourcen-, Datenprofil- und Permission-Verträge
  migrationsverwaltet registrieren,
- Provider und Bindings zunächst `disabled` halten,
- typisierte Ports, Konfigurationsgrenzen sowie Unit- und Vertragstests
  implementieren.

Diese Phase aktiviert keinen externen Anmelde- oder Directory-Weg.

### Phase B – Ein OIDC-End-to-End-Slice

- genau einen installationsweiten OIDC-Provider und Adapter anbinden,
- sichere Konfiguration und Materialreferenzen bereitstellen,
- Authorization Code, PKCE, State, Nonce, Tokenprüfung und exakte
  Subject-Bindung implementieren,
- positive und negative Integrationstests gegen das ausdrücklich unterstützte
  Providerprofil ausführen,
- Sessionausstellung, Throttling, Security-Audit und Deaktivierung fail-closed
  nachweisen.

Erst danach darf die konkrete Providerinstanz als „OIDC-Anmeldung verfügbar“
erscheinen.

### Phase C – Ein read-only Directory-Slice

- genau einen Directory-Adapter und einen expliziten Tenant anbinden,
- zunächst nur die tatsächlich benötigten Lese-Capabilities aktivieren,
- Mapping, Pagination, Limits, TLS, Deaktivierung und tenantfremde Zugriffe
  gegen eine Wegwerfinstanz testen,
- keine Accounts, Parteien, Memberships oder Rollen verändern.

Erst danach darf die konkrete Providerinstanz als „Verzeichniszugriff
verfügbar“ erscheinen.

### Phase D – Persistenter Directory-Import

- Importlauf, Cursor, Idempotenz und Reconciliation als Identity-eigene
  Zustände modellieren,
- explizite serverseitige Zuordnungsregeln zu Tenant, Konto und Party
  festlegen,
- Permission, Processing-Policy, Audit und Outbox atomar integrieren,
- Konflikte, Deaktivierung, Wiederanlauf und Löschgrenzen gesondert testen.

Diese Phase ist Provisionierung und darf nicht rückwirkend als Bestandteil von
Directory v1 behauptet werden.

### Phase E – SAML und weitere Verträge

- SAML als getrennten, vollständig getesteten Login-Adapter ergänzen,
- tenantlokale Login-Provider, LDAP-Passwortprüfung, SCIM, Writeback und
  Single-Logout jeweils nur nach eigener Vertrags- und Bedrohungsanalyse
  ergänzen,
- einen Geräte- und Netzwerkdomänendienst ausschließlich in einem eigenen,
  späteren ADR behandeln.

## Folgen

WERK kann externe Produkte schrittweise anbinden, ohne einen Produktnamen oder
ein gesprochenes Protokoll als Beweis für nicht implementierte Fähigkeiten zu
verwenden. Die getrennten Registrierungen und Konfigurationseigner erhöhen den
Modellierungs- und Testaufwand. Dafür bleiben Identity-Hoheit,
Tenant-Isolation, Providerwechsel, Audit und spätere Geräteintegration klar
prüfbar, und die Produktoberfläche kann zwischen „konfiguriert“, „gebunden“,
„getestet“ und tatsächlich „verfügbar“ unterscheiden.
