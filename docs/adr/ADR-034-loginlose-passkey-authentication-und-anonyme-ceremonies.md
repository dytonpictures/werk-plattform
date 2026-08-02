# ADR-034 – Loginlose Passkey-Authentifizierung und anonyme Ceremonies

**Status:** Angenommen  
**Datum:** 2026-08-01

## Kontext

WERK registriert Passkeys als discoverable Credentials mit verpflichtender
Benutzerverifikation. Der bisherige Login startete trotzdem mit einem
Anmeldenamen, lud das Konto und gab nur für aktive Konten mit Passkey eine
WebAuthn-Challenge aus. Dadurch konnten Kontovorhandensein und Passkey-Status
beobachtet werden. Jeder erfolgreiche Start schrieb außerdem eine
kontogebundene MFA-Challenge und ein dauerhaftes Audit-Ereignis.

`identity_mfa_challenges` besitzt bewusst einen Account, eine
Sessiongeneration und gegebenenfalls ein Credential. Eine anonyme
Authentifizierungszeremonie hat vor dem signierten WebAuthn-Nachweis keinen
vertrauenswürdigen Accountkontext und darf diese Invarianten nicht verwässern.

## Entscheidung

- Der öffentliche Passkey-Login ist loginlos. Sein Start nimmt keinen
  Anmeldenamen, Tenant, keine Kontoart und keine Audience entgegen.
- Core Identity verwendet eine discoverable WebAuthn-Zeremonie ohne
  `allowCredentials`. Browser beziehungsweise Betriebssystem wählen einen für
  die RP-ID verfügbaren Passkey.
- Erst die vollständig geprüfte Kombination aus Credential-ID und opakem
  WebAuthn-User-Handle wird serverseitig zu einem aktiven WERK-Konto aufgelöst.
  Anschließend werden Provider-Binding, Konto- und Tenantstatus,
  Sessiongeneration, Kontoart und Audience erneut geprüft.
- Kurzlebige anonyme Zustände liegen getrennt in
  `identity_authentication_ceremonies`. Browser erhalten nur ein zufälliges
  Roh-Token; PostgreSQL speichert dessen SHA-256-Digest. WebAuthn-Sessiondaten
  werden mit dem Identity-MFA-Schlüsselring und ceremonygebundener AAD
  verschlüsselt.
- Der Abschluss sperrt und konsumiert die Ceremony atomar mit Credentialupdate,
  Sessionausstellung und Erfolgs-Audit. Fehlgeschlagene kryptografische
  Prüfungen verbrauchen die Ceremony ebenfalls, damit ein öffentlicher Zustand
  nicht als unbegrenztes Verifikationsoracle dient.
- Der Start erzeugt kein dauerhaftes Security-Audit. Er wird durch einen
  begrenzten direkten Peer-Limiter geschützt. Die Verifikation besitzt
  zusätzlich eine prozessweite Parallelitätsgrenze.
- Beim Start werden abgelaufene oder verbrauchte Ceremonies in begrenzten
  Batches bereinigt. Eine installationsweite Obergrenze offener Ceremonies
  verhindert unbegrenztes Wachstum auch über mehrere API-Prozesse.

## Folgen

Die Passkey-Optionsantwort ist nicht mehr accountabhängig. WERK übernimmt die
bei Google, Apple, WebAuthn und etablierten IdPs übliche Accountauswahl durch
discoverable Credentials, ohne Kontoart-, Tenant- oder Audiencehoheit an den
Authenticator abzugeben.

Die neue Tabelle ist flüchtige, aber sicherheitsrelevante
Authentifizierungsinfrastruktur in PostgreSQL. Sie ist keine fachliche Session
und erzeugt keine Domain-Outbox. Bestehende konto- und sessiongebundene MFA-,
Enrollment- und Reauth-Challenges bleiben unverändert.

