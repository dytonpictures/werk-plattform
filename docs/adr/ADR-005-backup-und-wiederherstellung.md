# ADR-005 – PostgreSQL-Backup und Wiederherstellung

**Status:** Angenommen  
**Datum:** 2026-07-19

## Kontext

PostgreSQL ist die fachliche Wahrheit von WERK. Ein Backup darf deshalb weder
durch Row Level Security unbemerkt auf einen Tenant gefiltert werden noch als
unverschlüsselte Zwischen- oder Archivdatei entstehen. Gleichzeitig darf das
Backup-Credential keine Schreib-, DDL-, Owner- oder Superuser-Rechte erhalten.

Ein Restore ist eine bewusst destruktive Betriebsaktion. Er darf nicht gegen
eine laufende oder bereits befüllte WERK-Datenbank ausgeführt werden und muss bei
jedem Fehler vollständig zurückrollen.

## Entscheidung

### Rollen und Vollständigkeit

WERK verwendet zwei getrennte Backup-Rollen:

| Rolle | Anmeldung | Rechte |
|---|---:|---|
| `werk_backup` | ja | Datenbankverbindung und ausschließlich `SET ROLE werk_backup_reader` |
| `werk_backup_reader` | nein | `BYPASSRLS`, aber nur explizites `USAGE` und `SELECT` auf WERK-Schemas und -Objekten |

`werk_backup_reader` ist weder Eigentümer noch Superuser und besitzt keine
Schreib-, DDL-, Replikations- oder Owner-Mitgliedschaft. Neue WERK-Schemas müssen
ihre aktuellen und zukünftigen Tabellen und Sequenzen explizit für diese Rolle
lesbar machen. Eine Katalogprüfung erkennt fehlende Grants.

`pg_dump` verbindet sich als `werk_backup`, wechselt ausschließlich für den Dump
zu `werk_backup_reader` und verwendet die PostgreSQL-Voreinstellung
`row_security=off`. Dadurch ist das Ergebnis entweder vollständig oder der Dump
schlägt fehl; ein still tenantgefiltertes Backup ist nicht zulässig.

### Format und Verschlüsselung

- PostgreSQL-18-Client und -Server verwenden dasselbe Major-Release.
- Der Dump wird im PostgreSQL-Custom-Format direkt in `age` gestreamt.
- Der Backup-Container erhält nur öffentliche `age`-Empfänger, niemals einen
  privaten Wiederherstellungsschlüssel.
- Es gibt keine unverschlüsselte SQL-, Tar- oder Dump-Datei auf Platte.
- Erst ein vollständig erzeugtes Ciphertext-Artefakt wird atomar veröffentlicht;
  daneben wird eine SHA-256-Prüfsumme des Ciphertexts abgelegt.
- Rollenpasswörter, Betreiber-Secrets und private Schlüssel sind kein Bestandteil
  des Datenbankdumps.

### Wiederherstellung

Der Restore verwendet nicht die Backup-Rollen. Eine zuvor eingerichtete
`werk_migrator`-Verbindung wechselt mit `SET ROLE` zu `werk_owner`. Der
Ciphertext wird direkt von `age` nach `pg_restore` gestreamt.

Der Restore-Wrapper verlangt:

1. einen expliziten Bestätigungswert für die konkrete Zieldatenbank,
2. einen lesbaren privaten `age`-Schlüssel als read-only Secret,
3. eine frische Zieldatenbank ohne WERK-Relationen,
4. PostgreSQL Major 18,
5. `--single-transaction` und Abbruch beim ersten Fehler.

Das automatisierte Restore-Verfahren verwendet immer eine isolierte
Wegwerf-Datenbank. Es überschreibt keine Entwicklungs- oder Produktivdaten.

## Nachweis

Der Restore-Test erzeugt mindestens zwei Tenants, Organisationseinheiten und
installationsweite Daten. Er prüft danach:

- exakte Fixture-Daten und Migration-Checksummen,
- Objektbesitz durch `werk_owner`, Grants, Policies sowie `ENABLE` und `FORCE RLS`,
- keine Tenant-Sichtbarkeit ohne Kontext und Isolation mit Tenant-Kontext,
- Fehlschlag mit einer falschen `age`-Identität,
- das Ausbleiben unverschlüsselter Dump-Artefakte.

Jeder Testlauf verwendet einen eindeutigen Compose-Projektnamen und entfernt nur
seine eigenen Container, Netze, Volumes und temporären Dateien.

## Grenzen und Folgen

- Dieser erste Baustein ist ein verschlüsseltes logisches PostgreSQL-Backup. Er
  ersetzt noch kein WAL-/PITR-Konzept für ein späteres engeres RPO.
- Dokumente im künftigen Object Storage benötigen ein getrenntes, konsistent
  koordiniertes Backup. Sie werden nicht vorgezogen, solange der Object Store
  noch nicht Teil des Plattformfundaments ist.
- Der Verlust aller privaten `age`-Identitäten macht das Backup unbrauchbar.
  Recovery-Schlüssel müssen deshalb getrennt und mindestens einmal off-site
  verwahrt werden.
- Nur vertrauenswürdige, der eigenen Installation eindeutig zugeordnete Archive
  dürfen wiederhergestellt werden.
