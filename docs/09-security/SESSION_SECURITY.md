# Sitzungssicherheit

Browser erhalten zufällige, rotierbare Sitzungstoken ausschließlich in `HttpOnly`-, `Secure`- und passenden `SameSite`-Cookies. PostgreSQL speichert nur Token-Hashes sowie Ablauf und Widerruf. Login rotiert die Sitzung; Logout widerruft sie.
