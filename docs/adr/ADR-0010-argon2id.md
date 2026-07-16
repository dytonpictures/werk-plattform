# ADR-0010: Argon2id für Passwort-Hashing

- **Status:** Accepted
- **Kontext:** Passwörter benötigen speicherharten Schutz.
- **Entscheidung:** Passwörter werden mit Argon2id und versionierten Parametern gehasht.
- **Begründung:** Widerstand gegen GPU-basierte Angriffe.
- **Alternativen:** bcrypt, scrypt.
- **Positive Folgen:** Moderner Passwortschutz.
- **Negative Folgen:** Ressourcenparameter müssen kalibriert werden.
- **Sicherheitsauswirkungen:** Zufällige Salts, Limits und sichere Vergleiche sind erforderlich.
- **Überprüfung:** Regelmäßig anhand aktueller Hardware und Empfehlungen.
