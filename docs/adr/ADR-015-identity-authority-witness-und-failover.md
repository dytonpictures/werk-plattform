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

### QDevice-artiger Identity Witness

Ein automatischer Failover zwischen zwei Instanzen benötigt eine unabhängige
dritte Stimme: den **Identity Witness**. Er entspricht funktional einem
QDevice, ist aber ein herstellerneutraler Plattformvertrag.

Der Witness speichert ausschließlich minimalen Quorumzustand:

```text
IdentityAuthorityLease
  realm_id
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
