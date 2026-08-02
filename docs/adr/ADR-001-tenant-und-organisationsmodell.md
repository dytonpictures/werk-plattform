# ADR-001: Tenant- und Organisationsmodell

**Status:** angenommen · **Datum:** 2026-07-19 · **Präzisiert:** 2026-07-29

## Entscheidung

Ein `tenant` ist die oberste Daten- und Sicherheitsgrenze einer WERK-
Installation. Im primären Self-Hosted-Betriebsmodell entspricht ein Tenant dem
gesamten Unternehmen. Gesellschaften, Standorte, Bereiche und Teams sind
`organizational_units` innerhalb dieses Tenants.

Das aktuelle Produkt- und UX-Startprofil ist **Single-Company**: Eine
Installation soll regulär genau einen operativen Tenant führen, den die
Produktoberfläche als **Unternehmen** bezeichnet. Ist genau ein aktiver Tenant
und kein weiterer Verzeichniseintrag vorhanden, wird dieser serverseitig
bestätigte Kontext automatisch gewählt und nicht als freier Mandantenwähler
dargestellt. Das Startprofil bietet weder einen regulären Wechsel zwischen
Unternehmenswelten noch das Anlegen eines zweiten Unternehmens in derselben
Installation an. Einen bereits abweichenden oder unbekannten Bestand behandelt
die Oberfläche jedoch defensiv, ohne die technische Mehrmandantenfähigkeit
stillzulegen. Eine harte serverseitige Mengenbegrenzung ist mit diesem ADR noch
nicht entschieden.

```text
Installation
└── Unternehmen                         // Produktsprache
    = Tenant                            // Core und technische Datenisolation
    └── Organizational Unit             // Standort, Bereich, Abteilung, Team
```

`tenant`, `tenant_id` und die zugehörigen Grenzen bleiben die verbindliche
technische Sprache in Core, Datenbank, API, RLS, Audit, Ereignissen, Jobs,
Caches, Dateien und Sicherheitsnachweisen. Die Produktsprache ändert weder IDs
noch Verträge und schwächt keine Tenant-Prüfung ab.

Eine `OrganizationalUnit` des Typs Gesellschaft ist nur dann eine innere Einheit
desselben Unternehmensraums, wenn sie bewusst dieselbe Daten-, Sicherheits- und
Administrationsgrenze teilt. Eine eigenständig zu isolierende Unternehmenswelt
ist keine Abteilung oder Gesellschaftseinheit. Sie würde einen eigenen Tenant
beziehungsweise eine eigene Installation benötigen und gehört nicht zum
aktuellen Single-Company-Profil.

`organization_id` ist kein Ersatz für `tenant_id` und keine eigenständige
Sicherheitsgrenze. Fachliche Datensätze tragen `tenant_id` oder sind über eine
unveränderbare Elternreferenz daran gebunden; Organisationseinheiten begrenzen
fachliche Zuständigkeit und Sichtbarkeit zusätzlich.

## Folgen

- RLS, APIs, Caches, Suchindex, Dateien, Jobs und Ereignisse verwenden
  `tenant_id` als Isolationskontext.
- Eine spätere SaaS-Installation kann mehrere Kunden als getrennte Tenants
  führen, ohne das Fachmodell zu ändern.
- Das Single-Company-Startprofil verwendet in der Produktoberfläche
  „Unternehmen“ sowie darunter „Organisationseinheit“, „Bereich“, „Abteilung“,
  „Standort“ und „Team“. Ein Mandantenwähler ist im bestätigten regulären
  Ein-Unternehmen-Bestand nicht Teil der Nutzerführung; ein defensiver
  Mehrfachbestands-Fallback bleibt erhalten.
- Organisationsbezogene Verwaltungsrechte sind Workspace-Rollen. Sie machen ein
  Arbeitskonto niemals zu einem `admin`-Konto.
- Ein Wechsel zu mehreren Tenants pro Self-Hosted-Kundeninstallation benötigt
  ein neues ADR und Sicherheits-/Migrationsprüfung.

Diese Präzisierung ändert das Sicherheits- oder Datenmodell nicht und benötigt
daher kein neues ADR. Erst die tatsächliche Aktivierung mehrerer isolierter
Tenants in einer Self-Hosted-Kundeninstallation öffnet die letzte Folge zur
Neubewertung.
