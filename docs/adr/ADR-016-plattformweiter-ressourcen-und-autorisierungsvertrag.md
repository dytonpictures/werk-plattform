# ADR-016 – Plattformweiter Ressourcen- und Autorisierungsvertrag

**Status:** Angenommen  
**Datum:** 2026-07-21

## Kontext

Rollen und Berechtigungen beantworten allein noch nicht, welches konkrete Ziel
ein Akteur ansprechen darf. Eine Berechtigung darf nicht wie ein Generalschlüssel
für beliebige Objekte mit ähnlich klingender Aktion wirken. Gleichzeitig sollen
Core-Module und spätere Apps ihre eigenen Ressourcentypen besitzen, ohne eine
zweite Autorisierungslogik aufzubauen.

Die Plattform benötigt deshalb oberhalb einzelner Apps einen gemeinsamen
Schließsystem-Vertrag: Die Plattform registriert Schlösser und zulässige
Schlüsselarten, der serverseitig aufgelöste Akteur bringt seinen unveränderlichen
Sicherheitskontext mit, und erst danach darf das besitzende Modul seine
fachlichen Bedingungen prüfen und die Aktion ausführen.

## Entscheidung

### Plattformweite Registrierungen

PostgreSQL hält drei installationsweite Register als fachliche Wahrheit:

1. `platform_modules` reserviert einen Namensraum als `core`- oder `app`-Modul.
2. `resource_type_registrations` ordnet einen Ressourcentyp exakt einem
   Owner-Modul und einer Grenze `installation` oder `tenant` zu.
3. `permission_resource_types` legt explizit fest, welche registrierten
   Ressourcentypen eine Berechtigung adressieren darf.

Die bestehenden Berechtigungen erhalten zusätzlich eine positive
`contract_version`. Fehlt Modul, Ressourcentyp oder Zuordnung, wird der Zugriff
geschlossen abgelehnt. Das Deaktivieren eines Moduls oder Ressourcentyps nimmt
ihn ebenfalls aus der Laufzeitentscheidung. Im ersten Vertrag schreibt nur der
Migrations-/Owner-Pfad Registrierungen; ein dynamisches App-Installations-API ist
noch nicht freigegeben.

Namensräume verwenden `core.<modul>` oder `app.<modul>`. Ein Ressourcentyp muss
unterhalb des Namensraums seines Owners liegen. Dadurch kann ein Modul keine
Ressourcenart eines anderen Moduls registrieren.

### Serverbestimmter Plattformkontext

Aus dem authentifizierten Actor wird serverseitig ein `PlatformContext`
abgeleitet:

```text
PlatformContext
  actor_id
  account_class
  access_plane
  tenant_id?
```

Ein Request darf diesen Kontext weder auswählen noch erweitern. `admin` besitzt
keinen Tenant im Plattformkontext und darf ausschließlich installationsgebundene
Verwaltungsressourcen adressieren. `work` und tenantgebundene `agent`-Principals
dürfen ausschließlich Ressourcen ihres eigenen Tenants adressieren. Ein
`service`-Principal kann installations- oder tenantgebunden sein, darf aber nur
die dazu passende Grenze und ausschließlich Service-Berechtigungen verwenden.
Der heutige Rollenvertrag stellt noch keine installationsweite Service-Rolle
bereit; diese Möglichkeit ist lediglich sauber begrenzt vorbereitet.

Eine spätere stabile `realm_id`/`instance_id` für das Mehrinstanzprofil ergänzt
diesen Kontext gemäß
[`ADR-015`](ADR-015-identity-authority-witness-und-failover.md). Sie wird nicht
vorzeitig durch eine lokale Konstante vorgetäuscht.

### Kanonische Ressourcenreferenz

Jede Autorisierungsanfrage enthält eine validierte Referenz:

```text
ResourceRef
  boundary: installation | tenant
  tenant_id?: UUID
  kind: registrierter Ressourcentyp
  id: stabile ID innerhalb des Ressourcentyps
```

`tenant_id` ist bei `tenant` zwingend und bei `installation` verboten. Ein
fehlender Tenant bedeutet niemals „alle Tenants“. Installationsweite Aktionen
verwenden einen ausdrücklich als `installation` registrierten Ressourcentyp.

Ein administrativ verwalteter Mandant oder ein Arbeitskonto ist in diesem
Vertrag eine installationsgebundene **Verwaltungsressource**. Das gibt dem
Admin keinen Zugriff auf die fachlichen Tenant-Daten. Die anschließende Mutation
verwendet weiterhin einen expliziten Tenant-Transaktionskontext und RLS. Eine
fachliche Workspace-, Dokument- oder Workflow-Ressource bleibt dagegen
tenantgebunden und für den Admin-Bereich nicht adressierbar.

### Entscheidungsreihenfolge

Die Autorisierung läuft in dieser Reihenfolge und standardmäßig ablehnend:

1. Actor, Kontoart, Audience und Access Plane validieren.
2. `PlatformContext` serverseitig ableiten.
3. `ResourceRef`, Grenze und Actor-Tenant prüfen.
4. Aktives Modul, aktiven Ressourcentyp und explizite
   Berechtigung-Ressourcentyp-Zuordnung aus PostgreSQL bestätigen.
5. Aktives Ressourcendatenprofil und die zur Berechtigung-/Ressourcentyp-
   Kombination gehörende Processing-Policy bestätigen.
6. Datenklassifikation und Processing-Policy gemeinsam fail-closed prüfen.
7. Aktive, zeitlich gültige Rollen- und Scope-Zuweisung prüfen.
8. Erst bei `allow` die fachlichen Regeln des Owner-Moduls prüfen und handeln.

Der Core liefert eine maschinenlesbare `Decision` mit `allow` oder `deny` und
einem internen Grund. Nach außen bleibt die Ablehnung absichtlich generisch.
Eine App darf zusätzliche Fachbedingungen verschärfen, aber keine Plattform-
Ablehnung in eine Freigabe umwandeln.

### Policy-Hierarchie

Der Registry-Vertrag ist die unterste stabile Basis für spätere Policies:

```text
Plattformgrenze
  -> Tenant-Policy
     -> App-Policy
        -> Ressourcen-/Aktionsbedingung
```

Kindebenen dürfen Elternregeln nur einschränken. Ein ausdrückliches `deny` hat
Vorrang. Frei konfigurierbare Policy-Dokumente, Facts, JIT-Grants und Approval-
Checkpoints werden auf dieser Basis versioniert ergänzt; sie sind nicht Teil
dieses ersten Implementierungsschnitts.

## Folgen

- Eine Berechtigung ist nicht mehr ohne definierten Ressourcentyp nutzbar.
- Eine Rolle kann ein fehlendes Datenprofil oder eine fehlende beziehungsweise
  unpassende Processing-Policy nicht überstimmen.
- Core und Apps teilen eine Autorisierungsinstanz, behalten aber ihre
  Datenhoheit und fachlichen Regeln.
- Work-/Service-Ressourcen und administrative Kontrollobjekte können nicht durch
  einen fehlenden Tenant vermischt werden.
- Neue Module müssen Namespace, Ressourcentypen und Berechtigungszuordnungen
  migrations- beziehungsweise später manifestgestützt registrieren.
- Die Basis registriert noch keine einzelnen Business-Objekte und enthält noch
  keine allgemeine Policy-Sprache oder App-Installation.

## Änderbarkeit

Dies ist der angenommene Planungs- und Implementierungsstand. Feldumfang,
Policy-Facts, App-Manifest und öffentliche Registrierungs-APIs können sich mit
den nächsten Phasen ändern. Unverändert bleiben die fail-closed Entscheidung,
explizite Grenzen, serverbestimmter Kontext und die Hoheit des Owner-Moduls.
