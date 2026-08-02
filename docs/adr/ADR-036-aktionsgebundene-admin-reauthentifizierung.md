# ADR-036 – Aktionsgebundene Admin-Re-Authentifizierung

**Status:** Angenommen  
**Datum:** 2026-08-01

## Entscheidung

- Kritische Admin-Mutationen verlangen zusätzlich zur aktuellen Session und
  normalen serverseitigen Berechtigungsprüfung eine frische Bestätigung des
  aktuellen lokalen Passworts.
- Die Identity-Schicht stellt dafür ein zufälliges, nur als Hash persistiertes
  Ticket aus. Es ist an Admin-Konto, aktuelle Session und
  Sicherheitsgeneration sowie an Berechtigung, Ressourcenart und Ressourcen-ID
  gebunden, höchstens fünf Minuten gültig und genau einmal verwendbar.
- Die Bestätigung wird unmittelbar vor dem fachlichen Kommando verbraucht. Da
  Identity- und Admin-Runtime getrennte Datenbankrollen und Transaktionen
  besitzen, ist die Ticketverwendung nicht atomar mit der Fachmutation. Ein
  anschließend scheiterndes Kommando verbraucht das Ticket sicherheitshalber.
- Version 1 schützt die bereits ausführbaren kritischen Berechtigungen
  `core.tenancy.tenant.create`, `core.tenancy.tenant.update` und
  `core.identity.work-account.update`. Weitere kritische Verträge werden beim
  Anschluss explizit registriert; eine globale, frei wiederverwendbare
  „reauthentifiziert“-Session wird nicht eingeführt.
- Ausstellung und Verbrauch werden im Security-Audit protokolliert. Token und
  Passwort erscheinen weder im Audit noch in Logs oder fachlichen Tabellen.

## Folgen

Ein gestohlener, bereits entsperrter Admin-Browser kann kritische Änderungen
nicht allein mit der Session ausführen. Ein Ticket kann nicht für eine andere
Aktion oder Ressource wiederverwendet werden. Passkey- oder TOTP-basierte
Bestätigung kann später als zusätzliche Methode ergänzt werden, ohne die
Bindungs- und Verbrauchssemantik zu verändern.
