# ADR-004 – PostgreSQL-Rollen und Tenant-RLS

**Status:** Angenommen  
**Datum:** 2026-07-19

## Kontext

Tenant-Isolation darf nicht ausschließlich davon abhängen, dass jede Query einen
korrekten Filter enthält. Gleichzeitig wäre RLS wirkungslos, wenn API oder Worker
als Superuser, mit `BYPASSRLS` oder als ungeschützter Tabellenbesitzer arbeiten.
Connection Pooling darf außerdem keinen Tenant-Kontext zwischen zwei Vorgängen
übertragen.

## Entscheidung

WERK verwendet getrennte PostgreSQL-Rollen:

| Rolle | Anmeldung | Zweck |
|---|---:|---|
| Bootstrap-Rolle `werk` | ja | Nur lokale Cluster-Einrichtung und Rollenabgleich |
| `werk_owner` | nein | Besitzt Datenbank, Schemas, Tabellen, Funktionen und Policies |
| `werk_migrator` | ja | Kurzlebiger Migrator; darf explizit `SET ROLE werk_owner` ausführen |
| `werk_work_runtime` | ja | Ausschließlich Work-API |
| `werk_admin_runtime` | ja | Ausschließlich zukünftige Admin-API |
| `werk_service_runtime` | ja | Ausschließlich zukünftige Service-API |
| `werk_worker_runtime` | ja | Tenantgebundene Hintergrundarbeit |

Alle Login-Rollen sind `NOSUPERUSER`, `NOCREATEDB`, `NOCREATEROLE`,
`NOREPLICATION` und `NOBYPASSRLS`. Runtime-Rollen besitzen keine WERK-Objekte und
sind weder direkt noch indirekt Mitglied von `werk_owner`. Nur der Migrator hat
eine nicht vererbte, explizit per `SET ROLE` nutzbare Owner-Mitgliedschaft.

Rollen werden in einem privilegierten, idempotenten Bootstrap-Schritt vor den
Schema-Migrationen abgeglichen. Passwörter gehören nicht in Migrationen. Eine
produktive oder gemanagte PostgreSQL-Installation kann dieselben Rollen per DBA
oder Infrastructure as Code bereitstellen.

## RLS-Regeln

Mandantenabhängige Tabellen verwenden sowohl `ENABLE ROW LEVEL SECURITY` als
auch `FORCE ROW LEVEL SECURITY`. Die aktive Tenant-ID wird ausschließlich
innerhalb einer Transaktion gesetzt:

```sql
SELECT set_config('werk.tenant_id', $1::uuid::text, true);
```

Der Wert `true` begrenzt die Einstellung auf die laufende Transaktion. Die
Policy-Funktion behandelt fehlenden Kontext und den nach Transaktionsende
möglichen Leerstring fail-closed:

```sql
NULLIF(current_setting('werk.tenant_id', true), '')::uuid
```

Eine restriktive Tenant-Gate-Policy erzwingt die Tenant-Grenze zusätzlich zu den
permissiven, rollen- und befehlsspezifischen Policies. Dadurch kann eine spätere
permissive Policy die Grenze nicht durch eine logische ODER-Verknüpfung öffnen.

RLS ist nur eine zusätzliche Tenant-Grenze. Authentifizierung, Kontoart,
Berechtigungen, Scopes und fachliche Regeln werden weiterhin vor dem
Datenbankzugriff geprüft. Ein Tenant-Header oder Pfadwert reicht niemals aus, um
den Datenbankkontext festzulegen.

## Go-Zugriff

Runtime-Code erhält keinen exportierten `pgxpool.Pool`. `RuntimeDB` bietet nur
tenantgebundene Lese- und Schreibtransaktionen. Vor dem Callback wird als erste
SQL-Anweisung der transaktionslokale Tenant-Kontext gesetzt. Commit, Rollback,
Abbruch und Panic geben keine Connection mit Tenant-Kontext an den Pool zurück;
ein Release-Hook leert zusätzlich den WERK-Kontext oder verwirft die Connection.

Jede neu geöffnete Runtime-Connection prüft fail-closed:

- den exakt erwarteten Login,
- `NOSUPERUSER` und `NOBYPASSRLS`,
- keine Owner-Mitgliedschaft,
- kein Eigentum an WERK-Tabellen.

## Folgen

- API und Worker können RLS nicht regulär deaktivieren oder umgehen.
- `admin_subjects` und `schema_migrations` sind für Work-Runtime nicht lesbar.
- Runtime erhält weder `TRUNCATE`, `REFERENCES`, DDL noch pauschale Rechte auf
  zukünftige Tabellen; jede Migration vergibt Rechte explizit.
- Globale Worker umgehen RLS nicht. Sie arbeiten später Tenant für Tenant.
- Admin- und Work-Datenbankzugänge bleiben getrennt. Die aktuelle API erhält
  ausschließlich das Work-Credential; ein Admin-Pool wird erst mit Admin-IAM
  eingeführt.
- Backup erhält später eine eigene Rolle und verwendet `row_security=off`, damit
  ein versehentlich gefiltertes Backup fehlschlägt statt unvollständig zu sein.
