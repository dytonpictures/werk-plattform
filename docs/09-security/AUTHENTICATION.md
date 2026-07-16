# Authentifizierung

Anmeldung liefert generische Fehlermeldungen und wird rate-limitiert. Passwörter werden mit Argon2id und dokumentierten Parametern gehasht. Passwortänderungen und administrative Resets widerrufen bestehende Sitzungen und erzeugen Audit-Ereignisse.
