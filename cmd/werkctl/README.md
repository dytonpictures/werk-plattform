# werkctl – Command- und Runner-Grenze

`werkctl` bündelt explizite Betriebsbefehle, ist aber kein allgemeiner Shell-,
Docker- oder `systemd`-Wrapper. Jeder Befehl besitzt einen typisierten Runner
mit injizierbarem Kontext sowie getrennten Ein-/Ausgabeströmen. Der konkrete
Runner trägt seine Klassifikation `read-only` oder `mutation`, die Hilfe liest
sie direkt aus. Diese Klassifikation strukturiert den Dispatch; sie ersetzt
keine technische Privilegien-, Authentisierungs- oder Autorisierungsgrenze.

## Aktuelle Befehle

- `version` gibt nur die Build-Kennung des lokalen `werkctl`-Binärs aus.
- `doctor` ist eine read-only Betriebsinventur. Die Checks haben stabile IDs
  und die Zustände `PASS`, `WARN` und `FAIL`. `--json` liefert denselben mit
  `schema_version: "v1"` gekennzeichneten Vertrag maschinenlesbar.
  `--config-only` öffnet keine Netzwerkverbindungen.
- `status` liest ausschließlich `GET /meta`, `GET /health/live` und
  `GET /health/ready` einer explizit mit `--url` angegebenen WERK-API. Eine
  Listener-Adresse aus der Serverkonfiguration ist weder eine kanonische
  Betriebs-URL noch zwingend ein gültiges TLS-Ziel und wird deshalb nicht
  geraten.
- `migrate` ist der bereits vorhandene, ausdrücklich als Mutation markierte
  Wartungsbefehl. Er verwendet nur die Migrator-Datenbankverbindung.

Für `doctor` und `status` gilt der stabile Exitvertrag:

- `0`: alle ausgeführten Checks sind gesund,
- `1`: kein Fehler, aber mindestens eine Warnung,
- `2`: mindestens ein Fehler oder ungültiger Aufruf.

`status` belegt nur, dass das Ziel eine WERK API v1 ist, der HTTP-Prozess
antwortet und der öffentliche Readiness-Vertrag erfolgreich ist. Readiness
belegt aktuell die vom API-Router geprüfte Work-PostgreSQL-Verbindung. Der
Befehl behauptet ausdrücklich nichts über Identity-/Admin-Datenbank, Worker,
Kafka, Queue-Rückstände, Auditexport, Migrationen, Backups, Host, Container,
Hochverfügbarkeit oder Updates. Dafür wird auch keine Admin-Sitzung erzeugt und
`/admin/v1/operations/summary` nicht aufgerufen.

Der HTTP-Prober folgt keinen Redirects, erlaubt Klartext-HTTP nur zu Loopback,
verwendet mindestens TLS 1.2 und prüft Serverzertifikate gegen den Truststore
des Betriebssystems. Es gibt bewusst weder `--insecure` noch derzeit Flags für
eine zusätzliche private CA oder eine mTLS-Clientidentität. Eine mTLS-API oder
eine private CA, die nicht im System-Truststore installiert ist, kann deshalb
noch nicht per `status` beziehungsweise Doctor-`--url` geprüft werden und wird
als fehlgeschlagene TLS-Verbindung gemeldet. Eine spätere Unterstützung muss
CA-, Client-Zertifikat- und Client-Key-Dateien explizit und gemeinsam
validieren; sie darf die normale Zertifikats- oder Hostnamenprüfung nie
abschalten.

## Grenze für spätere Betriebs-Runner

Start, Stop, Neustart, Update, Rollback und Hybrid-Cloud-Pairing sind noch keine
werkctl-Befehle. Sie dürfen nicht als Shell-Aufruf in `doctor`, `status` oder
einem allgemeinen Runner ergänzt werden. Vor einer Implementierung benötigen
sie mindestens:

1. einen versionierten, typisierten und idempotenten Command-Vertrag,
2. eine getrennte, minimal privilegierte Ausführungsidentität,
3. starke Authentisierung, serverseitige Autorisierung und Audit,
4. erlaubte Ziel- und Zustandsübergänge statt freier Befehle,
5. für Updates signierte Release-Metadaten, Vorprüfungen und eine bestätigte
   Rollbackstrategie,
6. für Pairing einen eigenen Vertrauens-, Schlüssel-, Mandanten- und
   Widerrufsvertrag.

Ein späterer Mutations-Runner bleibt von den read-only Probe-Interfaces
getrennt. Er darf weder Datenbank-Secrets an Unterprozesse weiterreichen noch
Tenant-, Konto-, Policy- oder Auditgrenzen umgehen.
