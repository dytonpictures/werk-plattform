# ADR-030 – Identitäts-Onboarding und MFA-Aktivierung

**Status:** Angenommen  
**Datum:** 2026-07-29

## Kontext

Ein neuer Arbeitsbenutzer soll nicht durch einen ungefragten MFA-Zwang oder
einen pauschalen Passwortwechsel geführt werden. Auch ein Administrator soll
einen zweiten Faktor bewusst selbst einrichten können, ohne bis dahin von allen
regulär autorisierten Plattformaktionen ausgeschlossen zu sein. Vom
Administrator vergebene Startpasswörter und per E-Mail
versendete Geheimnisse würden außerdem ein dauerhaftes Benutzerpasswort für
eine weitere Person sichtbar machen.

WERK besitzt noch keinen ausführbaren Mail-Provider. Die Oberfläche darf daher
weder einen automatischen Versand behaupten noch SMTP-Zugangsdaten in Core
Identity aufnehmen.

## Entscheidung

- `admin` und `work` bleiben getrennte Kontoarten. Eine gültige interaktive
  `admin`-Session mit `admin`-Audience, ohne Tenant-Kontext und mit
  `single-factor`- oder `multi-factor`-Assurance darf die Admin-Plane betreten;
  `unknown` bleibt abgelehnt. Jede Query und Mutation benötigt weiterhin ihre
  serverseitige Permission-, Scope-, Policy-, CSRF- und Auditprüfung.
- Ein Admin-Konto ohne Faktor erhält nach korrektem Passwort eine reguläre
  Single-Factor-Session. Die Admin-Oberfläche empfiehlt die selbst gestartete
  MFA-Einrichtung, sperrt die sonst autorisierten Administrationsfunktionen aber
  nicht. Ein aktivierter TOTP-Faktor bleibt bei späteren Passwortanmeldungen
  wirksam.
- Work-Konten werden nicht zur MFA-Einrichtung umgeleitet. Das Profil empfiehlt
  den bereits vorhandenen Passkey-Self-Service, den der Benutzer selbst startet.
  Eine zusätzliche Work-TOTP- oder kontoweite MFA-Pflicht ist nicht Teil dieses
  Entscheids.
- Das Work-Konto-Onboarding kennt zwei ausdrückliche Wege:
  1. Bei einem manuell vergebenen Startpasswort entscheidet ein verpflichtend
     übermitteltes Boolean ausdrücklich, ob beim ersten Login ein
     Passwortwechsel verlangt wird. Die Oberfläche weist darauf hin, dass ein
     Administrator ein nicht zu wechselndes Startpasswort dauerhaft kennt.
  2. Ein Einladungslink lässt den Empfänger sein Passwort selbst setzen. Danach
     ist kein zusätzlicher Erstwechsel offen.
- Einladungstokens bestehen aus 32 kryptografisch zufälligen Bytes, sind
  kurzlebig und einmalig. PostgreSQL speichert ausschließlich den SHA-256-
  Digest. Rohwerte erscheinen weder in Audit, Outbox, Kafka noch Logs.
- Die Einlösung sperrt in der Reihenfolge Konto → Einladung → lokaler
  Provider/Binding, erzeugt anschließend das erste Credential und speichert
  Tokenverbrauch, Sessiongeneration, Widerrufe, Audit und Domain-Outbox atomar
  in PostgreSQL.
- Eine offene oder abgelaufene initiale Einladung kann ein gültig angemeldeter
  Admin mit der vorhandenen Berechtigung
  `core.identity.work-account.update` neu ausgeben. Die tenantgebundene
  Transaktion widerruft den bisherigen Link und speichert den Ersatzdigest,
  Audit und Outbox atomar. Sie sperrt den aktiven Tenant bis zum Commit und
  lehnt suspendierte oder archivierte Mandanten ab. Roh-Token und
  Empfängeradresse bleiben auch bei der Neu-Ausgabe auf die nicht cachebare
  Einmalantwort begrenzt.
- Solange kein versionierter Mail-Provider konfiguriert ist, liefert WERK den
  Einladungslink genau einmal an die autorisierte Admin-Oberfläche. Eine
  ausdrücklich eingegebene Versandadresse wird dafür nicht als Identity-Datum
  persistiert. Die Oberfläche kann einen lokalen E-Mail-Entwurf vorbereiten,
  bezeichnet ihn aber ausdrücklich nicht als versendet. Gültigkeitsdauer und
  Adresse werden beim Erstellen explizit angegeben. Automatischer Versand folgt
  erst mit einem
  getrennten Notification-/Providervertrag samt Secret-, Retry- und
  Zustellstatusmodell.

## Folgen

Neue Arbeitskonten können ohne pauschale Sicherheitsschritte starten und selbst
entscheiden, ob sie MFA aktivieren. Administratoren erhalten eine verständliche,
nicht blockierende Empfehlung. Die Mindest-Assurance der bestehenden Admin-
Funktionen sinkt bewusst auf `single-factor`; Kontoart, Audience, Tenant-,
Permission-, Policy-, CSRF- und Auditgrenzen bleiben bestehen. Ein
Einladungslink ersetzt das gemeinsame Versenden von Benutzername und Passwort,
ist aber bis zur Implementierung eines Mail-Providers bewusst ein manueller
Versandvorgang. Offene oder abgelaufene Links lassen sich neu ausgeben, ohne ein
deaktiviertes Konto durch direkte Status- oder Datenbankänderungen zu umgehen;
der vorherige Link verliert dabei atomar seine Gültigkeit.

Die Kontoart- und Sessiongrenzen aus
[`ADR-014`](ADR-014-principals-provider-credentials-und-audiences.md) sowie die
Sessionrotation aus [`ADR-024`](ADR-024-sessionrotation-und-sicherheitsgeneration.md)
bleiben verbindlich. Die aktuelle Admin-Assurance und die Grenze für spätere
aktionsgebundene Re-Authentifizierung legt
[`ADR-032`](ADR-032-optionale-admin-mfa-und-aktionsgebundene-reauthentifizierung.md)
fest.
