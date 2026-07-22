# ADR-018 – Organisationskoordinaten und explizite App-Entitlements

**Status:** Angenommen  
**Datum:** 2026-07-21

## Kontext

Die Plattform soll der zentrale organisatorische Bezugspunkt eines Unternehmens
sein, ohne Fachapps in einen zentralen Policy-Monolithen zu zwingen. Personen
sollen für eine Fachapp direkt oder über Abteilung und Gruppe freigeschaltet
werden können. Gleichzeitig benötigen Bereiche, Abteilungen und Teams eigene
innere Regelungsräume, ohne dafür künstlich neue Tenants anzulegen.

Ein reines Rollenmodell reicht dafür nicht aus: Eine Rolle beschreibt, was ein
Akteur innerhalb einer App tun darf, beantwortet aber nicht, ob die App in
seinem Tenant installiert ist und ob er sie überhaupt betreten darf. Eine
einzige generische Gruppenhierarchie würde dagegen Organisation, Zugriff und
Delegation vermischen und schnell zyklisch oder unbedienbar werden.

## Entscheidung

### Tenant bleibt die harte Grenze

Ein `Tenant` bleibt die oberste Daten- und Sicherheitsgrenze. Gesellschaften,
Standorte, Bereiche, Abteilungen und Teams werden innerhalb eines Tenants als
hierarchische `OrganizationalUnit`s modelliert:

```text
Tenant
  -> Bereich
     -> Abteilung
        -> Team
```

Diese Schalen dürfen lokale Zuständigkeiten und später delegierte Verwaltung
tragen. Eine Abteilung wird dadurch nicht zu einem eigenen Tenant. Ein eigener
Tenant ist nur für eine tatsächlich getrennte Daten-, Rechts- oder
Administrationsgrenze vorgesehen.

### Access-Gruppen bilden querliegende Edges

Eine `AccessGroup` ist eine tenantgebundene, fachappübergreifend nutzbare
Subjektgruppe. Sie kann direkte Work-Konten und Organisationseinheiten
enthalten. Bei einer Organisationseinheit wird ausdrücklich festgelegt, ob nur
die Einheit oder auch ihre Nachfahren einbezogen werden.

`governing_unit_id` bezeichnet die Organisationseinheit, in deren delegiertem
Verwaltungsbereich die Gruppe liegt. Sie ändert weder Tenant noch Datenhoheit.
Verschachtelte Access-Gruppen sind im ersten Vertrag nicht erlaubt. Dadurch
bleiben Auflösung und Oberfläche verständlich und Gruppenzyklen technisch
ausgeschlossen. Abteilungsübergreifende Gruppen entstehen durch mehrere direkte
Kanten, nicht durch eine zweite Organisationshierarchie.

### App-Installation und App-Freischaltung sind getrennt

Eine Plattformregistrierung mit Namensraum `app.*` kann tenantbezogen aktiviert
werden. Eine aktive `TenantAppInstallation` macht die App im Tenant verfügbar,
erteilt aber noch keinem Benutzer Zugriff.

Ein `AppEntitlement` öffnet die App-Tür ausdrücklich für genau eines der
folgenden Ziele:

- ein tenantgebundenes Work-Konto,
- eine Organisationseinheit, optional einschließlich Nachfahren,
- eine Access-Gruppe.

Fehlt ein aktives und zeitlich gültiges Entitlement, bleibt die App für den
Akteur geschlossen. Direkte Kontofreischaltungen sind möglich, bleiben aber die
Ausnahme gegenüber Abteilungs- und Gruppenzuweisungen. Admin-, Service- und
Agent-Konten sind keine menschlichen App-Entitlement-Subjekte.

### Entscheidungsfolge

Für eine spätere Fachapp gilt:

```text
serverseitig aufgelöster Actor und Tenant
  -> aktive App-Registrierung und Tenant-Installation
  -> aktive Organisations- und Gruppenkoordinaten
  -> passendes App-Entitlement
  -> Rolle, Permission, Ressource und Processing-Policy
  -> zusätzliche Fachregel der Owner-App
  -> Aktion, Audit und gegebenenfalls Ereignis
```

Ein Entitlement ist ausschließlich ein Verfügbarkeits-Gate. Es gewährt keine
Rolle, Permission oder Fachressource. Eine Fachapp darf eine Plattformablehnung
niemals in `allow` umwandeln; sie darf innerhalb des freigegebenen Rahmens nur
weitere Bedingungen verlangen.

### Speicher- und Laufzeitvertrag

PostgreSQL hält `tenant_app_installations`, `access_groups`,
`access_group_memberships` und `app_entitlements` als tenantgebundene Wahrheit.
Zusammengesetzte Fremdschlüssel verhindern Cross-Tenant-Kanten. RLS, getrennte
Runtime-Rollen und serverseitige Trigger begrenzen Verwaltung und direkte
Kontofreischaltung zusätzlich.

Der pure Go-Vertrag wertet eine kleine, serverseitig ermittelte
`ActorCoordinates`-Projektion aus: Konto, direkte Organisationseinheiten, deren
Vorfahren und aufgelöste Access-Gruppen. Der Client darf diese Koordinaten nicht
liefern. Die Laufzeit prüft nur relevante Kanten und berechnet keine vollständige
Unternehmensmatrix je Request.

## Bewusst noch nicht enthalten

- Verwaltungs-API und Administrationsoberfläche,
- dynamische Installation oder Manifestprüfung einer realen Fachapp,
- delegierte Administratorrollen für `governing_unit_id`,
- Kopplung des App-Gates an einen ersten produktiven Fachapp-Endpunkt,
- frei definierbare Gruppenverschachtelung,
- individuelle Policy-Sprache oder persistierte Matrixzellen,
- automatische Rollenzuweisung durch ein App-Entitlement.

Diese Funktionen bauen auf dem Vertrag auf und werden erst mit einem konkreten
Verbraucher ergänzt.

## Folgen

- Die Plattform wird zum gemeinsamen Organisations- und Zugriffspunkt, ohne
  Fachlogik der Apps zu übernehmen.
- Abteilungen können eigene innere Zuständigkeiten erhalten, bleiben aber in
  derselben Tenant-Grenze.
- App-Sichtbarkeit, Rollen und Fachberechtigungen bleiben getrennte und
  gemeinsam notwendige Prüfschichten.
- Direkte Benutzerfreischaltungen sind möglich, ohne Gruppen als künstliche
  Ein-Personen-Container verwenden zu müssen.
- Das erste Modell bleibt absichtlich ein gerichteter, zyklusfreier Graph statt
  einer universellen Policy- oder Gruppenengine.

## Änderbarkeit

Dies ist der angenommene Planungs- und Implementierungsstand. Verwaltungs-API,
Delegationsregeln, Auflösungsprojektionen, App-Manifeste und UI-Darstellung
werden mit dem ersten Fachapp-Verbraucher versioniert konkretisiert; Änderungen
vorbehalten. Unverändert bleiben die harte Tenant-Grenze, die explizite
App-Freischaltung, die Trennung von Entitlement und Rolle sowie das fail-closed
Verhalten.
