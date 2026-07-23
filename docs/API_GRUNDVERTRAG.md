# WERK – API-Grundvertrag

**Status:** Verbindliche Grundlage für den Plattformkern  
**Maschinenlesbarer Vertrag:** [`api/openapi.json`](../api/openapi.json)

## Grenzen

WERK verwendet nicht einen gemeinsamen API-Bereich für alle Kontoarten:

| Bereich | Präfix | Audience | Regel |
|---|---|---|---|
| Arbeit | `/api/v1` | `work` | Nur Arbeitskonten mit Tenant-Kontext |
| Administration | `/admin/v1` | `admin` | Nur Administrationskonten; keine Workspace-Nutzung |
| Dienste | `/service/v1` | `service` | Nur technische Service- oder tenantgebundene Agent-Identitäten |

Gemeinsame Betriebsendpunkte wie `/health/live`, `/health/ready` und `/meta`
sind keinem dieser Kontobereiche zugeordnet. `/metrics` ist nur im internen
Containernetz erreichbar. Bis die jeweilige Authentifizierungs-, Audit- und
Policy-Kette vorhanden ist, werden unter den drei Präfixen keine schreibenden
Handler veröffentlicht.

## Clientarten und Sitzungsbindung

Die Weboberfläche sowie installierbare Kotlin-/Compose-Multiplatform-Clients
verwenden dieselben fachlichen API-Präfixe, Problemtypen, Berechtigungen und
Tenant-Grenzen. Ein nativer Client erhält weder eine privilegierte Parallel-API
noch einen direkten internen Datenzugriff.

Der Browser verwendet das dokumentierte Cookie-, Origin-, Fetch-Metadata- und
CSRF-Schutzmodell. Dieses Modell wird nicht unverändert auf einen nativen Client
übertragen. Vor der ersten nativen Anmeldung wird der Sitzungs- und
Redirectvertrag in einem eigenen Authentifizierungs-ADR mit Bedrohungsmodell
festgelegt. Bis dahin ist kein nativer Loginvertrag Teil der öffentlichen API.
Die bereits verbindlichen Clientgrenzen stehen in
[`ADR-013`](adr/ADR-013-native-clients-kotlin-compose-multiplatform.md) und der
[`Clientarchitektur`](CLIENT-ARCHITEKTUR.md).

## Request-Identität

- WERK erzeugt für jeden HTTP-Versuch eine neue UUIDv7 als `X-Request-ID`.
- Eine gültige eingehende UUID in `X-Correlation-ID` wird übernommen; andernfalls
  erzeugt WERK eine UUIDv7.
- Eine ungültige oder mehrfach übermittelte Correlation-ID wird mit `400`
  abgelehnt.
- Beide IDs stehen in Antwortheadern, strukturierten Logs und Problem Details.
- IDs dienen der Nachverfolgung und sind niemals Berechtigungsnachweise.

## Fehler

HTTP-Fehler verwenden `application/problem+json` mit stabilen `type`- und
`code`-Werten. Beispiel:

```json
{
  "type": "urn:werk:problem:not-found",
  "title": "Resource not found",
  "status": 404,
  "detail": "The requested resource does not exist.",
  "instance": "urn:werk:request:019f...",
  "code": "not-found",
  "request_id": "019f...",
  "correlation_id": "019f..."
}
```

Antworten enthalten keine SQL-Fehler, Stacktraces, Secrets, Tokens oder internen
Verbindungsdetails. Validierungsfehler werden später über JSON Pointer einzelnen
Feldern zugeordnet, ohne den ungültigen Rohwert zurückzugeben.

## Optimistische Nebenläufigkeit

Änderungsverträge für bestehende, versionierte Ressourcen verwenden einen
starken HTTP-Entity-Tag. Der Client übernimmt die positive `version` aus der
zuletzt gelesenen Repräsentation und sendet sie beim vollständigen `PUT` als
`If-Match: "<version>"`.

- Ein fehlender `If-Match`-Header ergibt `428 Precondition Required` mit dem
  Problemcode `version-required`.
- Ein syntaktisch ungültiger, schwacher oder mehrfacher Entity-Tag ergibt
  `400 Bad Request` mit `invalid-version`.
- Wurde die Ressource zwischenzeitlich geändert, antwortet WERK mit
  `412 Precondition Failed` und `version-conflict`.
- Eine erfolgreiche Änderung erhöht `version` atomar und liefert den neuen
  starken `ETag` in der Antwort.

Der Vertrag gilt derzeit für Mandanten, Organisationseinheiten und frei
verwaltbare Work-Rollen. Systemrollen sind von diesem Änderungsvertrag
ausgeschlossen.

## Sicherheits-Audit lesen

`GET /admin/v1/security-audit` ist ein installationsweiter Admin-Vertrag. Er
erfordert eine MFA-bestätigte Admin-Session und die High-Risk-Berechtigung
`core.audit.security-event.read`. Tenant, exakter versionierter Ereignistyp und
Ergebnis können optional gefiltert werden. Die absteigende Timeline verwendet
einen opaken Cursor und liefert höchstens 100 Ereignisse je Seite.

Die Antwort enthält nur Zeitpunkt, Typ, Ergebnis, Actor-Konto samt Kontoart,
optionalen Tenant-Kontext sowie Request- und Correlation-ID. Die Kontoart wird
über eine eng begrenzte Projektion aufgelöst, ohne den globalen Kontobestand für
die Admin-Runtime freizugeben. Freie `details`-JSON-Daten und
Session-IDs bleiben serverintern. Antworten tragen `Cache-Control: no-store`.
Der Abruf und sein eigenes globales Security-Audit-Ereignis
`core.audit.security-events-listed.v1` werden atomar in einer eng begrenzten
Installationstransaktion gespeichert; RLS erlaubt dort keine andere globale
Audit-Mutation.

## Betriebsverhalten

- `/health/live` prüft nur, ob der HTTP-Prozess antworten kann.
- `/health/ready` prüft zwingende Abhängigkeiten. Valkey blockiert die
  Bereitschaft nicht, solange es ausschließlich austauschbare Infrastruktur ist.
- `/meta` liefert nur Produkt, Dienst und Build-Version.
- `/metrics` verwendet ausschließlich begrenzte Labels; Tenant-, Konto-,
  Request- oder freie Pfadwerte sind verboten.
- Zugriffslogs enthalten Methode, normalisierte Route, Status, Antwortgröße,
  Dauer und die beiden Request-IDs, aber weder Querystring noch Body oder Token.
