# Core Identity: MFA- und Login-Schutz

Stand: 2026-07-22

Administrationskonten können mit einem TOTP-Faktor und einmalig verwendbaren
Recovery-Codes geschützt werden. In Produktion ist MFA verpflichtend. Die
Kontoart und API-Audience werden weiterhin ausschließlich serverseitig bestimmt.

## Konfiguration

```env
WERK_IDENTITY_MFA_ENABLED=true
WERK_IDENTITY_MFA_KEY=<ungepaddetes Base64 einer zufälligen 32-Byte-Folge>
WERK_ALLOWED_ORIGINS=https://werk.example
```

Ein Schlüssel kann beispielsweise mit `openssl rand -base64 32 | tr -d '='`
erzeugt werden. Er ist ein Secret, gehört nicht ins Repository und muss gemeinsam
mit dem Datenbank-Backup gesichert werden. Ohne denselben Schlüssel können
gespeicherte TOTP-Secrets nach einer Wiederherstellung nicht entschlüsselt
werden. Eine spätere Schlüsselrotation benötigt einen expliziten, getesteten
Re-Encryption-Lauf.

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

## Ablauf

1. Ein Admin ohne aktiven Faktor meldet sich zunächst mit Passwort an. Die
   ausgestellte Single-Factor-Session kann die Admin-Zugriffsebene nicht
   autorisieren und führt nach einem nötigen Passwortwechsel zur MFA-Einrichtung.
2. Der Admin bestätigt sein aktuelles Passwort und registriert das angezeigte
   TOTP-Secret in einer Authenticator-App.
3. Erst ein gültiger TOTP-Code aktiviert den Faktor. Die Aktivierung widerruft
   atomar alle aktiven Sitzungen des Kontos und stellt genau eine neue
   interaktive Sitzung mit `multi-factor`-Assurance aus.
4. Zehn zufällige Recovery-Codes werden genau einmal angezeigt und ausschließlich
   gehasht gespeichert.
5. Spätere Anmeldungen erzeugen nach korrektem Passwort nur eine fünf Minuten
   gültige MFA-Challenge. Erst TOTP oder ein unbenutzter Recovery-Code erzeugt
   die Admin-Session.

## Schutzmaßnahmen

- TOTP-Secrets werden mit AES-256-GCM verschlüsselt und kryptografisch an Konto
  und Faktor gebunden.
- Session-, Challenge- und Recovery-Tokens werden nicht im Klartext gespeichert.
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

## Grenzen der aktuellen Ausbaustufe

- WebAuthn ist als Domänen- und Datenvertrag vorbereitet, aber noch nicht an die
  Browser-Routen angeschlossen.
- Self-Service-Widerruf, Faktorwechsel, neue Recovery-Code-Sätze und ein
  beaufsichtigter Kontowiederherstellungsprozess benötigen noch eigene,
  re-authentifizierte Admin-Workflows.
- Zusätzlich zum subjektbezogenen persistenten Throttling soll der Edge-Betrieb
  eine quellbezogene Rate-Limit-Policy besitzen. Dafür muss die Vertrauensgrenze
  für Reverse-Proxy-Header betrieblich festgelegt werden.

Diese offenen Punkte dürfen nicht durch direkte Datenbankänderungen oder ein
allgemeines Administrator-Bypass-Verfahren ersetzt werden.

Änderungen vorbehalten.
