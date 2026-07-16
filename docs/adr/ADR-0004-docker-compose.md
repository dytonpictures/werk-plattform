# ADR-0004: Docker Compose für Entwicklung und Self-Hosting

- **Status:** Accepted
- **Kontext:** Eine Installation soll auf einer normalen VM laufen.
- **Entscheidung:** Docker Compose orchestriert Web, API und PostgreSQL.
- **Begründung:** Reproduzierbar und ohne Kubernetes betreibbar.
- **Alternativen:** Native systemd-Dienste, Kubernetes.
- **Positive Folgen:** Einfache lokale und produktionsnahe Umgebung.
- **Negative Folgen:** Begrenzte Cluster-Orchestrierung.
- **Sicherheitsauswirkungen:** Gepinnte Images, minimale Rechte und interne Datenbankports.
- **Überprüfung:** Wenn Hochverfügbarkeit zwingend wird.
