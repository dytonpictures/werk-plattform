# ADR-042 – Verbindliche Admin-Assurance und Sessionrichtlinien

**Status:** Angenommen  
**Datum:** 2026-08-02

## Kontext

ADR-032 erlaubt Administrationszugriff mit einer reinen Passwortsitzung, wenn
noch kein zweiter Faktor eingerichtet wurde. Das erleichtert den Bootstrap,
lässt aber eine dauerhafte Single-Factor-Administration zu. Zudem verwenden
alle interaktiven Konten derzeit dieselbe feste Gesamtlaufzeit von zwölf
Stunden und aktionsgebundene Re-Authentifizierung ist an ein lokales Passwort
gekoppelt.

## Entscheidung

- Produktionsprofile verlangen für den Admin-Plane eine Multi-Factor- oder
  phishing-resistente Assurance. Eine Single-Factor-Adminsession erhält nur
  Zugriff auf Passwortwechsel, Faktorregistrierung, eigene Sitzungen und
  Logout.
- Der einmalige Bootstrap darf eine zeitlich begrenzte Enrollment-Session
  erzeugen. Sie ist keine allgemeine Adminsession und kann nach erfolgreicher
  Einrichtung nicht erneut verwendet werden.
- Entwicklungsprofile dürfen die verbindliche Policy ausdrücklich abschalten.
  Der unsichere Zustand bleibt in Sessionprojektion, Admin-UI und
  Betriebsstatus sichtbar.
- Re-Authentifizierung fordert eine Assurance statt eines festen Verfahrens.
  Passwort, Passkey und ein späterer föderierter Step-up dürfen jeweils eigene,
  gleich gebundene Einmaltickets ausstellen.
- Sessionrichtlinien unterscheiden mindestens Kontoart, Idle-Laufzeit und
  absolute Laufzeit. PostgreSQL bleibt die autoritative Sessionquelle.
- Jeder erfolgreiche authentifizierte Zugriff darf `last_seen_at` nur
  gedrosselt fortschreiben. Ein Cache darf den nächsten zulässigen
  Persistenzzeitpunkt koordinieren, aber weder Laufzeit verlängern noch einen
  Widerruf überstimmen.
- Admin- und Work-Sessions bleiben getrennte Datensätze und Audiences. Eine
  Assurance-Erhöhung rotiert die Session; sie ändert keine Kontoart und keine
  Berechtigung.

## Ablösung

Dieses ADR ersetzt die Freigabe dauerhafter Single-Factor-Administration aus
ADR-032. Dessen aktions- und ressourcengebundener Re-Authentisierungsvertrag,
Kontoartgrenzen, Permissions, CSRF, Audit und atomare Schreibgrenzen bleiben
verbindlich.

## Folgen

Bootstrap und Wiederherstellung bleiben möglich, ohne die normale
Administration dauerhaft auf Passwortniveau zu belassen. Unterschiedliche
Sessionlaufzeiten werden zu einer expliziten Policy statt zu verstreuten
Konstanten.

