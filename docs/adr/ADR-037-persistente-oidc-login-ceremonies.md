# ADR-037 – Persistente OIDC-Login-Ceremonies

**Status:** Angenommen  
**Datum:** 2026-08-01

## Entscheidung

- Der bestehende OIDC-Adapter wird nicht direkt an HTTP angeschlossen. Sein
  kurzlebiger `state`-, `nonce`- und PKCE-Zustand erhält zuerst einen
  PostgreSQL-basierten `CeremonyPort` unter alleiniger Hoheit von Core Identity.
- PostgreSQL speichert nur den SHA-256-Digest von `state`, die explizite
  Registry-Provider-ID, die exakte Redirect-URI und einen verschlüsselten
  Payload. Nonce und PKCE-Verifier erscheinen niemals im Browser, Audit oder
  Klartext in der Datenbank.
- Verbrauch ist ein atomisches `DELETE ... RETURNING`. Auch ein anschließend
  fehlschlagender Tokenaustausch oder eine beschädigte Ceremony kann deshalb
  nicht wiederholt werden.
- Offene Ceremonies sind installationsweit auf 4096 begrenzt. Anlage und
  begrenzte Bereinigung werden über einen PostgreSQL-Advisory-Lock
  serialisiert; flüchtige In-Process-Zähler sind keine Sicherheitsgrenze.
- Vor erfolgreicher OIDC-Prüfung existieren weder Account- noch Tenant-Kontext.
  Die Tabelle besitzt deshalb keine solche Bindung, ist jedoch per Grants,
  `FORCE ROW LEVEL SECURITY` und eigenen Policies ausschließlich für die
  Identity-Runtime zugänglich.

## Noch nicht freigegeben

Dieser Port allein aktiviert keinen externen Login. Öffentliches Begin und
Callback folgen erst zusammen mit unveränderlicher typisierter
OIDC-Clientkonfiguration, Secret-Port, frischer Registry-Auflösung, exakter
`issuer + sub`-Kontobindung, Sessionausstellung und Security-Audit.
