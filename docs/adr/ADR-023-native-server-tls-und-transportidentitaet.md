# ADR-023 – Native Server-TLS- und Transportidentität

**Status:** Angenommen  
**Datum:** 2026-07-22

## Kontext

Die Plattform darf ihre Transportvertraulichkeit und Dienstauthentifizierung
nicht von einem bestimmten Reverse Proxy, Container-Runtime, Hypervisor oder
Cloud-Angebot abhängig machen. Besonders Verbindungen zwischen Instanzen und
dem in ADR-022 beschriebenen Platform Witness benötigen später eine
verlässliche gegenseitige Authentifizierung. TLS allein darf dabei weder
fachliche Berechtigungen noch eine Authority-Lease oder Schreibhoheit erteilen.

Der heutige API-Prozess ist der erste native Netzwerkserver. Worker und
Migration sind keine Server und dürfen weder Serverzertifikate lesen noch wegen
fehlender Serverzertifikate am Start gehindert werden.

## Entscheidung

### Native Transportmodi

Jeder native Plattformserver verwendet einen expliziten Transportmodus:

```text
disabled | tls | mtls
```

- `disabled` ist ausschließlich eine bewusst konfigurierte Ausnahme für
  Entwicklung, Tests und anderweitig abgesicherte lokale Transportgrenzen.
- `tls` verschlüsselt die Verbindung und authentifiziert den Server.
- `mtls` authentifiziert zusätzlich das Clientzertifikat gegen ein
  ausdrücklich konfiguriertes Client-CA-Bundle.

Der produktive API-Server akzeptiert nur `tls` oder `mtls`. Allgemeine
Nicht-Server-Prozesse laden weiterhin ihre gemeinsame Konfiguration, ohne
Zugriff auf den privaten Schlüssel des API-Servers zu benötigen. Browserzugriff
verwendet regulär `tls`; `mtls` ist für kontrollierte Maschinen- und spätere
Control-Plane-Schnittstellen vorgesehen.

Die native Go-Konfiguration unterstützt mindestens TLS 1.2 und handelt mit
geeigneten Gegenstellen TLS 1.3 aus. Es existiert kein Konfigurationsschalter
zum Abschalten der Zertifikats- oder Hostnamenprüfung.

### Zertifikatsmaterial und Rotation

Die Serverkonfiguration enthält nur Dateireferenzen auf:

- Zertifikatskette,
- zugehörigen privaten Schlüssel,
- im Modus `mtls` ein Client-CA-Bundle.

Private Schlüssel werden weder in PostgreSQL noch in allgemeinen
Konfigurationsobjekten, Images oder Ereignissen gespeichert. Eine spätere PKI,
ein Secret Store oder eine Workload-Identity-Lösung stellt Dateien
beziehungsweise sichere Mounts bereit; der Core hängt nicht von deren Produkt
ab.

Beim Start werden Schlüsselpaar, Leaf-Zertifikat, Gültigkeitszeitraum und das
Client-CA-Bundle fail-closed geprüft. Neue TLS-Handshakes prüfen regelmäßig, ob
alle Materialdateien als vollständiger Satz ersetzt wurden. Nur ein vollständig
lesbarer und gültiger Satz wird für neue Verbindungen übernommen. Eine
teilweise, ungültige oder abgelaufene Ersetzung lässt neue Handshakes scheitern,
bis konsistentes Material bereitsteht. Bereits bestehende Verbindungen behalten
ihren ausgehandelten Zustand und müssen entsprechend ihrer Protokoll- und
Rotationsregeln erneuert werden.

### Transportidentität ist keine Berechtigung

Ein erfolgreich verifiziertes Clientzertifikat beweist zunächst nur eine durch
die konfigurierte Vertrauenskette bestätigte Transportidentität. Darauf folgen
separate Prüfungen:

```text
TLS-/mTLS-Verifikation
  -> Zuordnung zur erwarteten Instanz und zum Realm
  -> Plattform-Policy und Audience
  -> Authority-Generation, Lease und Fencing
  -> fachliche Operation und Audit
```

Die konkrete URI-SAN- beziehungsweise Workload-Identity-Zuordnung wird mit dem
ersten echten Control-Plane-Verbraucher versioniert. Ein Zertifikat allein darf
niemals eine Instanz promoten, eine alte Generation reaktivieren oder eine
Policy-Entscheidung ersetzen.

### HTTP-Sicherheitskontext

Bei direktem TLS bestimmt ausschließlich der erfolgreiche native Handshake den
sicheren HTTP-Kontext. Weitergereichte Protokollheader werden nur berücksichtigt,
wenn die unmittelbare Gegenstelle in einem ausdrücklich konfigurierten
Proxy-Netz liegt. Unvertrauenswürdige Clients können daher nicht durch ein
selbst gesetztes `X-Forwarded-Proto` sichere Cookie-Eigenschaften vortäuschen.

Produktive TLS-Antworten setzen HSTS mit zunächst einem Jahr Laufzeit. Eine
plattformweite Einbeziehung von Subdomains und Preload wird bewusst nicht
vorweggenommen, weil sie nur zusammen mit der DNS- und Zertifikatsverantwortung
des Betreibers sicher entschieden werden kann.

### Ausgehende Verbindungen

Die gleiche Invariante gilt für ausgehende Verbindungen. Produktionszugriffe
auf PostgreSQL verwenden `sslmode=verify-full`, damit Verschlüsselung,
Vertrauenskette und Servername geprüft werden. Kafka besitzt einen getrennten
TLS-/Authentifizierungsvertrag. Zukünftige Instanz-, Witness-, Key- und
Storage-Clients dürfen keine Option zum Überspringen der Zertifikatsprüfung
anbieten.

## Bewusste Nicht-Ziele

- keine eigene Certificate Authority in der Plattform,
- keine Speicherung privater Schlüssel im Core,
- noch kein Platform-Witness- oder Instanz-Remoteprotokoll,
- noch keine verbindliche SPIFFE-/SPIRE-Produktentscheidung,
- keine Ableitung fachlicher Rechte aus Common Name oder Zertifikatsbesitz,
- kein Zwang zu einem Reverse Proxy oder einer bestimmten Laufzeitumgebung.

## Folgen

Der API-Server kann HTTPS und mTLS unmittelbar mit Go bereitstellen und startet
in Produktion nicht mehr mit unverschlüsseltem HTTP. Derselbe kleine
Transportbaustein kann später für getrennte Control-Plane-Listener verwendet
werden. Zertifikatsausstellung, sichere Erstverteilung, Sperrung,
Überwachungsalarme und die versionierte Workload-Identity-Zuordnung bleiben vor
der ersten produktiven Mehrinstanzaktivierung verpflichtende Ausbauschritte;
Änderungen vorbehalten.
