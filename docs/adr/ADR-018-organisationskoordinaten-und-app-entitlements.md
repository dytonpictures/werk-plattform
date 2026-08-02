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

Der pure Go-Vertrag besitzt mit `ResolveActorCoordinates` eine serverseitige
Auflösung für genau einen authentifizierten, tenantgebundenen `work`-Actor. Sie
wertet alle zum Prüfzeitpunkt aktiven Memberships aus, verfolgt für jede direkte
Organisationseinheit den vollständigen aktiven Elternpfad und löst daraus die
aktiven direkten sowie organisationsbezogenen Access-Gruppen auf. Strukturell
unvollständige referenzierte Pfade, Zyklen, tenantfremde oder anderweitig
ungültige Snapshots werden fail-closed abgelehnt; jede projizierte
`OrganizationalMembership` muss zusätzlich dieselbe `AccountID` wie Actor und
Snapshot tragen. Eine `GroupMembership` für ein anderes Konto bleibt ein
gültiger Datensatz, erzeugt für diesen Actor aber keine Koordinate. Das Ergebnis
ist dedupliziert, deterministisch sortiert, außerhalb des Core-Pakets nicht frei
konstruierbar und an Actor sowie Prüfzeitpunkt gebunden.

Der öffentliche `EvaluationRequest` akzeptiert keine bereits aufgelösten
Koordinaten. Er führt Resolver und Gate-Entscheidung für denselben Actor und
Prüfzeitpunkt in einem Aufruf aus. Dadurch kann kein separat für einen anderen
Actor oder Zeitpunkt aufgelöster Koordinatenwert untergeschoben werden. Vor dem
ersten Match wird der vollständige Entitlement-Snapshot
einschließlich Tenant-, App- und ID-Eindeutigkeit validiert; kontaminierte oder
reihenfolgeabhängige Teilmengen scheitern fail-closed.

Der Client darf weder Organisations- noch Gruppenkoordinaten liefern. Ein
vertrauenswürdiger Datenbank-Store, der den geprüften Snapshot aus PostgreSQL
lädt, ist noch nicht implementiert. Erst dieser Runtime-Pfad muss pro
Autorisierungsprüfung einen frischen Snapshot und eine server- beziehungsweise
datenbankeigene Prüfzeit erzeugen. Ein altes `EvaluationRequest` ist als reiner
Wert wiederverwendbar und darf deshalb niemals als gecachte Autoritätsaussage
behandelt werden. Damit ist die pure Auflösung vorhanden, aber noch keine
ausführbare Dienstkette bis zu API, UI oder Fachapp.

Bereits vorhandene Organisationsmutationen schützen diese Koordinaten auch bei
Strukturänderungen. Hierarchiemutationen werden pro Tenant serialisiert, bevor
die betroffenen Pfade und Kanten geordnet gesperrt werden. Ein schmaler
Security-Definer-Vertrag erkennt organisationsgebundene Work- und
Service-Rollenzuweisungen, ohne dem Admin-Reader Service-Zuweisungsdaten
offenzulegen. Das Archivieren einer Einheit wird mit HTTP 409 abgelehnt,
solange sie aktive Kinder oder aktive beziehungsweise geplante Memberships,
eine aktive über `governing_unit_id` gebundene Access-Gruppe oder aktive
beziehungsweise geplante organisationsbezogene `GroupMembership`-,
`AppEntitlement`- oder Organisationsrollenkanten trägt. Abgelaufene Kanten,
widerrufene `GroupMembership`s und `AppEntitlement`s sowie deaktivierte
`governing_unit_id`-gebundene Access-Gruppen gelten dabei nicht als
fortbestehende Bindung.

Beim Umhängen einer Einheit wird die symmetrische Differenz von altem und neuem
Vorfahrenpfad geprüft. Eine aktive oder geplante
`include_descendants`-Gruppenmitgliedschaft beziehungsweise ein entsprechendes
App-Entitlement auf einem geänderten Vorfahren blockiert die Mutation mit HTTP
409, weil sich sonst ihre Nachfahrenreichweite stillschweigend ändern würde.
Exakte Einheitskanten, abgelaufene oder widerrufene Kanten und weiterhin
gemeinsame Vorfahren blockieren das Umhängen nicht. Eine aktive Kante bleibt
dagegen eine durable Referenz, wenn nur ihre umgebende App, Gruppe oder Rolle
deaktiviert wird. Geordnete Zeilensperren auf
Pfaden und betroffenen Kanten koordinieren die bereits vorhandenen
Mutationspfade. Neue Writer für App-Entitlements, Gruppen oder
Organisationsrollen müssen verbindlich dieselbe
Tenant-vor-Pfad-vor-Kante-Sperrreihenfolge
verwenden; erst dann ist auch ihre parallele Aktivierung gegen eine
Strukturänderung geschlossen.

Die maximale Organisationstiefe beträgt einschließlich direkter Einheit und
Wurzel 64 Ebenen. Resolver sowie Create-, Reparenting- und
Reaktivierungsvertrag verwenden dieselbe Core-Konstante. Eine Mutation, deren
gesamter verschobener Teilbaum die Grenze überschreiten würde, wird mit HTTP
409 und `organizational-unit-depth-limit-exceeded` abgelehnt.

## Bewusst noch nicht enthalten

- Verwaltungs-API und Administrationsoberfläche für Access-Gruppen,
  App-Installationen und App-Entitlements,
- PostgreSQL-Store und Runtime-Adapter für den vertrauenswürdigen
  Organisations- und Gruppen-Snapshot,
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

Dies ist der angenommene Planungs- und Implementierungsstand. Persistente
Auflösungsadapter, Verwaltungs-API, Delegationsregeln, App-Manifeste und
UI-Darstellung werden mit dem ersten Fachapp-Verbraucher versioniert
konkretisiert; Änderungen vorbehalten. Unverändert bleiben die harte
Tenant-Grenze, die explizite App-Freischaltung, die Trennung von Entitlement und
Rolle sowie das fail-closed Verhalten.
