# ADR-003 – API- und Kontoartgrenzen

**Status:** Angenommen  
**Datum:** 2026-07-19

## Kontext

WERK trennt Arbeitskonten, Administrationskonten und Service-Identitäten hart.
Ein gemeinsamer Router mit einer Rolle `admin=true` würde diese Trennung
verwischen und könnte Admin-Sitzungen unbeabsichtigt für Fachaktionen nutzbar
machen. Gleichzeitig benötigen Healthchecks und Build-Metadaten keine Kontoart.

## Entscheidung

- Arbeitsoperationen werden ausschließlich unter `/api/v1` veröffentlicht.
- Administrationsoperationen werden ausschließlich unter `/admin/v1`
  veröffentlicht.
- Service-Operationen werden ausschließlich unter `/service/v1` veröffentlicht.
- Interaktive Arbeits- und Administrationskonten betreten WERK über dieselbe
  neutrale öffentliche Login-Oberfläche. Die Oberfläche enthält keine
  Kontoart-, Rollen- oder Bereichsauswahl und verrät keinen gesonderten
  Admin-Anmeldepfad.
- Erst nach erfolgreicher Identitätsprüfung ermittelt der Server die fest am
  eindeutigen Konto gespeicherte Kontoart. Er erzeugt daraufhin ausschließlich
  die passende Session und leitet in den zugehörigen Oberflächenbereich weiter.
- Besitzt dieselbe Person ein Arbeits- und ein Administrationskonto, bleiben die
  Login-Kennungen eindeutig getrennt. Ein Konto kann niemals zwischen den
  Bereichen umgeschaltet werden.
- Service-Identitäten sind von der interaktiven Login-Oberfläche ausgeschlossen.
- Jeder Bereich erhält eine eigene Authentifizierungs- und Middlewarekette,
  Token-Audience, Sessionart und Berechtigungsdomäne.
- Eine Identität der falschen Kontoart wird nicht durch zusätzliche Rollen
  aufgewertet, sondern an der Bereichsgrenze abgelehnt.
- Fachneutrale Betriebsendpunkte liegen außerhalb dieser Präfixe.
- OpenAPI-Verträge und erzeugte Clients werden je Bereich getrennt, sobald die
  ersten authentifizierten Handler entstehen.
- Vor vorhandener Authentifizierung, Autorisierung, Auditierung und Tenant-RLS
  werden keine schreibenden Work-, Admin- oder Service-Handler exponiert.

## Folgen

Gemeinsame Implementierungsbausteine wie Signaturprüfung oder Fehlerformat dürfen
intern geteilt werden. Die Entscheidung, welche Kontoart einen Endpunkt betreten
darf, bleibt dennoch pro API-Bereich explizit. Neutrale Metadaten liegen daher
unter `/meta` und nicht unter `/api/v1/meta`.

Der gemeinsame Login-Einstieg reduziert sichtbare Angriffs- und
Enumerationshinweise, ist aber keine Sicherheitsgrenze. Selbst die Kenntnis eines
internen Admin-Pfads darf ohne passende Admin-Session, Audience, MFA und
Autorisierung keinerlei Zugriff ermöglichen. Fehlermeldungen der Anmeldung dürfen
außerdem weder das Vorhandensein eines Kontos noch dessen Kontoart offenlegen.
