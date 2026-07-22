# ADR-012: Fachliche Freigaben, Kontrollpunkte und Just-in-Time-Rechte

- **Status:** Angenommen
- **Datum:** 2026-07-21
- **Betrifft:** Core Identity, Autorisierung, Workflows, Worker, Audit und Fachmodule

## Kontext

Fachanwendungen müssen folgenreiche Vorgänge an kontrollierten Punkten anhalten
können. Beispiele sind Vertragsfreigaben, Zahlungen, Stammdatenänderungen oder
eine von einem Service vorgeschlagene Aktion. Die Entscheidung ist fachlich und
darf deshalb weder von einem Plattformadministrator noch allein durch einen
technischen Worker getroffen werden.

Gleichzeitig sollen Fachmodule keine eigenen, voneinander abweichenden Systeme
für Freigaben, temporäre Rechte, Re-Authentifizierung und Audit aufbauen.

## Entscheidung

WERK stellt eine fachneutrale Capability für dauerhafte
`ApprovalCheckpoint`s bereit. Ein Fachmodul definiert Anlass, Richtlinie,
benötigte Berechtigung und fachliche Wirkung. Der Core besitzt Zustand,
Entscheidungen, Sicherheitsnachweise, Audit und Ereignisverträge.

Ein Kontrollpunkt wird nicht als wartender Prozess im Arbeitsspeicher gehalten.
Der auslösende Command speichert den Zustand `pending` und das zugehörige
Outbox-Ereignis atomar in PostgreSQL. Nach einer gültigen Entscheidung erzeugt
der Core erneut ein Ereignis; ein idempotenter Worker oder das besitzende
Fachmodul setzt den Vorgang fort.

## Verbindliche Sicherheitsgrenzen

1. Fachliche Freigaben werden ausschließlich von `work`-Konten erteilt.
   `admin`-Konten verwalten Installation, Plattform und Betrieb, haben aber kein
   Entscheidungsrecht über Kunden- oder Unternehmensfachlichkeit.
2. Eine fachliche Leitungsrolle wie Betriebsleitung oder Geschäftsführung ist
   eine tenantgebundene `work`-Rolle. Dieselbe Person darf ein separates
   `admin`-Konto besitzen; dessen Sitzung und Rechte sind hierfür wertlos.
3. Kontoarten, API-Bereiche und Sessions bleiben getrennt. Ein fachlicher
   Kontrollpunkt ist nur über die Work API entscheidbar. Ein
   Just-in-Time-Recht kann die Zugriffsebene niemals wechseln.
4. Jede Operation benötigt einen expliziten `tenant_id`, eine serverseitige
   Policy-Prüfung und einen Ressourcenumfang. Ein Client darf Kontoart, Rolle
   oder Mandant nicht selbst behaupten.
5. Ein Benutzer kann sich keine Berechtigung selbst verleihen. Eine Richtlinie
   bestimmt, welche Subjekte für eine zeitlich begrenzte Aktivierung berechtigt
   sind. Wo Trennung der Pflichten gefordert ist, darf der Antragsteller seine
   eigene Aktion nicht freigeben.
6. Freigabeentscheidung, Audit-Eintrag und Outbox-Ereignis werden atomar in
   PostgreSQL gespeichert. UI-Zustand, Cache oder Echtzeitnachricht sind nie die
   fachliche Wahrheit.

## Re-Authentifizierung und Just-in-Time-Grant

Eine Freigabepolicy kann vor der Entscheidung eine aktive
Re-Authentifizierung verlangen. Zulässige Verfahren und deren notwendiges
Assurance-Niveau werden von Core Identity bestimmt, beispielsweise WebAuthn,
MFA oder eine erneute Passwortprüfung.

Nach erfolgreicher Prüfung darf der Core einen `JustInTimeGrant` ausstellen. Er
ist gebunden an:

- genau einen Tenant und ein `work`-Konto,
- eine konkrete Berechtigung,
- den exakten Kontrollpunkt beziehungsweise die exakte Ressource,
- eine Richtlinienversion und den Re-Authentifizierungsnachweis,
- eine kurze Ablaufzeit und vorzugsweise genau eine Verwendung.

Der Grant wird nach Verwendung atomar verbraucht. Ablauf, Widerruf und Verbrauch
werden auditiert. Er erweitert keine allgemeine Rolle und verleiht insbesondere
keine fachlichen Rechte an `admin`- oder `service`-Konten.

## Mehrpersonen- und Notfallverfahren

Die Policy kann mehrere unabhängige Entscheidungen sowie Maker-Checker-Regeln
verlangen. Der Checkpoint wird erst `approved`, wenn alle Bedingungen derselben
Policy-Version erfüllt sind. Widersprüchliche oder verspätete Entscheidungen
werden fail-closed abgewiesen.

Ein Notfallzugriff verwendet ein gesondertes, tenantgebundenes
`work`-Notfallkonto mit starker MFA, engem Ressourcenumfang, kurzer Laufzeit,
Pflichtbegründung und nachgelagerter Prüfung. Das `admin`-Konto ist kein
Break-Glass-Ersatz.

## API- und Ereignisgrenze

Fachliche Kontrollpunkte liegen unter `/api/v1/approval-checkpoints` in der Work
API. Eventuelle betriebliche Plattformfreigaben verwenden eigene
`/admin/v1/...`-Ressourcen, eigene Policies und eigene Ereignistypen. Zwischen
beiden Bereichen gibt es keine austauschbaren Tokens oder Freigaben.

Der versionierte Capability-Vertrag ist in
[`docs/APPROVAL-CHECKPOINTS.md`](../APPROVAL-CHECKPOINTS.md) beschrieben.

## Folgen

- Fachmodule können dieselbe belastbare Freigabemechanik verwenden, ohne ihre
  fachliche Datenhoheit aufzugeben.
- Autorisierung, Re-Authentifizierung, temporäre Rechte und Audit werden
  konsistent und zentral prüfbar.
- Worker bleiben horizontal skalierbar; menschliche Wartezeit bindet keinen
  Prozess und keine Queue-Nachricht.
- Die Umsetzung benötigt persistente Zustandsautomaten, Idempotenz,
  Optimistic Concurrency und ablaufende Sicherheitsnachweise.
- Ein universelles Administrationskonto mit fachlicher Allmacht wird bewusst
  ausgeschlossen.

## Nicht entschieden

Dieses ADR legt weder konkrete Genehmigungshierarchien noch fachliche
Schwellenwerte fest. Diese gehören versioniert in die Policy beziehungsweise in
das besitzende Fachmodul.
