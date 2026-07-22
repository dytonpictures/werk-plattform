# ADR-022 – Deploymentprofile und domänengebundener Platform Witness

**Status:** Angenommen  
**Datum:** 2026-07-22

## Kontext

Die Plattform unterscheidet bereits mit `WERK_ENV` zwischen `development`,
`test` und `production`. Diese Laufzeitumgebung sagt nichts darüber aus, ob eine
Installation auf einem Host, in zwei Clouds oder hybrid betrieben wird. Ebenso
ist der Aufstellungsort keine Aussage darüber, wie die einzige schreibende
Plattformautorität koordiniert und gegen Split-Brain geschützt wird.

ADR-015 hat den QDevice-artigen Witness zunächst für Identity beschrieben. Das
gleiche Sicherheitsproblem entsteht später auch bei anderen exklusiven
Kontrolloperationen, etwa Schlüsselrotation, plattformweiter Policy-Verwaltung,
Schema-Migration und Recovery-Promotion. Ein Witness pro Fachfunktion oder ein
Witness-Zugriff für jede normale Benutzeranfrage würde dagegen unnötige
Abhängigkeiten und einen zentralen Engpass erzeugen.

## Entscheidung

### Drei getrennte Achsen

Die Plattform modelliert drei voneinander unabhängige Angaben:

```text
RuntimeEnvironment
  development | test | production

DeploymentProfile
  single | dual-cloud | hybrid

AuthorityCoordination
  local | shared-database | platform-witness
```

- `RuntimeEnvironment` steuert Sicherheitsvalidierung, Entwicklungshelfer und
  Logging. Sie wird weiterhin ausschließlich über `WERK_ENV` gesetzt.
- `DeploymentProfile` beschreibt Fehlerdomänen und Aufstellungsorte. Es erteilt
  keine Schreibhoheit und verspricht allein noch keine Hochverfügbarkeit.
- `AuthorityCoordination` beschreibt, wodurch die einzige schreibende
  Kontrollautorität serialisiert und gefenced wird.

Für alle Profile gilt unverändert die Invariante **single writer**. `dual-cloud`
und `hybrid` bedeuten niemals Active/Active- oder Multi-Primary-Identity.

### Deploymentprofile

| Profil | Bedeutung | Erste Zielgrenze |
|---|---|---|
| `single` | eine betriebliche Fehlerdomäne | heutiges Single-Host-/Single-VM-Profil |
| `dual-cloud` | mindestens zwei getrennte Cloud-Fehlerdomänen | Active/Passive mit unabhängiger dritter Stimme |
| `hybrid` | lokale und Cloud-Fehlerdomäne | Active/Passive mit kontrollierter Daten- und Netzgrenze |

Die Begriffe beschreiben die logische Installation. Mehrere Prozesse auf einem
Host erzeugen kein `dual-cloud`-Profil. Ebenso macht eine zweite VM ohne
Replikation, Witness und Fencing noch kein aktiviertes HA-Profil.

OCI-Container bleiben das portable Anwendungsartefakt. Docker Compose bleibt das
Entwicklungs- und erste Single-Profil. Ein unprivilegierter Proxmox-LXC darf als
wegwerfbarer Lauffähigkeitsnachweis dienen, ist aber keine Abnahme der späteren
Cloud-/Hybrid-Isolation. Für produktive getrennte Fehlerdomänen werden gehärtete
VMs beziehungsweise gleichwertige hardwarevirtualisierte Grenzen bevorzugt.
Kubernetes, k3s oder ein anderer Orchestrator werden dadurch nicht vorab
verpflichtend.

### Authority-Koordination

- `local`: genau eine Instanz besitzt die lokale Autorität; es existiert kein
  automatischer Mehrinstanz-Failover.
- `shared-database`: mehrere Prozesse verwenden dieselbe autoritative
  PostgreSQL-Wahrheit; Transaktionen, Sperren und atomare Revisionen
  serialisieren den Zustand.
- `platform-witness`: getrennte Datenkopien beziehungsweise Fehlerdomänen
  benötigen eine exklusive, kurzlebige Lease, monotone Generation und extern
  wirksames Fencing.

Ein Deploymentprofil aktiviert keine Authority-Koordination automatisch. Vor
einer produktiven Aktivierung validiert die Plattform eine dokumentierte
Kompatibilitätsmatrix. Der heutige ausführbare Standard bleibt:

```text
RuntimeEnvironment   = development
DeploymentProfile    = single
AuthorityCoordination = local
```

### Domänengebundener Platform Witness

Der bisherige Identity Witness wird zu einem kleinen **Platform Witness**
verallgemeinert. Identity ist seine erste Authority-Domain und bleibt der erste
implementierte Anwendungsfall. Der Witness speichert pro Domain ausschließlich
minimalen Quorumzustand:

```text
PlatformAuthorityLease
  realm_id
  authority_domain
  holder_instance_id
  authority_generation
  lease_expires_at
  fencing_token_digest
```

Die erste versionierte Domain ist `identity-control`. Sie umfasst die für
Identity-Ausstellung und -Änderung erforderliche Schlüsselautorität, damit eine
Instanz nicht Credentials ohne die dazu passende Signatur- und
Verschlüsselungshoheit ausstellt. Weitere mögliche Domains werden erst mit
einem konkreten Verbraucher registriert:

- `policy-control`,
- `migration-control`,
- `recovery-control`,
- später gegebenenfalls `storage-control`.

Eine neue Domain benötigt einen versionierten Vertrag für geschützte Aktionen,
Abhängigkeiten, Lease-Dauer, Fencing-Grenze, Auditereignisse, degradierte
Weiterarbeit und Wiederaufnahme. Freie, vom Request gelieferte Domainnamen sind
unzulässig.

Normale tenantgebundene Dokument-, Workflow-, Aufgaben- oder Fachoperationen
verwenden weiterhin PostgreSQL-Transaktionen, RLS, Policy, Audit und Outbox. Sie
rufen nicht pro Anfrage den Platform Witness auf. Auch Kafka-Consumergruppen,
gewöhnliche Worker-Claims und idempotente Jobs erhalten keine Witness-Lease,
solange PostgreSQL oder der jeweilige Infrastrukturvertrag sie sicher
koordiniert.

### Failover und Rückkehr

Die Regeln aus ADR-015 gelten für jede aktivierte Authority-Domain:

- Health und Readiness melden nur Beobachtungen und vergeben keine Autorität.
- Promotion verlangt gültige Witness-Lease, höhere Generation, bestätigte
  Replikationsschranke und Fencing der bisherigen Autorität.
- Die übernehmende Instanz arbeitet ohne die ausgefallene Reserve weiter,
  solange Lease und Sicherheitsvoraussetzungen gültig bleiben.
- Eine zurückkehrende Instanz bleibt bis zur geprüften Synchronität gefenced.
- Nach dem domänenspezifischen Prüfintervall `X` wird explizit zwischen
  Nachsynchronisierung und verifiziertem Neuaufbau entschieden.

Die Wiederaufnahme einer Domain darf abhängige Domains nicht in einen
widersprüchlichen Zustand versetzen. Bevor mehrere Domains gleichzeitig
aktiviert werden, benötigt die Plattform deshalb einen eigenen Vertrag für
atomare beziehungsweise hierarchische Übernahmegruppen.

### Sicherheitsgrenze

Der Platform Witness speichert keine Tenants, Konten, Rollen, Credentials,
Schlüssel, Dokumente, Policies oder Fachdaten. Er ist kein Key Server und kein
allgemeiner Konfigurationsdienst. Kommunikation benötigt gegenseitige
Authentifizierung, Replay-Schutz, vertrauenswürdige Zeit und eine vom
Lease-Inhaber unabhängige Fencing-Grenze. Die native TLS-/mTLS-Grundlage ist in
[`ADR-023`](ADR-023-native-server-tls-und-transportidentitaet.md) festgelegt;
die konkrete Instanz-/Realm-Zuordnung sowie Remoteprotokoll, Replay-Schutz und
Fencing werden vor der ersten HA-Aktivierung separat implementiert und getestet.

## Bewusste Nicht-Ziele

- kein Active/Active- oder Multi-Primary-Betrieb,
- kein Kubernetes-Zwang,
- kein eigenes Betriebssystem in der aktuellen Ausbaustufe,
- keine Witness-Abhängigkeit für jede Business-Anfrage,
- keine produktive Aktivierung von `dual-cloud`, `hybrid` oder
  `platform-witness` allein durch einen Konfigurationswert,
- noch kein Lease-, Replikations-, Promotion- oder Remote-Transportdienst.

## Folgen

Der kleine Sync-Vertrag darf Deploymentprofil, Authority-Koordination und
Authority-Domain bereits als validierte Typen führen und veraltete oder falsch
zugeordnete Snapshots fail-closed ablehnen. Das ist weiterhin nur ein
Sicherheitsvertrag und keine HA-Implementierung.

Das Single-Profil und der LXC-Pilot bleiben klein. Cloud-/Hybridbetrieb kann
später VMs, unveränderliche Host-Images und einen Orchestrator ergänzen, ohne
OCI-Artefakte oder fachliche Core-Verträge neu zu entwerfen. Konkrete
Hypervisor-, Cloud-, Betriebssystem-, Datenbankreplikations- und
Orchestratorentscheidungen benötigen eigene Abnahmetests; Änderungen
vorbehalten.
