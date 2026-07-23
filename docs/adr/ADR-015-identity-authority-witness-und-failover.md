# ADR-015 – Identity-Autorität, Witness und kontrollierter Failover

**Status:** Angenommen  
**Datum:** 2026-07-21

## Kontext

Core und Core Identity sollen zunächst als einzelne Installation zuverlässig
betrieben werden können. Später soll eine zweite Instanz den Betrieb übernehmen
können, wenn die bisherige Hauptinstanz ausfällt. Dabei sind zwei verschiedene
Skalierungsfälle zu unterscheiden:

1. Mehrere API-, Core- oder Identity-Prozesse verwenden dieselbe autoritative
   PostgreSQL-Datenbank. Sperren, Widerrufe und Zähler besitzen dadurch bereits
   eine gemeinsame fachliche Wahrheit.
2. Zwei Instanzen besitzen getrennte Datenbankkopien. Dann entstehen
   Replikationsverzug, Promotion und die Gefahr, dass nach einer Netztrennung
   beide Seiten gleichzeitig Schreibhoheit beanspruchen.

Die fachliche Trennung von Principal, Provider, Credential, Audience und
Zugriffsebene aus
[`ADR-014`](ADR-014-principals-provider-credentials-und-audiences.md) bleibt
unverändert. Dieses ADR ergänzt ausschließlich die betriebliche Autorität über
diesen Zustand.

[`ADR-022`](ADR-022-deploymentprofile-und-platform-witness.md) verallgemeinert
die betriebliche Komponente zum domänengebundenen Platform Witness. Der in
diesem ADR beschriebene Identity Witness ist dessen erste Authority-Domain
`identity-control`; die strengeren Identity-Regeln bleiben unverändert.

Ein Healthcheck kann nur melden, dass ein anderer Prozess nicht erreichbar ist.
Er kann nicht unterscheiden, ob der Prozess ausgefallen oder lediglich die
Netzverbindung unterbrochen ist. Ein automatischer Failover allein aufgrund
eines fehlgeschlagenen Healthchecks würde deshalb Split-Brain ermöglichen.

## Entscheidung

### Eine logische Identity-Autorität

Eine hochverfügbare Installation besitzt genau ein `IdentityRealm` und zu jedem
Zeitpunkt höchstens eine schreibende Identity-Autorität. Die Reserve ist eine
Replik derselben Autorität und keine zweite unabhängige Quelle für Konten,
Providerbindungen, Credentials, Widerrufe oder Audit.

Jede beteiligte Instanz erhält eine unveränderliche `instance_id`; das gemeinsam
verwaltete Realm erhält eine unveränderliche `realm_id`. Eine monoton steigende
`authority_generation` kennzeichnet jede erfolgreiche Übernahme der
Schreibhoheit. Diese Begriffe werden vor einer HA-Implementierung in
Konfiguration, Betriebszustand und relevanten Sicherheitsnachweisen
versioniert eingeführt.

### QDevice-artige Identity-Domain des Platform Witness

Ein automatischer Failover zwischen zwei Instanzen benötigt eine unabhängige
dritte Stimme: die Domain `identity-control` des **Platform Witness**. Sie
entspricht funktional einem QDevice, ist aber ein herstellerneutraler
Plattformvertrag.

Der Witness speichert ausschließlich minimalen Quorumzustand:

```text
PlatformAuthorityLease
  realm_id
  authority_domain = identity-control
  holder_instance_id
  authority_generation
  lease_expires_at
  fencing_token_digest
```

Er speichert keine Tenants, Konten, Rollen, Provider-Subjects, Credential-
Digests, MFA-Secrets, privaten Signaturschlüssel oder Fachdaten. Witness und
beide Instanzen müssen in getrennten Fehlerdomänen betrieben werden und sich
gegenseitig authentifizieren. Lease-Entscheidungen verwenden eine
vertrauenswürdige monotone Zeitbasis des Witness und sind gegen Replay geschützt.

### Lease, Promotion und Fencing

- Die Hauptinstanz erneuert eine kurzlebige exklusive Lease.
- Ein fehlgeschlagener Healthcheck startet nur die Bewertung; er erteilt keine
  Schreibrechte.
- Die Reserve darf erst übernehmen, wenn die alte Lease abgelaufen ist, der
  Witness exklusiv eine höhere `authority_generation` vergeben hat und die
  vorgesehene Replikationsschranke erfüllt ist.
- Kann eine Hauptinstanz ihre Lease nicht rechtzeitig erneuern, verliert sie
  spätestens mit deren Ablauf ihre Schreibbereitschaft. Sie darf dann keine
  Sessions, Credentials, Schlüssel, Providerbindungen oder Widerrufe ausstellen
  beziehungsweise verändern.
- Selbst-Fencing bei Lease-Verlust ist nur die erste Schutzschicht. Vor einer
  Promotion muss eine von der alten Hauptinstanz unabhängige Kontrollgrenze ihr
  Schreiben verhindern, beispielsweise durch Entzug des Datenbank-Writer-
  Zugangs, Storage-/Cluster-Fencing, Netzisolation oder kontrolliertes
  Abschalten. Ein lokales Statusfeld oder Load-Balancer-Umschalten allein reicht
  nicht.
- Nach einer Promotion akzeptieren die neue Autorität und ihre unabhängige
  Schreibgrenze ausschließlich das aktuelle Fencing-Token und die aktuelle
  Autoritätsgeneration. Eine zurückkehrende alte Hauptinstanz bleibt dadurch
  technisch ausgegrenzt und muss sich zunächst als Reserve neu synchronisieren.
- Promotion, Ablehnung, manueller Eingriff und Rückkehr einer Instanz erzeugen
  unveränderliche Security-Audit-Ereignisse.

Ein Load Balancer, DNS-Umschaltung oder Healthcheck ersetzt diese Regeln nicht.

### Betriebsprofile

| Profil | Witness | Automatischer Failover | Schreibmodell |
|---|---:|---:|---|
| Einzelinstanz | aus | nein | lokale Autorität |
| Mehrere Prozesse, eine PostgreSQL-Wahrheit | nicht für Prozessverteilung erforderlich | nur innerhalb des gemeinsamen Datenbank-Betriebsmodells | PostgreSQL serialisiert |
| Zwei replizierte Instanzen | verpflichtend | ja, nach Lease, Generation und Fencing | Active/Passive |
| Zwei Instanzen ohne erreichbaren Witness | nicht verfügbar | nein | manuell und fail-closed |

Der Witness bleibt im heutigen Einzelinstanzprofil vollständig optional. Seine
spätere Aktivierung darf keine neue Kontoart, Audience oder fachliche
Berechtigung erzeugen.

### Fail-closed Policy-Frischeguard

Der kleine interne Vertrag unter `internal/platform/sync` prüft bereits, ob
eine serverseitig aufgelöste Policy-Anfrage exakt zur erwarteten Instanz, zum
Realm, zur `authority_generation` und zur Policy-Revision passt. Danach wird
weiterhin ausschließlich der kanonische Core-Autorisierungsvertrag ausgewertet.
Bei `replicated-active-passive` verlangt der Guard zusätzlich eine noch gültige
Lease und ein verifiziertes Fencing-Token. Abweichung, Ablauf oder Fencing
führen immer zu `deny`.

Dieser Guard ist kein Replikations-, Konsens- oder Failoverdienst und sein
`PolicyRequest` ist kein Credential. Er erwirbt keine Lease, prüft keinen
Healthcheck und überträgt keine Identitäts- oder Policy-Daten. Im
Shared-Database-Profil müssen Policy-Revision und Policy-Eingaben aus derselben
autoritativen PostgreSQL-Sicht stammen.

Die kleine Plattformbasis beschreibt zusätzlich stabile Realm- und
Instanzidentität, kapselt die Auswertung mit einer vertrauenswürdigen lokalen
`AuthoritySnapshotSource` und stellt einen noch nicht gerouteten
`platform.instance-health.v1`-Liveness-Handler bereit. Dessen Antwort enthält
keine Realm-ID, Policy-Revision, Autoritätsgeneration, Lease- oder
Fencing-Aussage. Ein Quellen- oder Kontextfehler erzeugt bereits im Client eine
explizite Deny-Entscheidung. Remote-HTTP-/Witness-Transport, gegenseitige
Authentifizierung, Replay-Schutz, Lease-Erwerb und Promotion bleiben Teil des
späteren HA-Betriebsprofils; Änderungen vorbehalten.

### Degradierter Betrieb

Bereits ausgestellte kurzlebige Nachweise dürfen während einer begrenzten,
konfigurierten Störung weiter geprüft werden, sofern ihr jeweiliger
Verifikationsvertrag, Generation, Audience, Tenant-Bindung, Gültigkeit und
lokale Widerrufssicht dies erlauben. Bei signierten Nachweisen gehört die
Signaturprüfung zwingend dazu. Nach Ablauf dieser Frist wird geschlossen
abgelehnt. Neue sicherheitsrelevante Nachweise oder Änderungen benötigen eine
gültige Authority-Lease.

Klartext-Credentials werden niemals repliziert. Erforderliche private
Schlüssel und MFA-Schlüsselringe werden ausschließlich verschlüsselt und nach
einem eigenen Rotations- und Wiederherstellungsvertrag an die Reserve
übertragen. Öffentliche Prüfschlüssel dürfen kontrolliert gecacht werden.

### Fortgesetzter Failoverbetrieb und Wiederaufnahme

Nach einer kontrollierten Promotion muss die neue aktive Instanz auch dann
weiterarbeiten können, wenn die ausgefallene Plattforminstanz noch nicht
zurückgekehrt ist. Die abwesende Reserve allein ist kein Grund, den bereits
übernommenen Betrieb wieder zu sperren. Voraussetzung bleiben eine erneuerbare
Witness-Lease, wirksames Fencing der alten Instanz, die bei der Promotion
nachgewiesene Replikationsschranke und verfügbare kritische Abhängigkeiten. Fällt
dagegen auch der Witness aus, darf die aktive Instanz nur innerhalb ihres noch
gültigen Lease-Vertrags schreiben; ein Healthcheck verlängert diese Autorität
nicht. Diese Regel gilt unabhängig davon, ob die Fehlerdomänen in zwei Clouds
oder in einem hybriden Betrieb liegen.

Eine zurückkehrende Instanz wird niemals allein durch Erreichbarkeit wieder zur
Reserve oder Autorität. Sie bleibt gefenced und durchläuft einen
Wiederaufnahme-Check:

- Sind erforderliche WAL-/Replikationsjournale, Schlüsselstände,
  Autoritätsgeneration und Schema kompatibel, darf sie kontrolliert nachziehen.
- Überschreitet die Abwesenheit ein konfiguriertes Prüfintervall `X`, erfolgt
  keine automatische Wiederaufnahme. Der Betreiber beziehungsweise spätere
  Recovery-Controller entscheidet anhand nachweisbarer Replikations- und
  Integritätsdaten zwischen inkrementellem Nachziehen und vollständigem
  Neuaufbau der Reserve.
- Fehlen benötigte Journale, wurden relevante Schlüsselrotationen verpasst,
  besteht ein unbekannter Divergenzzustand oder ist die Integrität nicht
  beweisbar, wird die Reserve aus einer verifizierten aktuellen Quelle neu
  aufgebaut. Ein alter Datenstand wird nicht blind zurückgespielt.
- Eine Datenbankschema-Migration ist nur erforderlich, wenn sich während der
  Abwesenheit die kompatible Software-/Schemaversion geändert hat. Eine lange
  Offline-Zeit allein ist noch keine Schema-Migration; meistens handelt es sich
  um Resynchronisierung oder Re-Seeding.

`X` wird nicht als willkürlicher Zeitwert festgeschrieben. Es muss vor dem
HA-Betrieb aus WAL-/Journal-Aufbewahrung, Schlüsselrotation, Widerrufsfenstern,
RPO/RTO, maximaler Aufholzeit und getesteter Schemaversionierung abgeleitet
werden. Erst nach erfolgreicher Daten-, Schlüssel-, Generation- und
Integritätsprüfung erhält die zurückgekehrte Instanz den Zustand
`reserve-ready`; Änderungen vorbehalten.

### Zähler und Replikationskonsistenz

Das heutige atomare API-Schlüssel-Nutzungslimit ist exakt über alle Prozesse,
die dieselbe autoritative PostgreSQL-Datenbank verwenden. Zwei getrennte,
gleichzeitig beschreibbare Datenbankkopien werden ausdrücklich nicht
unterstützt.

Ein automatischer Failover mit exakt fortgeführten Sicherheitszählern verlangt
entweder nachgewiesen verlustfreie Replikation bis zur bestätigten
Schreibposition oder ein später definiertes konservatives Reservierungsmodell.
Kann diese Schranke nicht nachgewiesen werden, erfolgt kein automatischer
Identity-Failover. Verfügbarkeit darf ein zugesichertes Limit oder einen
Widerruf nicht stillschweigend aufweichen.

## Bewusste Nicht-Ziele

- kein Active/Active- oder Multi-Primary-Identity-Betrieb,
- keine Föderation unabhängiger Installationen durch den Witness,
- kein Speichern von Identity- oder Fachdaten im Witness,
- keine Übernahmeentscheidung allein durch Liveness oder Readiness,
- noch keine Implementierung von Replikation, Lease-Dienst oder automatischer
  Promotion im Single-Host-Startprofil.

## Folgen

Das heutige Datenmodell bleibt auf eine autoritative PostgreSQL-Wahrheit
ausgerichtet. Eine zweite Instanz kann später als betriebsbereite Reserve
hinzukommen, ohne die Identity-Hoheit zu verdoppeln. Dafür werden vor dem ersten
automatischen HA-Betrieb ein Bedrohungsmodell, Replikations- und
Schlüsselverfahren, interne Authority-Statusverträge, Failover-Runbooks sowie
Tests für Netztrennung, veraltete Repliken, Witness-Ausfall und Rückkehr der
alten Hauptinstanz benötigt.

Der Witness erhöht die Sicherheit des Zwei-Instanz-Betriebs, ist aber selbst
eine kritische kleine Kontrollkomponente. Er benötigt Härtung, Monitoring,
gesicherte Wiederherstellung und eine Fehlerdomäne außerhalb der beiden
Plattforminstanzen.

## Neubewertung

Dieses ADR wird vor Beginn des HA-/Mehrinstanz-Betriebsprofils konkretisiert.
Dabei werden Replikationstechnologie, Lease-Dauer, maximaler Offline-Zeitraum,
Fencing-Mechanismus, Schlüsselverwahrung, RPO/RTO und manuelle
Notfallprozeduren verbindlich festgelegt und getestet.
