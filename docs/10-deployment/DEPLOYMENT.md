# Deployment

Docker Compose betreibt Web, API und PostgreSQL mit gepinnten Versionen, Health Checks, persistenten Volumes und möglichst Nicht-Root-Containern. PostgreSQL ist standardmäßig nicht am Host veröffentlicht. TLS wird für Produktion über einen Reverse Proxy terminiert.
