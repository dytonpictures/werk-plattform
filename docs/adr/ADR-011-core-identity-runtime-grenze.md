# ADR-011 – Core-Identity-Runtime und neutrale Login-Grenze

**Status:** Angenommen  
**Datum:** 2026-07-20

## Kontext

Der gemeinsame interaktive Login muss ein Konto anhand einer eindeutigen
Login-Kennung prüfen und erst danach dessen feste Kontoart, Audience und
Tenant-Zuordnung bestimmen. Zu diesem Zeitpunkt existiert noch kein
vertrauenswürdiger Tenant-Kontext. Die vorhandenen Rollen
`werk_work_runtime`, `werk_admin_runtime` und `werk_service_runtime` sind an
ihre jeweiligen API-Bereiche gebunden und dürfen für diese kontoartübergreifende
Auflösung nicht wiederverwendet werden.

## Entscheidung

- Core Identity erhält die eigene Non-Owner-Rolle `werk_identity_runtime`.
- Nur der neutrale Authentifizierungsadapter verwendet diese Rolle.
- Die Rolle erhält ausschließlich die für Bootstrap, Credential-Prüfung,
  Session-Ausstellung, Session-Auflösung, Logout und Passwortwechsel nötigen
  Tabellenrechte und RLS-Policies.
- Sie erhält keine Rechte auf Fach-, Organisations-, Admin- oder
  Service-Operationen und darf weder Superuser-, `BYPASSRLS`- noch
  Owner-Mitgliedschaft besitzen.
- Der Login nimmt keine Kontoart, Audience oder Tenant-ID vom Client an. Diese
  Werte werden ausschließlich aus dem verifizierten Konto gelesen.
- Nach der Anmeldung wird jede Session an genau eine serverseitig bestimmte
  Audience gebunden. Bereichsmiddleware prüft Kontoart und Audience erneut.
- Interaktive Admin-Sessions benötigen weiterhin MFA. Ein Bootstrap-Admin darf
  vor eingerichteter MFA ausschließlich den eng begrenzten Bootstrap- und
  Passwortwechselablauf verwenden, nicht die reguläre Admin-API.
- Credential- und Sessiongeheimnisse werden nur gehasht gespeichert. Antworten
  und Logs verraten weder Kontovorhandensein noch Kontoart.

## Folgen

Die neutrale Login-Grenze kann alle interaktiven Kontoarten sicher auflösen,
ohne dem Work- oder Admin-API-Prozess fremde Datenbankrechte zu geben. Dafür
entstehen eine zusätzliche Datenbankrolle, eine getrennte
Verbindungskonfiguration und eigene Integrationstests für Grants, RLS,
Session-Audience und kontoartübergreifende Zugriffsversuche.

Externe Identity-Provider verwenden später dieselbe Core-Identity-Runtime erst
nach erfolgreicher Provider-Prüfung; sie dürfen Kontoart, Audience und Tenant
nicht selbst festlegen.
