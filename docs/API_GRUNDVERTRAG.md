# WERK – API-Grundvertrag

**Status:** Verbindliche Grundlage für den Plattformkern  
**Maschinenlesbarer Vertrag:** [`api/openapi.json`](../api/openapi.json)

## Grenzen

WERK verwendet nicht einen gemeinsamen API-Bereich für alle Kontoarten:

| Bereich | Präfix | Audience | Regel |
|---|---|---|---|
| Arbeit | `/api/v1` | `work` | Nur Arbeitskonten mit Tenant-Kontext |
| Administration | `/admin/v1` | `admin` | Nur interaktive Administrationskonten ohne Tenant und mit bekannter Single- oder Multi-Factor-Assurance; keine Workspace-Nutzung |
| Dienste | `/service/v1` | `service` | Nur technische Service- oder tenantgebundene Agent-Identitäten |

Gemeinsame Betriebsendpunkte wie `/health/live`, `/health/ready` und `/meta`
sind keinem dieser Kontobereiche zugeordnet. `/metrics` ist nur über einen
internen Operator-Listener erreichbar. Bis die jeweilige Authentifizierungs-, Audit- und
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

## Admin-Session-Assurance

Eine Admin-Session muss zur Kontoart `admin` und zur Admin-Audience gehören,
interaktiv sein und darf keinen Tenant tragen. `single-factor` und
`multi-factor` sind bekannte, zulässige Assurance-Stufen; `unknown` wird
fail-closed abgewiesen. MFA ist eine selbst gestartete, nicht blockierende
Empfehlung und kein globales Gate für `/admin/v1`.

Jeder Endpunkt prüft daneben unverändert seine registrierte Berechtigung,
Ressourcenbindung, sein Datenprofil und seine Processing-Policy. Tenantgebundene
Befehle verlangen den expliziten Tenant; Cookie-Mutationen verlangen CSRF-
Schutz, und verbindliche Mutationen schreiben Audit und Outbox atomar. Eine
künftige zusätzliche Re-Authentifizierung gilt nur, wenn sie für eine besonders
sensible Aktion ausdrücklich aktions- und ressourcengebunden versioniert wurde.
Sie darf nicht aus Kontoart oder Risikoklassifikation implizit abgeleitet
werden. Siehe
[`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md).

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
- Kann eine Organisationseinheit wegen aktiver oder geplanter Organisations-,
  Zugriffsgruppen-, Add-on- oder Rollenverknüpfungen nicht archiviert werden,
  antwortet WERK mit `409 Conflict` und
  `organizational-unit-referenced`.
- Würde das Umhängen einer Organisationseinheit die Reichweite einer aktiven
  oder geplanten `include_descendants`-Kante verändern, antwortet WERK mit
  `409 Conflict` und `organizational-unit-inherited-access-conflict`.
- Würde Create, Umhängen oder Reaktivieren die gemeinsame maximale
  Organisationstiefe von 64 Ebenen überschreiten, antwortet WERK mit
  `409 Conflict` und `organizational-unit-depth-limit-exceeded`.
- Eine erfolgreiche Änderung erhöht `version` atomar und liefert den neuen
  starken `ETag` in der Antwort.

Der Vertrag gilt derzeit für Mandanten, Organisationseinheiten und frei
verwaltbare Work-Rollen. Systemrollen sind von diesem Änderungsvertrag
ausgeschlossen.

`PUT /admin/v1/work-users/{accountId}/roles` ist davon getrennt ein
Mengenersetzungsvertrag ausschließlich für aktive Work-Zuweisungen mit
`scope_type='tenant'`. Organisations- und Ressourcen-Scopes werden in seiner
Leseprojektion nicht geliefert und durch das `PUT` weder beendet noch auf den
gesamten Tenant erweitert.

## Sicherheits-Audit lesen

`GET /admin/v1/security-audit` ist ein installationsweiter Admin-Vertrag. Er
erfordert eine gültige Admin-Session und die High-Risk-Berechtigung
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
- `werk_kafka_runtime_logs_dropped_total` zählt lokal nicht nach Kafka
  exportierte Betriebslogs ohne zusätzliche Labels. Der Zähler ist kein
  Auditnachweis; autoritative Audit- und Outbox-Rückstände stammen aus
  PostgreSQL.
- Zugriffslogs enthalten Methode, normalisierte Route, Status, Antwortgröße,
  Dauer und die beiden Request-IDs, aber weder Querystring noch Body oder Token.
