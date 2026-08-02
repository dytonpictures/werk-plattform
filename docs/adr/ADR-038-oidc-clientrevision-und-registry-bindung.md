# ADR-038 – OIDC-Clientrevision und Registry-Bindung

**Status:** Angenommen  
**Datum:** 2026-08-01

## Entscheidung

- OIDC-Endpunkte, Client-ID, Redirect-URI, feste WERK-Audience,
  Authentifizierungsmethode und eine
  optionale Secret-Materialreferenz gehören Core Identity, nicht der globalen
  Provider-Registry.
- Konfigurationen sind unveränderliche, versionierte Datensätze. Eine aktive
  föderierte Anmeldebindung verweist auf genau eine Revision und genau eine
  installationsgebundene OIDC-Login-Capability der Registry.
- Jede Nutzung lädt Binding und Konfiguration und löst anschließend im selben
  PostgreSQL-Snapshot den vollständigen Registry-Vertrag frisch auf. Es gibt
  keine automatische Wahl des ersten oder einzigen Providers.
- Eine Konfigurationsrevision gilt entweder für `work` oder `admin`. Die
  Audience stammt weder aus Browserparametern noch aus OIDC-Claims und wird in
  der verschlüsselten Ceremony bis zum Callback unverändert gebunden.
- Die Datenbank speichert niemals ein Client-Secret, sondern nur ein opakes,
  exakt versioniertes `securematerial.MaterialRef`. Die Identity-Runtime darf
  Konfiguration und Binding lesen, aber nicht verändern.
- Ein vorhandenes Binding bedeutet nur „registriert“. Anmeldebereitschaft darf
  erst nach erfolgreicher Adapter-, Konfigurations-, Secret- und Registry-
  Prüfung gemeldet werden.
- Die Accountbindung speichert für OIDC nicht das nackte `sub`, sondern einen
  domänenseparierten SHA-256-Bezeichner über den exakten kanonischen
  `issuer + NUL + sub`-Wert. Ein Issuerwechsel unter demselben Provider-Key
  kann dadurch keine vorhandenen Subject-Bindungen übernehmen.
