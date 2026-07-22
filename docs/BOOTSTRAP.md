# Initialer Administrator

Im lokalen Entwicklungsprofil wird der erste Administrator unter
`admin@werk.local` angelegt und darf die Erstinitialisierung mit dem temporären
Passwort `werk-development` verwenden. Dieses Konto wird mit `must_change_password`
angelegt und darf keine normale Arbeit ausführen, bevor das Passwort geändert
wurde. Das Verhalten ist ausschließlich für Entwicklung und Tests zulässig.

Produktionsprofile benötigen weiterhin ein ausdrücklich gesetztes
Bootstrap-Geheimnis (mindestens 16 Zeichen), zum Beispiel aus einer geschützten
Environment- oder Secret-Datei. WERK besitzt dort kein Default-Passwort und
loggt das Geheimnis nie.

Der Bootstrap-Vorgang ist One-Shot. Der persistente Adapter muss die Anlage in
einer PostgreSQL-Transaktion durchführen und über eine Singleton-/Unique-
Sicherung atomar ablehnen, sobald bereits ein Administrator existiert. Danach
ist das Bootstrap-Geheimnis zu entfernen bzw. zu rotieren. Der Core-Vertrag
liegt in `internal/core/identity/bootstrap.go`.

## Entwicklungs-Arbeitskonto

Nur im Profil `WERK_ENV=development` legt die API zusätzlich idempotent das
normale Arbeitskonto `dev-worker@werk.local` an. Das temporäre Passwort ist
standardmäßig `werk-worker-development` und kann über
`WERK_DEV_WORKER_PASSWORD` ersetzt werden. Es muss mindestens 16 Zeichen lang
sein.

Zum Konto gehören der isolierte Mandant `WERK Development`, die
Organisationseinheit `Development Team`, eine Person, Membership und die
tenantgebundene Rolle `workspace-member`. Das Konto besitzt ausschließlich die
Work-Audience und muss beim ersten Login sein Passwort ändern. Ein Neustart legt
es nicht doppelt an und setzt ein bereits geändertes Passwort niemals zurück.

Außerhalb des Entwicklungsprofils wird `WERK_DEV_WORKER_PASSWORD` abgelehnt;
Produktion und Tests erhalten dadurch kein implizites Arbeitskonto.

Das Entwicklungsprofil aktiviert Admin-MFA standardmäßig mit einem ausschließlich
lokalen Entwicklungsschlüssel. Ein bestehender Admin muss sich nach dem Start
neu anmelden und den TOTP-Faktor einrichten, bevor Benutzer- oder
Tenantverwaltung freigegeben wird. Produktion verlangt weiterhin einen eigenen,
explizit gesetzten Schlüssel.
