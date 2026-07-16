# ADR-0014: Same-Origin-Architektur und CSRF-Schutz

- **Status:** Accepted
- **Kontext:** Cookie-Sitzungen benötigen Schutz gegen fremde Ursprünge.
- **Entscheidung:** Web und API erscheinen unter derselben Origin; mutierende Requests prüfen CSRF/Origin.
- **Begründung:** Reduziert CORS-Komplexität und Angriffsfläche.
- **Alternativen:** Getrennte Origins mit strengem CORS.
- **Positive Folgen:** Einfacheres Cookie- und Sicherheitsmodell.
- **Negative Folgen:** Reverse-Proxy-Routing ist erforderlich.
- **Sicherheitsauswirkungen:** Proxy-Header, erlaubte Origins und sichere Methoden werden strikt validiert.
- **Überprüfung:** Wenn getrennte Clients öffentlich unterstützt werden.
