# ADR-001: Tenant- und Organisationsmodell

**Status:** angenommen · **Datum:** 2026-07-19

## Entscheidung

Ein `tenant` ist die oberste Daten- und Sicherheitsgrenze einer WERK-
Installation. Im primären Self-Hosted-Betriebsmodell entspricht ein Tenant dem
gesamten Unternehmen. Gesellschaften, Standorte, Bereiche und Teams sind
`organizational_units` innerhalb dieses Tenants.

```text
Installation
└── Tenant (Unternehmen / Datenisolation)
    └── Organizational Unit (Gesellschaft, Standort, Bereich, Team)
```

`organization_id` ist kein Ersatz für `tenant_id` und keine eigenständige
Sicherheitsgrenze. Fachliche Datensätze tragen `tenant_id` oder sind über eine
unveränderbare Elternreferenz daran gebunden; Organisationseinheiten begrenzen
fachliche Zuständigkeit und Sichtbarkeit zusätzlich.

## Folgen

- RLS, APIs, Caches, Suchindex, Dateien, Jobs und Ereignisse verwenden
  `tenant_id` als Isolationskontext.
- Eine spätere SaaS-Installation kann mehrere Kunden als getrennte Tenants
  führen, ohne das Fachmodell zu ändern.
- Organisationsbezogene Verwaltungsrechte sind Workspace-Rollen. Sie machen ein
  Arbeitskonto niemals zu einem `admin`-Konto.
- Ein Wechsel zu mehreren Tenants pro Self-Hosted-Kundeninstallation benötigt
  ein neues ADR und Sicherheits-/Migrationsprüfung.
