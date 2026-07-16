# Architektur

WERK wird als Monorepo und modularer Monolith aufgebaut. Ein Go-Backend stellt versionierte APIs unter `/api/v1` bereit, Next.js liefert die Desktop-first-Weboberfläche, PostgreSQL persistiert Anwendungs- und Sitzungsdaten. Docker Compose ist die Referenz für Entwicklung und Self-Hosting.

Module kapseln Geschäftslogik, Repositories und Schnittstellen. HTTP-Handler enthalten keine Geschäftslogik; Abhängigkeiten werden explizit konstruiert.
