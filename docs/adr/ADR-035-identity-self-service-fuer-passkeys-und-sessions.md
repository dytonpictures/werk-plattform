# ADR-035 – Identity-Self-Service für Passkeys und Sessions

**Status:** Angenommen  
**Datum:** 2026-08-01

## Entscheidung

- Interaktive Work- und Admin-Konten können ausschließlich ihre eigenen
  aktiven Passkeys und Sessions lesen. Tenant und Konto stammen aus der
  aktuellen serverseitig aufgelösten Session.
- Passkey-Ansichten enthalten Bezeichnung, Lifecycle-Zeitpunkte und technische
  ID, aber weder Public Key, Credential-ID, AAGUID noch verschlüsselte Daten.
- Ein Passkey-Widerruf verlangt das aktuelle lokale Passwort. Faktorwiderruf,
  Erhöhung der Sicherheitsgeneration, Widerruf aller Sessions, genau eine
  Ersatzsession und Security-Audit bilden einen PostgreSQL-Commit.
- Sessions zeigen ID, Erzeugung, Ablauf, Authentifizierungsart und Assurance.
  Tokenhashes werden nie ausgegeben. Eine Session kann nur vom selben Konto
  widerrufen werden. Der Widerruf der ausführenden Session führt zum Logout.
- Listenoperationen erzeugen kein Security-Audit. Mutationen sind CSRF-geschützt
  und werden atomar auditiert.

