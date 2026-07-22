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
| Identity und getrennte Zugriffsebenen | integriert | Konten, Sessions, MFA, API-Keys, Audiences, Provider-Bindings | Sessionrotation, Re-Authentifizierung und vollständige Credential-Lebenszyklen |
| Native TLS-/mTLS-Servergrenze | sicherheitsgeprüft | direkter Go-TLS-Server, mTLS, Rotation, HSTS, vertrauenswürdige Proxy-Netze | PKI-Ausstellung, Sperrung und produktiver Zertifikatsbetrieb |
| Organisation und Tenancy | Vertrag festgelegt | Tenant-Kontext, Parteien und erste Organisationseinheiten | Hierarchie-, Abteilungs- und delegierte Verwaltungsverträge |
| Policy und Ressourcen | implementiert | globale Ressourcen, App-Zugriff und erste Policy-Entscheidungen | persistente Gruppen-/Abteilungsregeln, explizite App-Freigaben und Konfliktregeln |
| Audit und Ereignisse | integriert | Security-Audit, Outbox, Kafka-Export und Tagging | Aufbewahrung, Reconciliation, SIEM-Vertrag und Betriebsalarme |
| Service-/Provider-Registry | geplant | einzelne Adapter besitzen eigene Konfiguration | gemeinsamer versionierter Registrierungs-, Capability- und Lifecycle-Vertrag |
| Konfiguration, Secrets, Keys und Zertifikate | dokumentiert | MFA-Keyring und dateibasierter Zertifikatsprovider | Provider-Schnittstellen, Metadatenregistry, Rotation und Sperrung |
| Dokumente und Storage | Vertrag festgelegt | Dokument-/Blob-Typen, Migration und Transfergrundlage | echter Objektprovider, Streaming, Reconciliation, Backup und Recovery |
| Jobs, Aufgaben und Workflows | geplant | Outbox-Worker als erste Laufzeitbasis | Job-/Scheduler-Vertrag vor fachlichen Workflows |
| Suche und Projektionen | geplant | keine produktive Suchprojektion | tenantgesicherter Index-, Rebuild- und Löschvertrag |
| Benachrichtigungen und Integrationen | geplant | keine gemeinsame Providergrenze | Notification-, Webhook- und Zustellvertrag |
| Realm, Instanzen und Platform Witness | dokumentiert | validierte Sync-Typen und lokale Liveness | persistente Registry, mTLS-Identität, Lease, Generation, Fencing und Rejoin |
| Produktionsprofil | dokumentiert | Release-Images und Entwicklungs-/Testprofile | eigenes Profil ohne Entwicklungsfallbacks, sichere Secrets und Rotationstests |

## Aktuelle Backend-Reihenfolge

1. MFA- und Passwort-Sessionrotation atomar abschließen.
2. Eigenes Produktionsprofil ohne Entwicklungsfallbacks bereitstellen.
3. Service-/Provider-Registry als kleinen globalen Core-Vertrag festlegen.
4. Certificate-, Key- und Secret-Provider auf diesen Vertrag aufsetzen.
5. Job-/Scheduler-Vertrag als Grundlage für Dokument-, Storage- und
   Workflow-Verarbeitung bauen.
6. Dokument-/Storage-Provider mit begrenztem Streaming und Reconciliation
   integrieren.
7. Erst nach diesen Grundlagen den echten Platform-Witness-Transport und
   Mehrinstanz-Failover aktivieren.

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
