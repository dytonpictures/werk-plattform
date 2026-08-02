# ADR-024 – Sessionrotation und Sicherheitsgeneration

**Status:** Angenommen
**Datum:** 2026-07-22

## Kontext

Ein Passwortwechsel oder die erstmalige Aktivierung eines zweiten Faktors
verändert den Sicherheitszustand eines Kontos. Eine bestehende Session nur in
der Datenbank aufzuwerten oder lediglich die gerade bekannten Sessions zu
widerrufen verhindert jedoch keinen parallel begonnenen Login: Dieser könnte
den alten Passwort- oder Faktorstand bereits geprüft haben und erst nach der
Änderung eine neue Session speichern.

Die Lösung muss für Arbeits- und Administrationskonten gelten, Tenant- und
Audience-Grenzen erhalten, keine Roh-Tokens auditieren und ohne einen
bestimmten Identity Provider funktionieren.

## Entscheidung

### Atomare Ersatzsitzung

Ein erfolgreicher Passwortwechsel und die erstmalige TOTP-Aktivierung werden in
jeweils einer PostgreSQL-Transaktion abgeschlossen:

1. Konto und ausführende Session werden in stabiler Reihenfolge gesperrt.
2. Credential beziehungsweise Faktor wird verbindlich geändert.
3. Die Sicherheitsgeneration des Kontos wird erhöht.
4. Alle nicht widerrufenen Sessions des Kontos werden widerrufen.
5. Genau eine neue interaktive Session wird mit der neuen Generation erzeugt.
6. Mutation und `identity.session.rotated.v1` werden im Security-Audit
   gespeichert; dessen Export-Queue entsteht durch den bestehenden Trigger.

Schlägt ein Schritt fehl, werden Credential-/Faktoränderung, Widerruf,
Ersatzsitzung und Audit gemeinsam zurückgerollt.

### Sicherheitsgeneration

Konten besitzen eine monoton steigende `session_generation`. Sessions und
MFA-Challenges speichern die Generation, die beim Authentifizieren unter
Kontosperre beobachtet wurde. Auflösung und Abschluss akzeptieren nur eine
Übereinstimmung mit der aktuellen Kontogeneration.

Ein Login, der vor einer Rotation begonnen wurde, darf deshalb nach der
Rotation keine Session mehr veröffentlichen. Die Datenbank verlangt die
Generation bei neuen Sessions und Challenges explizit; ein stiller Default ist
nach der Migration nicht zulässig. Eine Kontogeneration darf nicht sinken.

### Assurance und Laufzeit

- Ein Passwortwechsel übernimmt Audience, Tenant, Authentifizierungsart und
  Assurance der ausführenden Session. Seine Ersatzsitzung läuft höchstens bis
  zur bisherigen absoluten Ablaufzeit, damit ein Passwortwechsel keine alte
  Multi-Factor-Zeremonie verlängert.
- Eine erfolgreich bestätigte erstmalige TOTP-Aktivierung stellt eine neue
  Admin-Session mit `multi-factor`-Assurance und neuer Sitzungslaufzeit aus.
- Eine Sessionrotation erteilt keine Rollen oder fachlichen Berechtigungen.

### Transport und Fehler

Der Core gibt die Ersatzberechtigung als nicht JSON-serialisierbaren internen
`SessionRotation`-Wert an den HTTP-Adapter. Dieser ersetzt Session- und
CSRF-Cookie gemeinsam. Tokens erscheinen weder im Response-Body noch im Audit.
Erwartbare Credentialfehler werden als Ablehnung behandelt; interne Speicher-,
Commit- und Auditfehler werden nicht als vermeintlich falsche Zugangsdaten
klassifiziert.

Teure Passwortverifikation erfolgt ohne gehaltene PostgreSQL-Zeilensperren.
Innerhalb der Schreibtransaktion wird der zuvor beobachtete Passwort-Hash unter
Sperre nochmals konstantzeitlich verglichen. Änderungen zwischen Prüfung und
Commit schlagen dadurch geschlossen fehl.

## Bewusste Nicht-Ziele

- noch kein allgemeiner Re-Authentifizierungsvertrag für besonders kritische
  Fachaktionen,
- noch kein Self-Service für Faktorwechsel, Recovery oder globales
  Session-Management,
- keine Änderung der getrennten Work-, Admin- und Service-Zugriffsebenen,
- kein Ersatz für spätere datenbankgestützte Parallelitäts- und Lasttests.

## Folgen

Sicherheitsrelevante Zustandswechsel besitzen eine explizite, providerneutrale
Fencing-Grenze. Zusätzliche Credentialarten können dieselbe Generation später
verwenden, müssen ihre eigene Zeremonie, Assurance und Laufzeit aber weiterhin
festlegen. Die env-gebundenen PostgreSQL-Integrationstests bleiben vor einem
Produktionsrelease verpflichtend; Änderungen vorbehalten.
