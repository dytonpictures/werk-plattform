# ADR-032 – Optionale Admin-MFA und aktionsgebundene Re-Authentifizierung

**Status:** Angenommen  
**Datum:** 2026-07-29

## Kontext

Die bisherige Zugriffsregel verlangte für jede Nutzung von `/admin/v1` eine
Multi-Faktor-bestätigte Admin-Sitzung. Ein neu eingerichtetes Admin-Konto ohne
aktiven Faktor erhielt dadurch zwar eine gültige Single-Factor-Sitzung, konnte
aber außer der MFA-Einrichtung keine Administrationsfunktion verwenden. Diese
pauschale Sperre erschwert den Einstieg und vermischt die unveränderliche
Kontoartgrenze mit der Stärke einer einzelnen Anmeldung.

Kontoart, Audience, Tenant-Grenze, serverseitige Berechtigungen,
Ressourcenscope, Processing-Policy, CSRF-Schutz und Audit sind bereits
eigenständige, weiterhin verbindliche Schutzschichten. MFA soll als bewusst
gewählte Verstärkung verfügbar bleiben, aber nicht länger den gesamten
bestehenden Admin-Bereich sperren.

## Entscheidung

- Eine gültige interaktive `admin`-Session mit `admin`-Audience und ohne
  Tenant-Kontext darf die Admin-Zugriffsebene mit `single-factor`- oder
  `multi-factor`-Assurance betreten. `unknown` bleibt fail-closed abgelehnt.
- Diese Freigabe ändert weder Rollen noch Berechtigungen. Jeder Admin-Endpunkt
  prüft weiterhin serverseitig seine registrierte Permission, den adressierten
  Ressourcenscope, Datenprofil und Processing-Policy. Tenantbezogene Befehle
  benötigen weiterhin einen expliziten Tenant-Kontext; Mutationen behalten
  CSRF-Schutz, Audit und atomare Outbox-Schreibvorgänge.
- `work`, `admin`, `service` und `agent` bleiben getrennte Kontoarten mit
  getrennten Audiences, Sessions und APIs. Insbesondere erhält ein Work-Konto
  durch diese Entscheidung keinen Zugang zu `/admin/v1`.
- Ein Admin ohne aktiven Faktor startet die MFA-Einrichtung weiterhin selbst.
  Die Admin-Oberfläche zeigt dafür eine nicht blockierende Empfehlung und lädt
  zugleich die normal autorisierten Administrationsfunktionen.
- Wer einen TOTP-Faktor aktiviert, entscheidet sich bewusst für den bestehenden
  zweiten Faktor bei späteren Passwortanmeldungen. Passkey-Anmeldungen und
  MFA-Sessionrotation bleiben unverändert verfügbar.
- Der bisherige Sessionhinweis `mfa_enrollment_required` wird als nicht mehr
  zutreffende Pflichtangabe abgekündigt. Ein additiver Hinweis
  `mfa_enrollment_recommended` beschreibt die neue, nicht blockierende
  Empfehlung.
- Besonders sensible, künftig eingeführte Aktionen dürfen einen eigenen,
  aktions- und ressourcengebundenen Re-Authentifizierungs-, JIT- oder
  Mehrpersonenvertrag verlangen. Eine solche Anforderung muss am konkreten
  Endpunkt versioniert werden und darf nicht wieder als implizites globales
  Admin-Gate entstehen.

## Folgen

Ein korrekt authentifiziertes Administrationskonto bleibt auch ohne bereits
eingerichteten zweiten Faktor arbeitsfähig. Die Oberfläche kann MFA erklären,
ohne das Konto in einen praktisch unbenutzbaren Zustand zu versetzen.

Damit sinkt die Mindest-Assurance für die heute vorhandenen
Administrationsfunktionen bewusst von Multi-Factor auf Single-Factor. Die
unabhängigen Kontoart-, Session-, Permission-, Tenant-, Policy-, CSRF- und
Audit-Grenzen bleiben erhalten. Betreiber und Administratoren können MFA
weiterhin aktivieren; nach einer bewussten Aktivierung bleibt der bestehende
zweite Faktor bei Passwortanmeldungen wirksam.

Dieser Entscheid ersetzt ausschließlich die pauschale MFA-Zugriffsvoraussetzung
aus [`ADR-003`](ADR-003-api-und-kontoartgrenzen.md),
[`ADR-011`](ADR-011-core-identity-runtime-grenze.md) und
[`ADR-030`](ADR-030-identitaets-onboarding-und-mfa-aktivierung.md). Deren
Kontoart-, Onboarding-, Credential-, Sessionrotations-, Einladungs- und
Auditgrenzen bleiben verbindlich.
