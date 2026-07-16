# Lokale Entwicklung

Die Anwendung ist vollständig über Docker Compose baubar und startbar. `make setup` erzeugt einmalig eine geschützte `.env`, `make up` baut und startet die Dienste, `make migrate` wendet Schemaänderungen an und `make health` prüft Web und API. `make down` bewahrt das PostgreSQL-Volume; nur `make reset-dev-data CONFIRM=yes` entfernt Entwicklungsdaten.

Web läuft standardmäßig auf Port 3000. Der API-Diagnoseport 8080 ist ausschließlich an Loopback gebunden; PostgreSQL besitzt keinen Host-Port.
