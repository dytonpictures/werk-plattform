# Backend-Implementierungsstand

**Stand:** 2026-07-22  
**Geltungsbereich:** Core-Verträge, Persistenz, Plattformdienste, APIs,
Ereignisse, Sicherheit, Betrieb und Backend-Prüfungen

## Zweck

Diese Übersicht trennt Architekturabsicht, vorhandenen Code und tatsächliche
Produktionsreife. Ein dokumentierter Vertrag gilt nicht automatisch als
implementiert; ein implementierter Baustein gilt ohne Betriebs- und
Sicherheitsnachweis nicht als produktionsfähig. UI-Arbeiten werden in diesem
Backend-Arbeitsstrang nicht geplant oder bearbeitet.

## Reifegrade

```text
geplant
  -> dokumentiert
  -> Vertrag festgelegt
  -> implementiert
  -> integriert
  -> sicherheitsgeprüft
  -> produktionsfähig
```

- **Dokumentiert:** Ziel, Grenze und Datenhoheit sind beschrieben.
- **Vertrag festgelegt:** versionierte Typen, Ressourcen, Berechtigungen oder
  Ereignisse sind verbindlich definiert.
- **Implementiert:** der Backend-Code existiert und besitzt lokale Tests.
- **Integriert:** Datenbank, Adapter und Prozessgrenzen sind gemeinsam geprüft.
- **Sicherheitsgeprüft:** Missbrauchs-, Mandantentrennungs-, Rotations- und
  Fehlerfälle sind nachgewiesen.
- **Produktionsfähig:** Betrieb, Monitoring, Backup, Recovery, Upgrade und
  Rollback erfüllen das freigegebene Produktionsprofil.

## Aktueller Stand

| Backend-Baustein | Reifegrad | Vorhanden | Nächste verbindliche Grenze |
|---|---|---|---|
| Identity und getrennte Zugriffsebenen | integriert | Konten, Sessions, atomare Passwort-/MFA-Sessionrotation, API-Keys, Audiences, Provider-Bindings | Re-Authentifizierung und vollständige Credential-Lebenszyklen |
| Native TLS-/mTLS-Servergrenze | sicherheitsgeprüft | direkter Go-TLS-Server, mTLS, vollständige Ablehnung teilweise ungültiger CA-Bundles, Rotation, HSTS, vertrauenswürdige Proxy-Netze | PKI-Ausstellung, Sperrung und produktiver Zertifikatsbetrieb |
| Organisation und Tenancy | Vertrag festgelegt | Tenant-Kontext, Parteien und erste Organisationseinheiten | Hierarchie-, Abteilungs- und delegierte Verwaltungsverträge |
| Policy und Ressourcen | implementiert | globale Ressourcen, App-Zugriff und erste Policy-Entscheidungen | persistente Gruppen-/Abteilungsregeln, explizite App-Freigaben und Konfliktregeln |
| Audit und Ereignisse | integriert | Security-Audit, Outbox, Kafka-Export und Tagging | Aufbewahrung, Reconciliation, SIEM-Vertrag und Betriebsalarme |
| Service-/Provider-Registry | implementiert | versionierte Dienst-, Capability-, Provider- und Binding-Verträge mit Scope-/Tenant-Auflösung | erster typisierter Domänenverbraucher sowie auditierte Verwaltungsabläufe |
| Konfiguration, Secrets, Keys und Zertifikate | Vertrag festgelegt | getrennte Certificate-, SigningKey- und Secret-Ports sowie bestehender MFA-Keyring und dateibasierter TLS-Pfad | least-privilege Registry-Reader, providerlokale Konfiguration und erster nativer TLS-Verbraucher |
| Dokumente und Storage | Leseschnitt integriert | Dokument-/Blob-Typen, Migration, direkte Kontosichtbarkeit und tenantgesicherte Work-Metadaten-API | atomarer Grant-/Revoke-Service mit Audit/Outbox, danach Objektprovider, Streaming, Reconciliation, Backup und Recovery |
| Jobs, Aufgaben und Workflows | geplant | Outbox-Worker als erste Laufzeitbasis | Job-/Scheduler-Vertrag vor fachlichen Workflows |
| Suche und Projektionen | geplant | keine produktive Suchprojektion | tenantgesicherter Index-, Rebuild- und Löschvertrag |
| Benachrichtigungen und Integrationen | geplant | keine gemeinsame Providergrenze | Notification-, Webhook- und Zustellvertrag |
| Realm, Instanzen und Platform Witness | dokumentiert | validierte Sync-Typen und lokale Liveness | persistente Registry, mTLS-Identität, Lease, Generation, Fencing und Rejoin |
| Produktionsprofil | dokumentiert | Release-Images und Entwicklungs-/Testprofile | eigenes Profil ohne Entwicklungsfallbacks, sichere Secrets und Rotationstests |

## Aktuelle Backend-Reihenfolge

1. Eigenes Produktionsprofil ohne Entwicklungsfallbacks bereitstellen.
2. Certificate-, Key- und Secret-Provider auf die globale Registry-Basis
   aufsetzen.
3. Job-/Scheduler-Vertrag als Grundlage für Dokument-, Storage- und
   Workflow-Verarbeitung bauen.
4. Dokument-/Storage-Provider mit begrenztem Streaming und Reconciliation
   integrieren.
5. Erst nach diesen Grundlagen den echten Platform-Witness-Transport und
   Mehrinstanz-Failover aktivieren.

## Session-Rotationsinvariante

Ein erfolgreicher Passwortwechsel und die erste TOTP-Aktivierung widerrufen in
derselben PostgreSQL-Transaktion alle aktiven Sitzungen des betroffenen Kontos
und stellen genau eine neue interaktive Sitzung mit passender Assurance aus.
Die HTTP-Schicht rotiert Session- und CSRF-Cookie gemeinsam; alle vorherigen
Token sind danach ungültig. Jede erfolgreiche Rotation schreibt das
Security-Audit-Ereignis `identity.session.rotated.v1`, ohne Session-Rohwerte zu
protokollieren. Eine persistente `session_generation` bindet Sitzungen und
MFA-Challenges an den beim Authentifizieren beobachteten Sicherheitsstand des
Kontos. Dadurch kann ein vor der Rotation begonnener Login nach deren Commit
keine alte Sitzung mehr veröffentlichen. Beim Passwortwechsel überschreitet die
Ersatzsitzung die absolute Ablaufzeit der Ursprungssitzung nicht; die frisch
bestätigte TOTP-Aktivierung beginnt eine neue Multi-Factor-Sitzungslaufzeit.
Der verbindliche Entscheid steht in
[`ADR-024`](adr/ADR-024-sessionrotation-und-sicherheitsgeneration.md).
Änderungen vorbehalten.

## Definition of Done für Backend-Bausteine

Ein Baustein ist erst abgeschlossen, wenn:

- Datenhoheit und Tenant-Grenze dokumentiert sind,
- öffentliche Typen, Ressourcen, Berechtigungen und Ereignisse versioniert
  sind,
- verbindliche Änderung und Outbox/Audit atomar gespeichert werden,
- Eingabegrößen, Zeitlimits und Parallelität begrenzt sind,
- negative Sicherheits- und Wiederholungsfälle getestet sind,
- native Unit-/Integrationstests erfolgreich sind,
- Migrations-, Upgrade-, Backup- und Recovery-Auswirkungen benannt sind,
- das passende Produktions- und Monitoringprofil existiert.

Die Reifegrade werden nach jedem abgeschlossenen Backend-Schritt aktualisiert;
Änderungen vorbehalten.
