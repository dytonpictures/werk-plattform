# Core Identity: MFA- und Login-Schutz

Stand: 2026-08-01

Interaktive Work- und Administrationskonten können Passkeys registrieren und
damit phishing-resistent anmelden. Administrationskonten können zusätzlich mit
einem TOTP-Faktor und einmalig verwendbaren Recovery-Codes geschützt werden.
Eine gültige interaktive Admin-Sitzung darf `single-factor` oder `multi-factor`
tragen; eine unbekannte Assurance wird fail-closed abgewiesen. MFA bleibt eine
nicht blockierende, selbst gestartete Empfehlung. Kontoart, Tenant und
API-Audience werden weiterhin ausschließlich serverseitig bestimmt.

## Provider- und Bindungsgrenze

Core Identity entscheidet nicht allein anhand eines einmal erfolgreich
geprüften Credential über eine Anmeldung. Jeder Abschluss eines Passwort-,
MFA-, Passkey- oder API-Key-Vorgangs prüft in seiner abschließenden Transaktion
erneut das exakt verwendete aktive und nicht abgelaufene Credential, den dazu
gespeicherten aktiven Identity-Provider mit der zur Methode passenden
Providerart und die konkrete aktive Konto-Provider-Bindung. Die Provider- und
Binding-Zeilen werden dabei bis zum Transaktionsende gesperrt. Fehlt oder
ändert sich eine dieser drei Grundlagen, werden Session-Ausstellung,
Assurance-Erhöhung und Credential-Mutation fail-closed abgelehnt.

Ausstehende MFA-Challenges speichern die `credential_id` des Passwort-
Credentials, das den ersten Faktor erfüllt hat. Der MFA-Abschluss muss genau
dieses Credential samt Provider und Bindung erneut auflösen; Austausch,
Deaktivierung oder Ablauf des Credentials macht die begonnene Challenge
unbrauchbar. Der Provider wird aus vertrauenswürdigen gespeicherten
Credential-Metadaten beziehungsweise für Passkeys aus der festen lokalen
Bindung bestimmt, nicht aus einer frei wählbaren Login-Angabe. Alle Ablehnungen
verwenden dieselbe allgemeine Authentifizierungsfehlermeldung wie ungültige
Credentials.

Auch ein späterer externer Identitätsnachweis darf nur eine Provideridentität
bestätigen. Core Identity löst daraus serverseitig das aktive Konto und dessen
unveränderliche Kontoart, Tenant-Zuordnung, Audience und Assurance auf. Ein
Provider darf diese Werte und Berechtigungen nicht liefern oder überschreiben.

Eine gültige Admin-Sitzung mit der Berechtigung `core.identity.provider.read`
kann unter `/admin/v1/identity/providers` eine schreibgeschützte Übersicht der Anmeldeanbieter
abrufen. Sie zeigt registrierte Provider-Metadaten,
deren Lifecycle und Anmeldestatus sowie für den lokalen Provider aktivierte
Passwort-/Passkey-Funktionen, MFA-Status, WebAuthn-RP-ID, RP-Name und nur die
Anzahl konfigurierter Origins. Die Antwort enthält höchstens 100 Provider und
setzt `truncated`, wenn weitere Einträge vorhanden sind. `provider_key` ist
auf 120 Zeichen begrenzt; ein vorhandener `issuer` muss getrimmt sein und darf
höchstens 2048 Zeichen enthalten. Secrets und konkrete Originwerte werden nicht
ausgegeben; der Abruf ist nicht cachebar und wird auditiert.

Ein eng begrenzter OIDC-Protokolladapter ist implementiert und isoliert geprüft,
aber noch nicht mit Ceremony-Persistenz, HTTP-Callback, Secret-Provider und
Composition Root zu einem ausführbaren externen Anmeldeweg verbunden. SAML und
LDAP sind noch nicht implementiert. Deshalb bleibt die externe
Providerkonfiguration deaktiviert, und ein aktiver Metadateneintrag wird als
als `not-ready` ausgewiesen. Die OIDC-Protokollprüfung ist separat als
`protocol-implemented` sichtbar, ohne damit Konfiguration oder
Anmeldebereitschaft zu behaupten. Diese Identity-Übersicht ist nicht die globale
Service-/Provider-Registry und ersetzt deren Laufzeitprüfung nicht.

## Admin-Assurance und MFA-Empfehlung

Die Administrationsebene akzeptiert ausschließlich eine interaktive Session
der Kontoart `admin` mit Admin-Audience und ohne Tenant-Kontext. Ihre Assurance
muss bekannt sein: `single-factor` und `multi-factor` sind zulässig,
`unknown` wird abgewiesen. Eine Single-Factor-Session erhält die normal
autorisierten Admin-Funktionen und wird nicht in eine Einrichtungs-Shell
gesperrt.

Die Session-Antwort kennzeichnet ein Administrationskonto ohne aktiven Faktor
mit `mfa_enrollment_recommended`. Das Feld ist ein Hinweis für den
selbst gestarteten Einrichtungsweg und kein Berechtigungs-Gate. Das veraltete
Feld `mfa_enrollment_required` bleibt nur kompatibilitätshalber vorhanden und
ist keine globale Zugangsbedingung. Sobald TOTP aktiviert wurde, bleibt der
zweite Faktor bei späteren Passwortanmeldungen verpflichtend.

Berechtigungen, Ressourcenscope, Datenprofil und Processing-Policy, ein
expliziter Tenant für tenantgebundene Befehle, CSRF, Audit und atomare Outbox
werden unabhängig von der Session-Assurance geprüft. Künftige besonders
sensible Aktionen dürfen eine ausdrückliche, aktions- und ressourcengebundene
Re-Authentifizierung, Just-in-time-Freigabe oder Mehrpersonenfreigabe verlangen;
eine solche Anforderung muss im jeweiligen Vertrag stehen. Die Grenze legt
[`ADR-032`](adr/ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md)
fest.

## Tenant-Bindung und Kontostatus

Die Tenant-Zuordnung eines Work-Kontos wird bei der Provisionierung
serverseitig festgelegt und ist anschließend eine unveränderliche
Sicherheitsgrenze. Das Administrationsverzeichnis zeigt sie ausdrücklich an.
Der Statusvertrag adressiert Tenant und Konto separat und erlaubt nur
`active` ↔ `disabled`; er verschiebt weder das Konto noch Person, Login oder
Membership in einen anderen Tenant.

Eine wirkliche Statusänderung verlangt die aktuelle Version über ein starkes
`If-Match`, erhöht `version` und `session_generation` atomar und schreibt Audit
plus Outbox. Dadurch können zuvor ausgestellte Sessions des Kontos nicht weiter
verwendet werden. Eine Wiederholung desselben Status ist ein idempotenter
No-op. Eine Tenant-Verschiebung oder Multi-Tenant-Anmeldung ist kein Bestandteil
dieses Vertrags.

Ein Work-Konto mit offener initialer Einladung befindet sich absichtlich im
Status `disabled` und besitzt noch kein Passwort-Credential. Sein Status darf
nicht über die allgemeine Admin-Statusmutation aktiviert werden. Ausschließlich
die atomare Einladungseinlösung kann das erste Credential anlegen, die
Einladung verbrauchen und das Konto aktivieren.

## Work-Konto-Onboarding

`POST /admin/v1/work-users` verlangt neben Tenant, Organisationseinheit und
Personendaten eine ausdrückliche Bereitstellungsart:

- `initial-password` legt ein aktives Konto mit einem vom Administrator
  vergebenen Startpasswort an. `require_password_change` muss vorhanden sein;
  nur `true` erzwingt den Wechsel beim ersten Login. Bei `false` weist die
  Oberfläche darauf hin, dass der Administrator das dauerhafte Startpasswort
  kennt.
- `invitation-link` legt ein deaktiviertes Konto ohne Credential an. Der
  gültig angemeldete und für die Kontoerstellung berechtigte Admin wählt
  ausdrücklich eine Gültigkeit zwischen 1 und 168 Stunden und erhält den
  Aktivierungslink genau in der Erstellungsantwort. Die eingegebene
  Empfängeradresse wird nicht in Core Identity gespeichert und WERK behauptet
  keinen automatischen Versand; der Browser kann nur einen lokalen
  E-Mail-Entwurf öffnen.

Der Link trägt den 32-Byte-Roh-Token ausschließlich im URL-Fragment. Die
Aktivierungsseite entfernt das Fragment unmittelbar und sendet Token und selbst
gewähltes Passwort im Body an
`POST /api/v1/auth/invitations/initial/accept`. PostgreSQL speichert nur den
SHA-256-Digest. Erfolgreiche Einlösung aktiviert Konto und Credential, erhöht
die Sessiongeneration und schreibt Security-Audit sowie Outbox atomar. Es wird
keine Sitzung ausgestellt; der Benutzer meldet sich anschließend normal an.
Unbekannte, abgelaufene, verbrauchte und widerrufene Einladungen liefern
dieselbe öffentliche Ablehnung.

Ein gültig angemeldeter und für die Kontoänderung berechtigter Admin kann eine
noch offene oder bereits abgelaufene initiale Einladung über
`POST /admin/v1/tenants/{tenantId}/work-users/{accountId}/invitation` neu
ausgeben. Der Vertrag verwendet die vorhandene Berechtigung
`core.identity.work-account.update`, verlangt Empfängeradresse und eine neue
Gültigkeit von 1 bis 168 Stunden ausdrücklich und ersetzt den bisherigen Link
atomar. Eine Neuausgabe ist nur bei aktivem Mandanten möglich. Auch hier bleiben
Adresse und Roh-Token auf die einmalige, mit
`Cache-Control: no-store` markierte Antwort begrenzt; die Oberfläche öffnet
höchstens einen lokalen E-Mail-Entwurf und behauptet keinen Versand.

Der öffentliche Aktivierungsendpunkt begrenzt jeden direkt verbundenen Peer in
einem festen Zeitfenster auf 12 Versuche pro Minute. `Forwarded`- und
`X-Forwarded-For`-Header gelten dabei absichtlich nicht als vertrauenswürdige
Quellidentität. Prozessweit laufen höchstens zwei Argon2id-Hashvorgänge für
diese Zeremonie gleichzeitig; ein weiterer Aufruf wartet höchstens zwei
Sekunden auf Kapazität. Beide Begrenzungen antworten mit `429 Too Many Requests`
und einem `Retry-After`-Header.

## Konfiguration

```env
WERK_IDENTITY_MFA_ENABLED=true
WERK_IDENTITY_MFA_KEY=<ungepaddetes Base64 einer zufälligen 32-Byte-Folge>
WERK_ALLOWED_ORIGINS=https://werk.example
WEBAUTHN_RP_ID=werk.example
WEBAUTHN_RP_NAME=WERK
WEBAUTHN_ORIGINS=https://werk.example
```

`WERK_IDENTITY_MFA_ENABLED=true` stellt die MFA-Funktion und ihre
Einrichtungswege bereit. Die Einstellung schreibt keinem Admin einen Faktor vor
und ist weder ein globales Enrollment- noch ein globales Admin-Zugangs-Gate.

Ein Schlüssel kann beispielsweise mit `openssl rand -base64 32 | tr -d '='`
erzeugt werden. Er ist ein Secret, gehört nicht ins Repository und muss gemeinsam
mit dem Datenbank-Backup gesichert werden. Ohne denselben Schlüssel können
gespeicherte TOTP-Secrets, Passkey-Credentials und laufende WebAuthn-Zeremonien
nach einer Wiederherstellung nicht entschlüsselt werden. Eine spätere
Schlüsselrotation benötigt einen expliziten, getesteten Re-Encryption-Lauf.

Für eine Rotation kann statt des Einzelschlüssels ein Schlüsselring verwendet
werden:

```env
WERK_IDENTITY_MFA_CURRENT_KEY_ID=current
WERK_IDENTITY_MFA_KEYS=current:<Base64-32-Byte>,previous:<Base64-32-Byte>
```

Neue Secrets werden mit der aktuellen Key-ID geschrieben; bestehende
`v1`-Referenzen und `v2`-Referenzen früherer Schlüssel bleiben lesbar, solange
der betreffende Schlüssel im Ring vorhanden ist. Ein alter Schlüssel darf erst
nach einem geprüften Re-Encryption-Lauf entfernt werden.

In einem späteren Active/Passive-Profil müssen Haupt- und Reserveinstanz vor
einer Promotion denselben vollständig verfügbaren Schlüsselring besitzen. Der
Platform Witness erhält diese Schlüssel ausdrücklich nicht. Nur der aktuelle
Lease-Inhaber darf neue MFA-Secrets schreiben oder eine Rotation beginnen; ein
alter Schlüssel wird erst entfernt, wenn Re-Encryption, Replikation,
Reserve-Lesbarkeit und Wiederherstellung nachgewiesen sind. Die allgemeine
Failover-Grenze steht in
[`ADR-015`](adr/ADR-015-identity-authority-witness-und-failover.md).

`WERK_ALLOWED_ORIGINS` ist eine kommaseparierte Liste vollständiger Origins ohne
Pfad. In Produktion muss sie explizit gesetzt sein. Wildcards werden nicht
akzeptiert.

`WEBAUTHN_RP_ID` ist der klassische WebAuthn-Hostname ohne Schema oder Port.
`WEBAUTHN_ORIGINS` enthält ausschließlich vollständige Origins unterhalb dieser
RP-ID; in Produktion ist nur HTTPS zulässig. Lokal ist `localhost` mit
`http://localhost:3000` vorbelegt, weil Browser WebAuthn dort als sicheren
Entwicklungskontext behandeln.

## Ablauf

1. Ein Admin ohne aktiven Faktor meldet sich mit Passwort an. Die ausgestellte
   Single-Factor-Session darf die normal autorisierten Admin-Funktionen nutzen;
   die Oberfläche empfiehlt die optionale MFA-Einrichtung, ohne den Zugang zu
   sperren.
2. Entscheidet sich der Admin für die Einrichtung, startet er sie selbst,
   bestätigt sein aktuelles Passwort und registriert das angezeigte TOTP-Secret
   in einer Authenticator-App.
3. Erst ein gültiger TOTP-Code aktiviert den Faktor. Die Aktivierung widerruft
   atomar alle aktiven Sitzungen des Kontos und stellt genau eine neue
   interaktive Sitzung mit `multi-factor`-Assurance aus.
4. Zehn zufällige Recovery-Codes werden genau einmal angezeigt und ausschließlich
   gehasht gespeichert.
5. Spätere Anmeldungen erzeugen nach korrektem Passwort nur eine fünf Minuten
   gültige MFA-Challenge. Erst TOTP oder ein unbenutzter Recovery-Code erzeugt
   die Admin-Session.

### Passkey-Ablauf

1. Ein angemeldetes Work- oder Administrationskonto bestätigt sein aktuelles
   Passwort und benennt den neuen Passkey.
2. WERK erzeugt eine fünf Minuten gültige, serverseitig verschlüsselte
   WebAuthn-Zeremonie. Der Browser registriert einen residenten Credential mit
   verpflichtender Benutzerverifikation.
3. Erst nach erfolgreicher Prüfung von Challenge, Origin, RP-ID,
   Benutzerpräsenz, Benutzerverifikation und Attestation wird der Passkey aktiv.
   Die Aktivierung widerruft atomar alle bisherigen Sessions und stellt genau
   eine neue Multi-Faktor-Session aus.
4. Die Anmeldung startet ohne Anmeldenamen als discoverable WebAuthn-Zeremonie
   mit leerer Credential-Liste. Browser oder Betriebssystem wählen einen
   residenten Passkey. Erst die signierte Credential-ID und der opake
   User-Handle werden serverseitig zu einem aktiven Konto aufgelöst.
5. Anonyme Zustände liegen getrennt von kontogebundenen MFA-Challenges, sind
   fünf Minuten gültig und werden atomar einmalig konsumiert. Startversuche
   erzeugen kein dauerhaftes Audit; direkte Peer-, Prozess- und
   installationsweite Bestandsgrenzen verhindern unbegrenztes Wachstum.
6. WERK prüft Challenge, Origin, RP-ID, Benutzerverifikation, Signatur,
   Credential-Zustand, Provider-Binding, Konto, Tenant und Sessiongeneration
   und aktualisiert Credential, Session und Erfolgs-Audit in einer Transaktion.
7. Ein Administrationskonto mit ausschließlich Passkey-Faktor kann nicht über
   ein Passwort auf die Admin-Ebene ausweichen; es verwendet den Passkey-Knopf
   auf der gemeinsamen Anmeldung. TOTP bleibt ein getrennt einrichtbarer
   Ausweichweg.

## Schutzmaßnahmen

- TOTP-Secrets werden mit AES-256-GCM verschlüsselt und kryptografisch an Konto
  und Faktor gebunden.
- Vollständige WebAuthn-Credential-Datensätze und der unveränderte
  Zeremoniezustand werden mit AES-256-GCM an Konto und Faktor beziehungsweise
  eine anonyme Ceremony-ID gebunden. Nur Lookup-ID, Hash, RP-ID, Ablauf und die
  zur Konsistenzprüfung benötigten Projektionen liegen separat vor.
- WebAuthn erzwingt `userVerification=required`, residente Credentials, eine
  feste RP-ID, eine explizite Origin-Liste und serverseitig durchgesetzte fünf
  Minuten Ablaufzeit. Ein Konto kann bis zu zehn Passkeys besitzen.
- Session-, Challenge- und Recovery-Tokens werden nicht im Klartext gespeichert.
- Initiale Einladungstokens werden ebenfalls nie im Klartext persistiert,
  auditiert oder in der Outbox veröffentlicht. Die Annahme sperrt Konto,
  Einladung sowie lokalen Provider/Binding und erzeugt Credential, Aktivierung,
  Audit und Outbox in einer PostgreSQL-Transaktion.
- Session- und Challenge-Cookies sind `HttpOnly`, `Secure` bei HTTPS und
  `SameSite=Strict`.
- Ein erfolgreicher Passwortwechsel und die erste TOTP-Aktivierung widerrufen
  atomar alle aktiven Sitzungen des Kontos und stellen genau eine neue
  interaktive Sitzung aus. Beim Passwortwechsel wird die passende bestehende
  Assurance übernommen, bei der TOTP-Aktivierung ist sie `multi-factor`.
- Die HTTP-Schicht rotiert dabei sowohl das Session- als auch das CSRF-Cookie.
  Vorherige Session- und CSRF-Token sind nach Abschluss der Transaktion
  ungültig.
- Sitzungen und MFA-Challenges tragen die aktuelle `session_generation` des
  Kontos. Passwortwechsel und erste TOTP-Aktivierung erhöhen sie atomar; ein
  bereits vorher begonnener Login oder eine alte Challenge kann danach keine
  Sitzung mehr ausstellen.
- Eine ausstehende MFA-Challenge ist zusätzlich an die `credential_id` des
  tatsächlich geprüften Passwort-Credentials gebunden. Sie kann nach dessen
  Austausch, Deaktivierung oder Ablauf nicht mit einem anderen Credential
  abgeschlossen werden.
- Der Passwortwechsel verlängert keine bestehende Sitzung: Die Ersatzsitzung
  übernimmt höchstens deren absolute Ablaufzeit. Die erfolgreiche erstmalige
  TOTP-Bestätigung darf als frische Multi-Factor-Zeremonie eine neue
  Sitzungslaufzeit beginnen.
- Jede erfolgreiche Rotation schreibt das Security-Audit-Ereignis
  `identity.session.rotated.v1`; Session-Rohwerte werden nicht protokolliert.
  Die übergreifende Invariante ist in
  [`ADR-024`](adr/ADR-024-sessionrotation-und-sicherheitsgeneration.md)
  festgelegt.
- Schreibende Cookie-Aufrufe benötigen eine erlaubte `Origin`, passende Fetch
  Metadata und ein konstante-Zeit-geprüftes Double-Submit-CSRF-Token.
- Anmeldefehler werden pro normalisiertem, gehashtem Loginbezeichner persistent
  gezählt. Acht Fehler innerhalb von 15 Minuten sperren weitere Versuche für 15
  Minuten. Die Tabelle enthält keinen Loginbezeichner im Klartext.
- Nicht vorhandene Konten durchlaufen eine Argon2id-Dummyprüfung und erhalten
  dieselbe öffentliche Fehlermeldung wie ein falsches Passwort.
- MFA-Challenge und Enrollment werden nach fünf falschen Codes unbrauchbar.
- Erfolgreiche und abgelehnte MFA-Schritte werden im append-only Security-Audit
  mit Request- und Korrelations-ID erfasst; Geheimnisse und Codes werden nicht
  protokolliert.

## Passkey- und Session-Self-Service

`GET /api/v1/auth/passkeys` und `GET /api/v1/auth/sessions` liefern nur den
Bestand des Kontos der aktuellen Session. Credential-IDs, Public Keys,
Tokenhashes und verschlüsselte Referenzen verlassen Core Identity nicht.

Der Widerruf eines Passkeys verlangt das aktuelle lokale Passwort. Faktor,
Sicherheitsgeneration, Widerruf aller Sessions, Ersatzsession und Audit werden
atomar gespeichert. Der gezielte Sessionwiderruf sperrt Account und betroffene
Sessions in stabiler Reihenfolge und kann niemals eine Session eines anderen
Kontos adressieren. Beim Widerruf der aktuellen Session löscht die HTTP-Schicht
Session- und CSRF-Cookie.

## Grenzen der aktuellen Ausbaustufe

- OIDC-, SAML- und LDAP-Anmeldung sowie eine schreibende externe
  Providerverwaltung benötigen zuerst jeweils einen implementierten,
  getesteten Adaptervertrag. Ein Registry-Eintrag allein genügt nicht.
- TOTP-Faktorwechsel, neue Recovery-Code-Sätze und ein
  beaufsichtigter Kontowiederherstellungsprozess benötigen noch eigene,
  re-authentifizierte Admin-Workflows.
- Einladungen können sicher angenommen und durch einen gültig angemeldeten und
  berechtigten Admin neu ausgegeben werden. Ein produktiver Mail-Provider und
  ein eigenständiger adminseitiger Widerrufsworkflow benötigen noch eigene
  versionierte Verträge.
- Zusätzlich zum direkten Peer-Limit kann der Edge-Betrieb eine weitergehende
  quellbezogene Rate-Limit-Policy besitzen. Reverse-Proxy-Header werden erst
  nach einem eigenen Trusted-Proxy-Vertrag als Quellidentität ausgewertet.

Diese offenen Punkte dürfen nicht durch direkte Datenbankänderungen oder ein
allgemeines Administrator-Bypass-Verfahren ersetzt werden.

Änderungen vorbehalten.
