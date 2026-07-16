# ADR-0009: Serverseitige Cookie-Sitzungen

- **Status:** Accepted
- **Kontext:** Browserauthentifizierung benötigt widerrufbare, sichere Sitzungen.
- **Entscheidung:** Undurchsichtige Token werden in sicheren HttpOnly-Cookies übertragen.
- **Begründung:** Kein Token in `localStorage`, zentrale Kontrolle und Widerruf.
- **Alternativen:** Browser-JWTs, externe Identity-Plattform.
- **Positive Folgen:** Gute Kontrolle über Lebenszyklus und Logout.
- **Negative Folgen:** Serverseitiger Sitzungszustand.
- **Sicherheitsauswirkungen:** Secure, HttpOnly, SameSite, Rotation und CSRF-Schutz sind Pflicht.
- **Überprüfung:** Bei Einführung externer Clients oder SSO.
