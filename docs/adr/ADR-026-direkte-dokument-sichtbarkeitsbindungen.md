# ADR-026 – Direkte Dokument-Sichtbarkeitsbindungen

**Status:** Angenommen
**Datum:** 2026-07-22

## Kontext

Der erste Work-Leseschnitt von Core Documents begrenzt Dokumente absichtlich
auf `created_by_account_id` des authentifizierten Arbeitskontos. Diese Regel
verhindert eine tenantweite Sichtbarkeit, unterstützt aber noch keine bewusst
geteilten Dokumente.

Plattformrollen können eine Operation grundsätzlich erlauben. Sie dürfen nicht
zugleich zum fachlichen Share-Speicher werden: Rollen und Berechtigungen gehören
Core Identity, während die Sichtbarkeit eines Dokuments Core Documents gehört.
Eine generische ACL in Documents würde umgekehrt eine zweite
Autorisierungsplattform aufbauen.

## Entscheidung

### Direkte Kontobindung als dokumentlokale Regel

Core Documents besitzt die Tabelle
`werk_core.document_account_visibility_bindings`. Eine aktive Bindung verbindet
genau ein Dokument mit genau einem normalen Work-Konto desselben Tenants.
Gruppen, Organisationseinheiten, Allow-/Deny-Regeln und frei definierbare
Principals sind nicht Bestandteil von Version 1.

Ein Dokument ist für ein Arbeitskonto lokal sichtbar, wenn das Konto entweder
sein Ersteller ist oder eine nicht widerrufene direkte Bindung besitzt. Bei
einer redundanten Bindung gewinnt defensiv der Zugriffsgrund
`created-by-me`; die Datenbank lehnt solche Selbstbindungen bereits ab.

### Zwei unabhängige Zugriffsgates

Eine Bindung ist keine Plattformberechtigung.

1. Die Liste erfordert weiterhin `core.documents.document.list` auf der
   tenantgebundenen Collection.
2. Das Detail erfordert weiterhin `core.documents.document.read` auf dem
   konkreten Dokument.
3. Freigabe und Widerruf verwenden künftig
   `core.documents.document.visibility-manage` auf dem konkreten Dokument.
4. Nach der Plattformprüfung wertet Core Documents seine eigene aktive
   Bindung neu aus.

Damit macht weder ein tenantweites Rollenrecht ein fremdes Dokument sichtbar,
noch verleiht eine Dokumentbindung Update-, Versions- oder Downloadrechte.

### Lifecycle und Datenhoheit

Eine Bindung wird aktiv angelegt und genau einmal irreversibel widerrufen. IDs,
Tenant, Dokument, Empfänger, Grantor und Erstellzeit bleiben unveränderlich. Ein
erneutes Teilen nach Widerruf erzeugt eine neue Zeile. Physisches Löschen und
Un-Revoke sind unzulässig; PostgreSQL bleibt die historische Wahrheit.

Version 1 erlaubt eine Bindung ausschließlich durch den Ersteller des aktiven
Dokuments an ein anderes aktives Work-Konto desselben Tenants. Dadurch können
Empfänger nicht weiterteilen. Technisch erzeugte Dokumente benötigen später
einen ausdrücklichen Owner-/Steward-Vertrag und erhalten keine implizite
Abkürzung.

### Audit, Ereignisse und Schreibfreigabe

Die unveränderlichen Verträge
`core.documents.document-visibility-granted.v1` und
`core.documents.document-visibility-revoked.v1` registrieren die spätere
fachliche Wirkung. Grant beziehungsweise Widerruf, strukturierter Audit-Eintrag
und Outbox-Ereignis müssen atomar in derselben tenantgebundenen
Service-Transaktion gespeichert werden.

Solange dieser Application-Service-Producer noch fehlt, bleibt die öffentliche
Schreibschnittstelle geschlossen. Der erste Ausbauschritt darf Bindungen lesen
und ihre DB-Invarianten prüfen, aber keinen scheinbar funktionsfähigen
Teilen-Button ausliefern.

### API-Vertrag und Rollout

Die Work-Metadaten-API hebt ihren Vorabvertrag mit diesem Leseschnitt auf
`0.3.0` an. `DocumentSummary.access_reason` wird verpflichtend und der
`visibility_scope` lautet nun `created-or-directly-shared-with-me`. API und
Dashboard werden deshalb als eine atomare Plattformversion ausgerollt;
generierte Clients müssen vor dem Rollout aus dem neuen OpenAPI-Vertrag neu
erzeugt werden. Ein alter Client darf den neuen Scope nicht stillschweigend als
reine Erstelleransicht interpretieren. Die strikte Scope-Prüfung im Dashboard
schlägt bei einem gemischten Rollout bewusst geschlossen fehl.

## Konsequenzen

- Liste und Detail können eigene sowie direkt geteilte Dokumente sicher
  projizieren, ohne Account-, Binding- oder Grantor-IDs auszugeben.
- Ein Widerruf wirkt für jede neue PostgreSQL-Lesetransaktion sofort; Cache und
  Realtime dürfen nur invalidieren.
- Work darf Bindungen lesen, aber nicht schreiben. Service darf sie anlegen und
  widerrufen, jedoch nicht löschen. Admin und Worker erhalten keinen Zugriff.
- Gruppen- und Organisationsfreigaben benötigen einen späteren eigenen
  Vertrag; sie werden nicht durch polymorphe V1-Spalten vorweggenommen.
- Der spätere Bytepfad prüft Plattformpermission, aktive Dokumentbindung und
  Transfer-/Blobzustand erneut.

## Verworfene Alternativen

### Resource-Rollen als Share-Speicher

Verworfen, weil Identity dadurch fachliche Dokumentfreigaben besitzen und Core
Documents in fremde Tabellen schreiben müsste.

### Generische Dokument-ACL

Verworfen, weil Principal-Arten, Allow/Deny, Vererbung und JSON-Regeln eine
zweite, schwer prüfbare Autorisierungswelt erzeugen würden.

### Tenantweite Sichtbarkeit

Verworfen, weil Tenant-RLS nur die Mandantengrenze schützt und keine
dokumentlokale fachliche Sichtbarkeit ersetzt.
