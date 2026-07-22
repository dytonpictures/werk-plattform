# ADR-014 – Principals, Provider, Credentials und Audiences

**Status:** Angenommen  
**Datum:** 2026-07-21

## Kontext

Core Identity muss lokale Passwörter, Passkeys, externe Identity Provider,
technische Service-Identitäten und gespeicherte Agenten tragen können. Kontoart,
Anmeldeverfahren, API-Bereich und Token-Empfänger sind unterschiedliche
Sicherheitsdimensionen. Werden sie über denselben Schlüssel modelliert, kann
eine neue Kontoart unbeabsichtigt eine neue API-Audience oder Berechtigung
erhalten.

Ein zentraler Credential-Dienst muss außerdem mehrere Credentials pro Konto,
Widerruf, Rotation, Ablauf und instanzübergreifende Nutzungslimits unterstützen.
Ein externer Provider darf weiterhin weder Kontoart noch Tenant oder
Berechtigungen bestimmen.

## Entscheidung

- `AccountClass` beschreibt den gespeicherten Principal. Unterstützt werden
  Arbeits-, Administrations-, Service- und Agentenkonten.
- Ein Agent ist tenantgebunden, nicht interaktiv und verwendet den technischen
  API-Bereich. Eine eigene Agent-Audience entsteht erst, wenn eine tatsächlich
  getrennte API- und Verifier-Grenze eingeführt wird.
- `CredentialKind` beschreibt ausschließlich den Nachweis, beispielsweise
  Passwort, API-Schlüssel oder später ein Workload-Zertifikat.
- Provider liefern nach erfolgreicher Prüfung nur eine unveränderliche
  Provider-/Subject-Bindung, Authentifizierungsmethode, Zeitpunkt und Assurance.
  Core Identity löst diese Bindung serverseitig zu Kontoart, Tenant und
  Audience auf.
- Kontoarten, Audiences und ihre zulässigen Zuordnungen werden in PostgreSQL
  registriert. Fremdschlüssel und Trigger erzwingen die Sessiongrenze. Neue
  Werte benötigen weiterhin eine Migration, aber keine Änderung historischer
  Migrationen oder verstreuter String-Constraints.
- Ein Konto kann mehrere versionier- und widerrufbare Credentials besitzen.
  Credentials erhalten eigene IDs, öffentliche Lookup-Digests, Ablauf,
  Nutzungszähler und Audit-Zeitpunkte.
- API-Schlüssel bestehen aus einer öffentlichen zufälligen Kennung und einem
  hochentropischen Geheimnis. Beide werden nur als Digest gespeichert; das
  Klartext-Token wird ausschließlich bei der Erzeugung ausgegeben.
- Ein exaktes globales Nutzungslimit wird atomar in PostgreSQL gezählt. Spätere
  kurzlebige Leases dürfen Kontingentblöcke reservieren, müssen aber dieselbe
  fachliche Obergrenze einhalten.
- Rollen und Scopes bleiben das aktuelle RBAC-Fundament. Modell-, Tool-,
  Datenklassen-, Zeit- und Quotenbedingungen werden später als versionierte
  Policy-Attribute ergänzt und niemals aus dem API-Schlüssel abgeleitet.

## Sicherheitsgrenzen

- Interaktive Sessions und technische Credentials verwenden getrennte
  Transport- und HTTP-Verträge.
- Ein Agent kann keine Admin- oder Work-Session erhalten.
- Ein Provider-Ergebnis enthält keine vom Provider gewählte Kontoart, Audience,
  Tenant-ID oder Rolle.
- Deaktivierung des Kontos, Credentials oder Tenants wirkt bei der nächsten
  Auflösung fail-closed.
- Rotation darf ein altes und ein neues Credential kurzzeitig parallel führen;
  Widerruf bleibt je Credential möglich.

## Folgen

Die bestehende lokale Anmeldung bleibt nutzbar, während Passkey-, OIDC-, SAML-,
LDAP- und API-Key-Adapter an dieselbe serverseitige Account-Auflösung anschließen
können. Die zusätzliche Normalisierung erhöht die Zahl der Tabellen und
Invarianten, verhindert dafür aber, dass Provider- oder Credentialdetails in
Kontoart und Autorisierung hineinwachsen.

Replikation und Failover ändern diese fachlichen Dimensionen nicht. Die
Active/Passive-Autorität und der dafür notwendige Identity Witness sind in
[`ADR-015`](ADR-015-identity-authority-witness-und-failover.md) festgelegt.
