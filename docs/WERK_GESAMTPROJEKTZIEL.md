# WERK – Gesamtprojektziel

**Dokumentstatus:** Verbindliche strategische Zieldefinition  
**Version:** 1.1  
**Stand:** 19. Juli 2026  
**Zeithorizont:** Produktive Nutzbarkeit und Weiterentwicklung über mindestens zehn Jahre

---

## 1. Projektauftrag

WERK wird als zentrale, selbst hostbare Unternehmensplattform entwickelt. Die Plattform verbindet Menschen, Geschäftsdaten, Dokumente, Entscheidungen und Prozesse in einem einheitlichen digitalen Arbeitsraum.

WERK soll nicht lediglich ein weiteres ERP-System oder eine Sammlung nebeneinanderliegender Fachmodule werden. Das langfristige Ziel ist ein **digitales Unternehmensbetriebssystem**, das operative Arbeit organisiert, Zusammenhänge sichtbar macht, Entscheidungen nachvollziehbar hält und spezialisierte Fachsysteme integrieren oder schrittweise ersetzen kann.

WERK wird zunächst als internes Unternehmenswerkzeug für mindestens 50
Mitarbeitende entwickelt. Kontrollierte externe Testinstanzen dienen Feedback und
Qualitätssicherung, sind aber kein früher Markt- oder SaaS-Start. Jede
Installation muss eigenständig, sicher und vollständig im Unternehmen betrieben
werden können; ein späterer SaaS-Modus bleibt nur eine Architekturperspektive.

---

## 2. Produktvision

> WERK schafft einen gemeinsamen digitalen Arbeitsraum, in dem Menschen, Geschäftsdaten und Prozesse nicht länger auf getrennte Anwendungen verteilt sind. Die Plattform zeigt nicht nur, welche Daten vorhanden sind, sondern auch, was als Nächstes getan werden muss, warum ein Vorgang blockiert ist und welche Auswirkungen eine Entscheidung besitzt.

WERK soll im Arbeitsalltag zur primären Oberfläche des Unternehmens werden. Mitarbeiter erhalten einen rollenbezogenen Arbeitsbereich statt einer unübersichtlichen Ansammlung von Modulen. Geschäftsführung und Führungskräfte erhalten einen nachvollziehbaren Gesamtblick auf Arbeit, Leistung, Risiken und Entscheidungen.

---

## 3. Strategische Ziele

### 3.1 Einheitlicher Unternehmenskontext

WERK verbindet Kunden, Kontakte, Aufträge, Projekte, Aufgaben, Mitarbeiter, Dokumente, Kosten, Risiken und Entscheidungen über nachvollziehbare Beziehungen. Informationen sollen nicht mehrfach und widersprüchlich in verschiedenen Modulen gepflegt werden.

### 3.2 Durchgängige Arbeitssteuerung

Alle Aufgaben, Freigaben, Fristen, Warnungen und Erwähnungen laufen in einer persönlichen, priorisierten Arbeitsbox zusammen. Benutzer müssen nicht mehrere Module durchsuchen, um ihre aktuelle Arbeit zu erkennen.

### 3.3 Kontrollierbare Anpassbarkeit

Unternehmen können Statusmodelle, Formulare, Felder, Freigaben, Benachrichtigungen und ausgewählte Automatisierungen konfigurieren, ohne den Plattformkern zu verändern. Anpassungen bleiben versionierbar, prüfbar und upgradefähig.

### 3.4 Sichere Erweiterbarkeit

Neue Fachmodule, Integrationen und spätere Apps werden über stabile Verträge, Berechtigungen, Ereignisse und definierte UI-Erweiterungspunkte angebunden. Direkte unkontrollierte Zugriffe auf interne Tabellen oder Kernfunktionen sind ausgeschlossen.

### 3.5 Dauerhafte Datenhoheit

PostgreSQL bleibt die maßgebliche Quelle geschäftskritischer Daten. Kunden können ihre Fachdaten, Dokumente, Beziehungen, Historien und Konfigurationen vollständig exportieren. Self-Hosting darf nicht zu einer technischen oder wirtschaftlichen Sackgasse führen.

### 3.6 Langfristige Produktfähigkeit

WERK muss über mindestens zehn Jahre kontrolliert aktualisiert, migriert und funktional erweitert werden können. Fachliche Modelle und öffentliche Verträge werden deshalb von kurzlebigen Framework-Details getrennt.

---

## 4. Zielgruppen und Betriebsmodelle

### Primäre Zielgruppen

- interne Arbeitsbereiche und technische Betreiber des ersten Unternehmens,
- Unternehmensleitungen und Bereichsverantwortliche,
- Verwaltung, Vertrieb, Personal, Projekt- und Betriebsorganisation,
- interne Administratoren und technische Betreiber,
- spätere Partner und Entwickler von Erweiterungen.

### Unterstützte Betriebsmodelle

1. **Dedizierte Self-Hosted-Installation:** Eine eigenständige Installation im Unternehmen oder bei dessen Infrastrukturpartner.
2. **Pilot- und Testinstallation:** Kontrollierte Instanz für Erprobung, Feedback und Verbesserungen.
3. **Späterer SaaS-Betrieb:** Eine mögliche spätere Betriebsform, die erst nach
   eigener Produkt-, Sicherheits- und Betriebsentscheidung aktiviert wird.

Dabei werden drei Ebenen eindeutig unterschieden:

- `installation_id` bezeichnet eine technische Installation,
- `tenant_id` bezeichnet die oberste Daten- und Sicherheitsgrenze und entspricht
  im Self-Hosted-Startprofil dem gesamten Unternehmen,
- `organizational_unit_id` bezeichnet Gesellschaft, Standort, Bereich oder Team
  innerhalb eines Tenants.

---

## 5. Funktionsumfang

### 5.1 Verbindlicher Plattformkern

Der Plattformkern stellt gemeinsame Fähigkeiten für alle Fachbereiche bereit:

- Organisationen, Standorte, Teams und Benutzer,
- Authentifizierung, Sitzungen und Identitätsanbindung,
- Rollen, Berechtigungen und Richtlinien,
- globale Navigation und Suche,
- persönliche Arbeitsbox,
- Aufgaben, Wiedervorlagen, Kommentare und Aktivitäten,
- Dokumente, Anhänge und Versionen,
- Status, Freigaben und kontrollierte Workflows,
- Benachrichtigungen,
- Beziehungen zwischen Geschäftsobjekten,
- Audit- und Entscheidungsprotokoll,
- Ereignisse, Webhooks, Importe und Exporte,
- Modul-, Installations- und Konfigurationsverwaltung,
- technische Betriebs-, Diagnose- und Updatefunktionen.

### 5.2 Erste produktive Fachfunktionen

Die erste produktive Version bildet einen vollständigen vertikalen Arbeitsablauf ab:

1. Organisation und Benutzer einrichten,
2. Kunden und Kontakte verwalten,
3. einen Geschäftsvorgang oder ein Projekt anlegen,
4. Aufgaben, Verantwortliche und Fristen zuweisen,
5. Dokumente und Aktivitäten zuordnen,
6. Entscheidungen oder Freigaben durchführen,
7. Änderungen und Verantwortlichkeiten vollständig auditieren,
8. Vorgänge über Suche und Arbeitsbox wiederfinden und bearbeiten.

### 5.3 Spätere Fachmodule

- CRM und Vertrieb,
- Angebote, Aufträge und Beschaffung,
- Projekt- und Ressourcenmanagement,
- Personalverwaltung,
- Finanzprozesse und Controlling,
- Business Intelligence,
- Risiko- und Betriebskontinuitätsmanagement,
- Anlagen, Inventar, Lager und betriebliche Ressourcen,
- branchenspezifische Erweiterungen,
- Plugin- und App-Ökosystem.

Finanzbuchhaltung, Lohnabrechnung und andere stark regulierte Fachbereiche werden nicht ohne eigene fachliche, rechtliche und revisionsbezogene Spezifikation als vollständig abgedeckt dargestellt. WERK kann diese Bereiche zunächst integrieren und später kontrolliert erweitern.

---

## 6. Bewusste Abgrenzung der ersten Version

Die erste Version ist nicht:

- ein vollständiger Ersatz für sämtliche ERP-, CRM-, HR- und Finanzsysteme,
- eine frei programmierbare Low-Code-Plattform ohne Sicherheitsgrenzen,
- eine Sammlung kundenspezifischer Sonderlösungen im Plattformkern,
- eine Microservice-Landschaft ohne nachgewiesenen Skalierungsbedarf,
- ein KI-Chatfenster ohne kontrollierten Zugriff auf Prozesse,
- ein System, bei dem Cache-, Such- oder Nachrichtendienste die einzige Datenquelle bilden.

Die Breite der Vision wird durch eine kontrollierte Entwicklungsreihenfolge beherrscht. Jede neue Funktion muss entweder eine Kernfähigkeit stärken oder einen vollständigen, messbaren Geschäftsvorgang verbessern.

---

## 7. Architekturziel

### 7.1 Grundarchitektur

WERK startet als **modularer Monolith**. Die Module werden fachlich und technisch voneinander getrennt, jedoch zunächst gemeinsam betrieben und ausgeliefert. Ein Modul wird erst dann zu einem eigenständigen Dienst, wenn Skalierung, Isolation oder unabhängige Veröffentlichung dies nachweislich erfordern.

### 7.2 Technologische Basis

- **Web-Frontend:** Next.js, React und TypeScript,
- **Native Clients:** Kotlin Multiplatform und Compose Multiplatform für
  Android, iOS/iPadOS und spätere Desktop-Ziele,
- **Backend:** Go mit klar getrennten Domain-, Application- und Adapter-Schichten,
- **HTTP-API:** OpenAPI 3.1 als verbindlicher, versionierter Vertrag,
- **Routing:** `chi`,
- **Datenbank:** PostgreSQL mit konsequenter Mandantentrennung und Row-Level Security,
- **Datenzugriff:** `sqlc`,
- **Migrationen:** `goose`,
- **Performance-Dienst:** Valkey über eine Redis-kompatible, austauschbare Go-Schnittstelle,
- **Auslieferung:** reproduzierbare Container und Docker Compose für die erste Betriebsstufe.

Konkrete Framework-Versionen sind austauschbare Ausgangspunkte und keine dauerhaften Produktgrenzen.

Web und native Clients verwenden ausschließlich dieselben versionierten
Business-APIs. Die native Work-App ist ein eigenes installierbares Produkt und
keine PWA oder WebView-Shell. Eine spätere native Administrationsoberfläche
bleibt als getrenntes Artefakt mit eigener Audience und Sessionablage von der
Work-App isoliert. Die Entscheidung und ihre Liefergrenzen sind in
[`ADR-013`](adr/ADR-013-native-clients-kotlin-compose-multiplatform.md) und der
[`Clientarchitektur`](CLIENT-ARCHITEKTUR.md) festgehalten.

### 7.3 Daten- und Ereignisgrundsätze

- PostgreSQL ist die Source of Truth.
- Jede fachliche Tabelle ist eindeutig einem Tenant zugeordnet oder ausdrücklich
  installationsweit definiert. Organisationseinheiten ergänzen fachliche
  Zuständigkeit und Sichtbarkeit, ersetzen aber nie die Tenant-Grenze.
- Kritische Änderungen werden transaktional mit Audit- und Outbox-Einträgen verbunden.
- Fachliche Ereignisse sind versioniert, nachvollziehbar und wiederholbar verarbeitbar.
- Valkey beschleunigt Abfragen und kurzlebige technische Zustände, hält aber keine alleinigen Geschäftsdaten.
- Der Ausfall von Valkey darf höchstens zu reduzierter Leistung, nicht zu fachlichem Datenverlust führen.
- Cache-Schlüssel enthalten Installations- und Organisationskontext sowie eine definierte Lebensdauer.

### 7.4 Erweiterungsmodell

Module und Plugins erhalten nur ausdrücklich freigegebene Fähigkeiten. Das Erweiterungsmodell umfasst langfristig:

- versionierte APIs und Webhooks,
- fachliche Ereignisabonnements,
- isolierte Konfigurations- und Datenbereiche,
- deklarierte Berechtigungen,
- definierte UI-Erweiterungspunkte,
- Kompatibilitätsangaben und Upgrade-Prüfungen,
- signierte und nachvollziehbare Pakete.

---

## 8. Innovationskern

Innovation in WERK bedeutet nicht, möglichst viele Funktionen einzubauen. Sie muss Arbeit reduzieren, Zusammenhänge verständlich machen oder Entscheidungen verbessern.

### 8.1 Einheitliches Geschäftsobjekt-Modell

Zentrale Geschäftsobjekte folgen gemeinsamen Regeln für Identität, Organisation, Status, Verantwortlichkeit, Berechtigungen, Beziehungen, Dokumente, Aktivitäten und Audit. Dadurch funktionieren Suche, Arbeitsbox, Automatisierungen und Erweiterungen modulübergreifend.

### 8.2 Unternehmensgraph

WERK bildet fachliche Beziehungen ausdrücklich ab, beispielsweise:

> Kunde → Angebot → Auftrag → Projekt → Mitarbeiter → Dokument → Rechnung → Risiko

Der Unternehmensgraph ermöglicht kontextbezogene Navigation, Auswirkungsanalysen, Prozessverständnis und spätere Assistenzfunktionen. Für die erste Stufe werden diese Beziehungen kontrolliert in PostgreSQL modelliert; eine zusätzliche Graphdatenbank ist nicht automatisch erforderlich.

### 8.3 Intelligente Arbeitsbox

Die Arbeitsbox führt Aufgaben, Freigaben, Fristen, Risiken und Erwähnungen zusammen und strukturiert sie mindestens nach:

- jetzt bearbeiten,
- Entscheidung erforderlich,
- Frist oder Ziel gefährdet,
- zur Kenntnis,
- automatisch erledigt.

Priorisierungen bleiben erklärbar und vom Benutzer kontrollierbar.

### 8.4 Entscheidungsprotokoll

WERK speichert nicht nur, was geändert wurde, sondern kann bei relevanten Vorgängen auch erfassen:

- warum entschieden wurde,
- wer beteiligt war,
- welche Informationen zugrunde lagen,
- welche Alternativen betrachtet wurden,
- welche Auswirkungen erwartet wurden.

So entsteht ein dauerhaftes, kontrolliertes Unternehmensgedächtnis.

### 8.5 Kontrollierter KI-Akteur

KI-Funktionen werden als kontrollierte Akteure mit eigener Identität, Berechtigung und Audit-Spur behandelt. Sie dürfen nur Daten verwenden, auf die der auslösende Benutzer zugreifen darf.

Mögliche Aufgaben sind:

- Dokumente klassifizieren und zusammenfassen,
- Informationen strukturiert extrahieren,
- Vorgänge, Abweichungen und Risiken erklären,
- Entwürfe und Handlungsvorschläge vorbereiten,
- berechtigtes Unternehmenswissen auffindbar machen,
- wiederkehrende Arbeit erkennen und Automatisierungen vorschlagen.

Kritische Änderungen benötigen eine ausdrückliche Freigabe. Vorschlag, Entscheidung und ausgeführte Aktion bleiben getrennt und nachvollziehbar.

### 8.6 Prozessintelligenz und Simulation

Auf Grundlage fachlicher Ereignisse kann WERK später Engpässe, wiederkehrende Verzögerungen und unnötige Freigabeschritte erkennen. Neue Regeln, Prozesse, Preise oder Rollen sollen vor ihrer Aktivierung in einem abgegrenzten Simulationsmodus überprüfbar werden.

### 8.7 Unternehmens-Zeitmaschine

Für auditpflichtige und entscheidungsrelevante Vorgänge soll nachvollziehbar sein, welchen Zustand Daten und Beziehungen zu einem bestimmten Zeitpunkt besaßen. Eine vollständige Event-Sourcing-Architektur ist dafür nicht zwingend; geeignete Historisierung wird pro fachlichem Bedarf eingesetzt.

### 8.8 WERK-zu-WERK-Kommunikation

Langfristig können eigenständige Installationen ausgewählte Geschäftsobjekte wie Anfragen, Aufträge, Dokumente oder Rechnungsinformationen signiert und nachvollziehbar austauschen, ohne ihre Datenbanken oder internen Netzwerke direkt zu verbinden.

---

## 9. Benutzererlebnis und Bedienprinzipien

### 9.1 Ein System statt vieler Oberflächen

Listen, Detailansichten, Aktivitäten, Aufgaben, Freigaben, Dokumente, Beziehungen und Historien verwenden in allen Modulen dieselben Interaktionsmuster.

### 9.2 Rollenbezogene Arbeitsbereiche

Benutzer sehen die für ihre Rolle, Organisation und aktuelle Arbeit relevanten Funktionen. Die Plattform vermeidet sowohl überladene Modulmenüs als auch eine unkontrolliert wechselnde Oberfläche.

### 9.3 Erklärbarkeit

Status, Prioritäten, Berechtigungsentscheidungen, Automatisierungen und KI-Vorschläge müssen verständlich begründet werden können.

### 9.4 Effiziente Bedienung

Häufige Arbeitsabläufe werden für Tastatur, Maus und geeignete mobile Nutzung optimiert. Globale Suche, Schnellaktionen und konsistente Befehle verkürzen Navigationswege.

### 9.5 Barrierefreiheit

Die Oberfläche wird auf vollständige Tastaturbedienung, nachvollziehbare Fokusführung, ausreichende Kontraste, skalierbare Darstellung und nicht ausschließlich farbbasierte Statuskommunikation ausgelegt.

### 9.6 Kontrollierte Veränderung

Größere UI-Änderungen werden über Nutzungstests, Pilotgruppen und schrittweise Einführung abgesichert. Zehnjährige Nutzbarkeit bedeutet kontinuierliche Verbesserung bei stabilen Grundmustern, nicht das Einfrieren der ersten Oberfläche.

---

## 10. Sicherheits- und Vertrauensziel

Sicherheit ist eine tragende Produkteigenschaft von WERK. Sie wird über den gesamten Lebenszyklus geplant, implementiert, getestet, betrieben und weiterentwickelt. Eine Installation darf nicht allein deshalb als sicher gelten, weil sie im internen Unternehmensnetz betrieben wird.

WERK verfolgt **Security by Design**, **Privacy by Design**, **Secure by Default**, **Least Privilege**, **Defense in Depth** und **Zero Trust zwischen Sicherheitsgrenzen**. Sicherheit umfasst Anwendung, Daten, Identitäten, Mandanten, Plugins, KI-Funktionen, Lieferkette, Infrastruktur, Updates und den organisatorischen Umgang mit Schwachstellen.

### 10.1 Schutzziele und Schutzobjekte

Für jede Funktion werden mindestens folgende Schutzziele betrachtet:

- **Vertraulichkeit:** Daten sind ausschließlich für berechtigte Identitäten und Systeme sichtbar.
- **Integrität:** Änderungen sind autorisiert, nachvollziehbar und gegen unbemerkte Manipulation geschützt.
- **Verfügbarkeit:** Kernprozesse bleiben innerhalb definierter Betriebsziele nutzbar und wiederherstellbar.
- **Authentizität:** Benutzer, Dienste, Erweiterungen und Artefakte können verlässlich identifiziert werden.
- **Nachvollziehbarkeit:** Kritische Aktionen lassen sich eindeutig einer Identität, einem Zeitpunkt und einem Kontext zuordnen.
- **Mandantenisolation:** Eine Organisation kann weder absichtlich noch versehentlich auf Daten einer anderen Organisation zugreifen.
- **Datenhoheit:** Betreiber behalten Kontrolle über Speicherung, Export, Aufbewahrung, Übertragung und Löschung.

Besonders zu schützen sind:

- Identitäten, Sitzungen und Wiederherstellungswege,
- Personen-, Kunden-, Personal- und Finanzdaten,
- Dokumente, Geschäftsgeheimnisse und Vertragsinformationen,
- Berechtigungen, Richtlinien und Administrationsfunktionen,
- Audit-, Entscheidungs- und Ereignisdaten,
- Schlüssel, Zertifikate, Tokens und andere Geheimnisse,
- Backups, Exporte, Support-Bundles und Testdaten,
- Build-, Release-, Update- und Plugin-Lieferketten.

### 10.2 Bedrohungsmodell

Vor Implementierung eines Moduls oder einer kritischen Funktion wird ein dokumentiertes Bedrohungsmodell erstellt oder aktualisiert. Es betrachtet mindestens:

- kompromittierte oder böswillige Benutzerkonten,
- fehlerhafte Berechtigungen und unzulässige Objektzugriffe,
- Ausbruch aus einer Organisation oder Installation,
- Diebstahl, Fixierung oder Wiederverwendung von Sitzungen,
- manipulierte Dateien, Importe, Webhooks und API-Anfragen,
- SQL-Injection, XSS, CSRF, SSRF, Pfadmanipulation und unsichere Weiterleitungen,
- Brute Force, Credential Stuffing, Rate-Limit-Umgehung und Ressourcenerschöpfung,
- kompromittierte Abhängigkeiten, Build-Systeme, Container oder Updatekanäle,
- unsichere oder bösartige Plugins,
- Prompt Injection, Datenabfluss und unkontrollierte Aktionen durch KI-Funktionen,
- Ransomware, Verlust von Schlüsseln, Fehlkonfiguration und Bedienfehler,
- Insider-Risiken und Missbrauch privilegierter Administration,
- ungewollte Datenoffenlegung über Logs, Telemetrie, Backups oder Supportdaten.

Jede identifizierte Bedrohung erhält Schutzmaßnahmen, Tests, Verantwortlichkeit und eine dokumentierte Restrisikobewertung. Kritische Restrisiken dürfen nicht stillschweigend akzeptiert werden.

### 10.3 Sicherheitsstandard und Governance

WERK verwendet als technische Prüfbasis:

- **OWASP ASVS 5.0 Level 2** als Mindestziel für die produktive Webanwendung und API,
- ausgewählte Anforderungen aus **OWASP ASVS Level 3** für Identitäten, Administration, Mandantentrennung, besonders sensible Daten und sicherheitskritische Funktionen,
- **NIST SP 800-218 SSDF** als Rahmen für den sicheren Softwareentwicklungsprozess,
- geeignete Bausteine des **BSI IT-Grundschutzes** für Entwicklung, Betrieb sowie Identitäts- und Berechtigungsmanagement,
- die Datenschutz-Grundverordnung für die Verarbeitung personenbezogener Daten,
- eine formale Prüfung der Anwendbarkeit und Pflichten des **EU Cyber Resilience Act** vor kommerzieller Bereitstellung.

Diese Orientierung stellt noch keine Zertifizierung oder automatisch erreichte Rechtskonformität dar. Anforderungen werden als versionierte, testbare Security Controls geführt und einer verantwortlichen Rolle zugeordnet.

### 10.4 Identitäten und Authentifizierung

- Jede natürliche Person verwendet eine eigene Identität; gemeinsam genutzte Benutzerkonten sind unzulässig.
- Externe Identitätsanbieter werden über standardisierte Verfahren wie OpenID Connect angebunden. Weitere Enterprise-Verfahren können kontrolliert ergänzt werden.
- Lokale Konten bleiben für Self-Hosting und Notfallzugriff möglich, werden aber besonders geschützt.
- Mehrfaktor-Authentifizierung ist für privilegierte Konten verpflichtend und für alle Benutzer unterstützbar.
- Passkeys werden als phishing-resistente Anmeldeoption vorgesehen.
- Kennwörter werden ausschließlich mit einem aktuellen, dafür geeigneten Passwort-Hashverfahren und individuellen Salts gespeichert. Parameter müssen ohne Datenverlust nachschärfbar sein.
- Wiederherstellungscodes und Reset-Tokens sind einmalig, zeitlich begrenzt und gespeichert nur gehasht oder anderweitig angemessen geschützt.
- Fehlgeschlagene Anmeldungen, Wiederherstellungen und Änderungen von Sicherheitsmerkmalen werden rate-limitiert, erkannt und auditierbar gemacht.
- Servicekonten besitzen eindeutige Identitäten, einen engen Zweck, rotierbare Zugangsdaten und dürfen keine interaktive Anmeldung erben.
- Notfallkonten werden getrennt verwahrt, stark authentifiziert, regelmäßig geprüft und bei jeder Nutzung alarmiert.
- Das Deaktivieren eines Benutzers beendet aktive Sitzungen und entzieht wirksam alle weiteren Zugriffsmöglichkeiten.

### 10.5 Autorisierung und Berechtigungsmodell

WERK kombiniert rollenbasierte und attributbasierte Zugriffskontrolle:

- Rollen beschreiben nachvollziehbare fachliche Verantwortungen.
- Attribute begrenzen Zugriffe beispielsweise nach Organisation, Standort, Team, Eigentümerschaft, Vertraulichkeitsstufe oder Objektstatus.
- Standard ist **verweigern**, solange keine ausdrückliche Berechtigung besteht.
- Berechtigungen werden serverseitig bei jeder geschützten Aktion geprüft. Die Benutzeroberfläche ist keine Sicherheitsgrenze.
- Listen-, Such-, Export- und Statistikabfragen verwenden dieselben Zugriffsregeln wie Detailansichten.
- Besonders kritische Aktionen können Vier-Augen-Freigabe, erneute Authentifizierung oder zeitlich begrenzte Privilegien verlangen.
- Privilegierte Administration wird von fachlicher Bearbeitung getrennt.
- Rollenänderungen, Delegationen und zeitlich begrenzte Rechte besitzen Gültigkeitszeiträume, Begründung und Audit-Spur.
- Ein Benutzer darf sich nicht selbst unkontrolliert zusätzliche Privilegien erteilen oder die eigene Freigabe genehmigen.
- Jede Berechtigungsregel erhält Positiv-, Negativ- und organisationsübergreifende Isolationstests.

### 10.6 Mandanten- und Datenisolation

Mandantentrennung wird mehrfach und unabhängig durchgesetzt:

1. Die authentifizierte Sitzung bestimmt Installations- und Organisationskontext.
2. Die API autorisiert Aktion und Objektzugriff.
3. PostgreSQL Row-Level Security begrenzt den tatsächlich ausführbaren Datenzugriff.
4. Exporte, Suche, Caches, Ereignisse, Dateien und Hintergrundjobs übernehmen denselben Kontext.

Zusätzliche Regeln:

- Ein vom Client übermittelter `organization_id`-Wert ist niemals allein vertrauenswürdig.
- Organisationskontext wird transaktionsgebunden gesetzt; Connection Pooling darf keinen Kontext zwischen Anfragen übertragen.
- Anwendungsrollen erhalten keine reguläre Möglichkeit, RLS zu umgehen.
- Installationsweite Administration und organisationsbezogene Administration werden getrennt modelliert.
- Globale Datensätze sind ausdrücklich gekennzeichnet und dürfen nicht aus fehlendem Mandantenbezug entstehen.
- Objekt-IDs müssen nicht erratbar sein, ersetzen aber niemals eine Autorisierungsprüfung.
- Cache-Schlüssel, Dateipfade, Suchindizes, Job-Payloads und Telemetrie enthalten einen sicheren, validierten Mandantenkontext.
- Automatisierte Isolationstests versuchen bei jeder mandantenfähigen Funktion ausdrücklich den Zugriff auf fremde Daten.

### 10.7 Sitzungen, Cookies und API-Zugänge

- Websitzungen verwenden zufällige, serverseitig widerrufbare Sitzungskennungen.
- Sitzungscookies sind mindestens `Secure`, `HttpOnly` und mit einem geeigneten `SameSite`-Wert gesetzt.
- Sitzungskennungen werden nach Anmeldung, Rechteänderung und sicherheitsrelevanten Ereignissen rotiert.
- Inaktivitäts- und absolute Laufzeiten werden risikobasiert begrenzt.
- Zustandsändernde Browseranfragen besitzen wirksamen CSRF-Schutz.
- CORS wird über eine enge Positivliste konfiguriert und niemals pauschal mit Zugangsdaten freigegeben.
- API-Tokens werden nur mit minimalen Scopes, Ablaufdatum und eindeutigem Besitzer ausgegeben.
- Tokens werden in Protokollen, URLs und Fehlermeldungen konsequent verborgen.
- Widerruf, Schlüsselrotation und Beendigung kompromittierter Sitzungen müssen ohne Neustart der Installation möglich sein.
- Rate Limits werden nach Aktion, Identität, Organisation und technischer Herkunft risikoorientiert angewendet.

### 10.8 Datenschutz, Verschlüsselung und Geheimnisse

- Kommunikation über unsichere Netze wird mit aktuellen TLS-Konfigurationen geschützt.
- Schutz ruhender Daten wird für Datenbank, Dokumente, Backups und Exporte dokumentiert. Die Verantwortungsgrenze zwischen WERK und Betreiber muss eindeutig sein.
- Besonders sensible Felder können zusätzlich anwendungsseitig verschlüsselt werden, sofern Suche, Betrieb und Schlüsselrotation kontrolliert lösbar bleiben.
- Schlüssel und Geheimnisse werden nicht im Quellcode, Container-Image, Repository oder normalen Log gespeichert.
- Geheimnisse werden über Dateien, Secret Stores oder andere dafür vorgesehene Mechanismen injiziert und können ohne Neuinstallation rotiert werden.
- Entwicklung, Test, Pilot und Produktion verwenden getrennte Zugangsdaten und Vertrauensräume.
- Produktivdaten dürfen nicht ungeprüft in Entwicklungs- oder Testumgebungen kopiert werden.
- Datenklassifikation, Zweckbindung, Aufbewahrung und Löschung werden pro Datenkategorie definiert.
- Telemetrie ist datensparsam, transparent, abschaltbar und enthält standardmäßig keine Geschäfts- oder Personendaten.

### 10.9 Anwendungs-, Datei- und API-Sicherheit

- Eingaben werden an Vertrauensgrenzen strukturell und fachlich validiert.
- SQL wird parametrisiert erzeugt; dynamische Abfragebestandteile verwenden kontrollierte Positivlisten.
- Ausgaben werden kontextabhängig kodiert; eine strenge Content Security Policy begrenzt ausführbare Webinhalte.
- Fehlerantworten enthalten eine nachvollziehbare Referenz, aber keine Stacktraces, SQL-Details, Geheimnisse oder interne Pfade.
- Datei-Uploads werden nach Größe, Typ, Inhalt, Erweiterung und Berechtigung geprüft und außerhalb direkt ausführbarer Bereiche gespeichert.
- Aktive oder riskante Dateiformate können isoliert verarbeitet, abgelehnt oder durch einen Malware-Scan geprüft werden.
- Downloadnamen und Content-Header verhindern unbeabsichtigte Ausführung im Browser.
- Serverseitige URL-Abrufe verwenden Protokoll- und Ziel-Positivlisten, blockieren interne Adressräume und verhindern SSRF.
- Importer und Parser erhalten Ressourcenlimits und werden gegen fehlerhafte oder bösartige Eingaben getestet.
- Webhooks verwenden Signaturen, Zeitstempel, Replay-Schutz, Zeitlimits und kontrollierte Wiederholungen.
- API-Limits für Nutzlast, Seitengröße, Komplexität und Laufzeit verhindern unkontrollierte Ressourcenbelegung.

### 10.10 Valkey, Hintergrundjobs und interne Dienste

- PostgreSQL bleibt die einzige maßgebliche Quelle geschäftskritischer Daten.
- Valkey und andere interne Dienste sind nicht direkt aus dem öffentlichen Netz erreichbar.
- Zugriff erfolgt authentifiziert, netzseitig begrenzt und – abhängig vom Betriebsmodell – transportverschlüsselt.
- Cache-Inhalte besitzen TTL, Mandantenkontext und eine dokumentierte Invalidierungsstrategie.
- Cache-Ausfall, Cache Poisoning und veraltete Einträge werden im Bedrohungsmodell berücksichtigt.
- Vertrauliche Inhalte werden nur gespeichert, wenn Zweck, Lebensdauer und Schutz ausdrücklich definiert sind.
- Dauerhafte Jobs und Ereignisse werden transaktional über PostgreSQL und Outbox abgesichert; flüchtiges Pub/Sub darf keinen alleinigen Geschäftszustand tragen.
- Job-Handler prüfen Berechtigung und Mandantenkontext erneut und sind gegen doppelte Ausführung ausgelegt.

### 10.11 Plugin- und Erweiterungssicherheit

- Plugins besitzen ein versioniertes Manifest mit Herausgeber, Version, Kompatibilität und angeforderten Fähigkeiten.
- Installation und Erweiterung erfordern eine verständliche Zustimmung zu den angeforderten Berechtigungen.
- Plugins erhalten keinen direkten Datenbankzugriff und keine pauschale interne API-Berechtigung.
- Zugriffe erfolgen über versionierte, autorisierte Plattformverträge.
- Plugin-Aktionen erscheinen mit Plugin-Identität im Audit-Protokoll.
- Pakete werden signiert, auf Integrität geprüft und auf eine vertrauenswürdige Herkunft zurückgeführt.
- Erweiterungen werden technisch isoliert oder mit nachweisbar wirksamen Grenzen betrieben.
- Netzwerkzugriffe, Dateizugriffe, Hintergrundaufgaben und Geheimnisse benötigen einzeln deklarierte Fähigkeiten.
- Ein kompromittiertes Plugin muss deaktiviert und widerrufen werden können, ohne die gesamte Installation unbrauchbar zu machen.
- Für Updates gelten Kompatibilitäts-, Schwachstellen- und Rollbackprüfungen.

### 10.12 KI-Sicherheit

KI-Funktionen bilden eine eigene Sicherheitsgrenze:

- Jede KI-Anfrage wird im Kontext einer auslösenden Identität und ihrer wirksamen Berechtigungen ausgeführt.
- Abgerufene Dokumente, Webseiten, E-Mails und Plugin-Inhalte gelten als nicht vertrauenswürdige Eingaben und dürfen Systemregeln nicht überschreiben.
- Prompt Injection und indirekte Anweisungen werden in Architektur, Tests und Benutzeroberfläche ausdrücklich berücksichtigt.
- Modelle erhalten keine Datenbestände „auf Vorrat“, sondern nur den minimal erforderlichen, autorisierten Kontext.
- Geheimnisse, Sicherheitstokens und nicht freigegebene Systeminformationen werden nicht an Modelle übertragen.
- Externe KI-Anbieter werden pro Installation ausdrücklich konfiguriert; Datenflüsse, Speicherorte und vertragliche Nutzung müssen transparent sein.
- Unternehmensdaten dürfen nicht ohne ausdrückliche Vereinbarung zum Training externer Modelle verwendet werden.
- Lesen, Vorschlagen, Ändern und externes Ausführen sind getrennte Fähigkeiten.
- Kritische oder irreversible Aktionen benötigen eine menschliche Freigabe und zeigen Ziel, Daten, Auswirkung und Begründung vor Ausführung.
- KI-Ausgaben gelten als untrusted und werden vor Speicherung, Darstellung oder Tool-Nutzung validiert.
- KI-Aktionen sind mit Modell, Richtlinienversion, Quellen, Freigabe und Ergebnis auditierbar, soweit dies datenschutz- und geheimnisschutzkonform möglich ist.

### 10.13 Audit, Protokollierung und Erkennung

Kritische Audit-Ereignisse enthalten mindestens:

- Zeitpunkt und eindeutige Ereigniskennung,
- Installation und Organisation,
- handelnde Benutzer-, Dienst-, Plugin- oder KI-Identität,
- Aktion, Zielobjekt und Ergebnis,
- relevante Berechtigungs- oder Freigabekontexte,
- korrelierbare Anfrage- oder Ablaufkennung,
- begründete administrative Sonderzugriffe.

Audit-Protokolle sind append-orientiert, gegen unbemerkte Veränderung geschützt und nur eng berechtigten Rollen zugänglich. Normale Anwendungslogs dürfen keine Kennwörter, Tokens, Schlüssel oder unnötigen Geschäfts- und Personendaten enthalten.

WERK unterstützt:

- Alarmierung bei verdächtigen Anmeldungen und Privilegienänderungen,
- Erkennung wiederholter Zugriffsverletzungen und ungewöhnlicher Exportmengen,
- Integritätsprüfung kritischer Sicherheitsereignisse,
- kontrollierten Export an externe SIEM- oder Monitoring-Systeme,
- konfigurierbare, gesetzes- und zweckgerechte Aufbewahrungsfristen,
- Zugriff auf Auditdaten nur mit eigener Auditierung.

### 10.14 Sichere Self-Hosting-Infrastruktur

- Öffentliche Angriffsflächen werden minimiert; Datenbank, Valkey und interne Verwaltungsendpunkte bleiben intern.
- Container und Prozesse laufen ohne Root-Rechte, soweit technisch möglich, mit minimalen Linux-Fähigkeiten und restriktiven Dateisystemrechten.
- Images sind minimal, versioniert, reproduzierbar gebaut, gescannt und unveränderlich referenziert.
- Entwicklungs- oder Debugfunktionen sind in Produktion standardmäßig deaktiviert.
- Health- und Metrikendpunkte geben keine Geheimnisse oder unnötigen Systemdetails preis.
- Reverse Proxy, TLS, Sicherheitsheader, Netzwerksegmentierung und vertrauenswürdige Proxygrenzen werden dokumentiert.
- Ressourcenlimits verhindern, dass ein einzelner Prozess oder Import die gesamte Installation unkontrolliert blockiert.
- Administrationszugänge werden getrennt geschützt und sind nicht automatisch aus dem gesamten Netz erreichbar.
- Eine sichere Standardkonfiguration ist ohne zusätzliche Härtung nutzbar; Abweichungen werden sichtbar gewarnt.
- Betreiber erhalten eine Härtungsrichtlinie, eine sichere Beispielkonfiguration und einen automatisierten Konfigurationscheck.

### 10.15 Sichere Softwarelieferkette und Entwicklung

Der Entwicklungsprozess umfasst mindestens:

- Bedrohungsmodell und Security-Anforderungen für sicherheitsrelevante Änderungen,
- verpflichtende Reviews bei Authentifizierung, Autorisierung, Kryptografie, RLS, Parsern, Plugins und Updatefunktionen,
- statische Codeanalyse, Dependency- und Lizenzprüfung sowie Secret Scanning,
- automatisierte Tests gegen typische Web-, API- und Mandantenangriffe,
- Fuzzing für Parser, Importer und besonders kritische Go-Komponenten,
- Software Bill of Materials in einem verbreiteten maschinenlesbaren Format,
- signierte Release-Artefakte, Images und Update-Metadaten,
- dokumentierte Build-Herkunft und reproduzierbare beziehungsweise nachvollziehbare Builds,
- Schwachstellenprüfung von Images und ausgelieferten Abhängigkeiten,
- unabhängigen Penetrationstest vor breitem Produktivbetrieb und nach wesentlichen Sicherheitsänderungen,
- getrennte Rechte für Entwicklung, Build, Signierung und Veröffentlichung,
- geschützte Branches, überprüfte Änderungen und kurzlebige CI-Zugangsdaten.

Eine erfolgreiche Kompilierung oder ein automatischer Scan ersetzt keine fachliche Security-Prüfung.

### 10.16 Schwachstellen- und Vorfallmanagement

WERK benötigt vor dem produktiven Vertrieb:

- einen veröffentlichten, überwachten Meldeweg für Sicherheitslücken,
- eine Responsible-Disclosure- beziehungsweise Vulnerability-Disclosure-Policy,
- ein einheitliches Verfahren für Bewertung, Priorisierung, Behebung und Veröffentlichung,
- eindeutig unterstützte Produktversionen und Security-Updatekanäle,
- die Fähigkeit, betroffene Versionen und Komponenten über SBOM und Releaseinformationen zu bestimmen,
- vorbereitete Abläufe für Schlüsselwiderruf, kompromittierte Releases, Plugin-Sperren und Notfallupdates,
- Vorfallpläne für Datenabfluss, Mandantendurchbruch, Ransomware und kompromittierte Administratoren,
- Beweissicherung, Kommunikationsverantwortung und datenschutzrechtliche Eskalationswege,
- eine rechtliche und organisatorische Vorbereitung auf Meldepflichten, insbesondere aus Datenschutzrecht und gegebenenfalls Cyber Resilience Act.

Kritische Sicherheitsupdates müssen unabhängig von Funktionsreleases bereitgestellt werden können. Der zugesagte Security-Supportzeitraum wird für jede Produktlinie transparent veröffentlicht.

### 10.17 Backup-, Wiederherstellungs- und Ransomware-Schutz

- Backups umfassen Datenbank, Dokumente, notwendige Konfigurationen und Wiederherstellungsinformationen.
- Backups werden verschlüsselt; Schlüssel werden getrennt und wiederherstellbar verwaltet.
- Mindestens eine Sicherungskopie kann gegen Veränderung oder Löschung durch eine kompromittierte Produktividentität geschützt werden.
- Wiederherstellungen werden regelmäßig in einer getrennten Umgebung getestet und dokumentiert.
- Restore-Tests prüfen Datenkonsistenz, Mandantentrennung, Dokumente, Auditdaten und notwendige Schlüssel.
- Sicherungen und Exporte folgen denselben Aufbewahrungs- und Löschregeln wie Produktivdaten.
- Ein Backup gilt erst dann als funktionsfähig, wenn die Wiederherstellung nachgewiesen wurde.

### 10.18 Datenschutz und sichere Datenlebenszyklen

- Personenbezogene Daten werden nach Zweck, Rechtsgrundlage, Sensibilität und Aufbewahrungsfrist klassifiziert.
- Standardkonfigurationen minimieren erhobene und protokollierte Daten.
- Betroffenenrechte, Auskunft, Berichtigung, Export, Einschränkung und Löschung werden technisch unterstützt, soweit keine gesetzlichen Aufbewahrungspflichten entgegenstehen.
- Löschungen berücksichtigen Primärdaten, Suchindizes, Caches, Dokumente, Ableitungen und definierte Backupzyklen.
- Test- und Demodaten sind synthetisch oder wirksam anonymisiert.
- Berechtigte Exporte werden gekennzeichnet, auditierbar erstellt und zeitlich begrenzt bereitgestellt.
- Externe Auftragsverarbeiter und Dienste werden in einem transparenten Datenfluss- und Unterauftragsverarbeitermodell erfasst.

### 10.19 Verbindliche Security-Abnahmekriterien

Eine Funktion ist nicht produktionsbereit, wenn eines der zutreffenden Kriterien fehlt:

1. Schutzbedarf und Bedrohungen sind dokumentiert.
2. Authentifizierung und Autorisierung sind serverseitig umgesetzt.
3. Mandantenkontext wird auf API-, Daten-, Datei-, Cache- und Jobebene geprüft.
4. Positive, negative und organisationsübergreifende Sicherheitstests bestehen.
5. Audit- und Datenschutzverhalten sind definiert und getestet.
6. Eingaben, Dateien, Ausgaben und externe Aufrufe besitzen angemessene Schutzmaßnahmen.
7. Geheimnisse und personenbezogene Daten erscheinen nicht in Logs oder Fehlermeldungen.
8. Abhängigkeiten, Images und Artefakte durchlaufen die festgelegten Lieferkettenprüfungen.
9. Backup-, Migrations- und Wiederherstellungsfolgen sind bewertet.
10. Dokumentation beschreibt sichere Konfiguration, Betrieb und bekannte Restrisiken.
11. Es bestehen keine offenen kritischen Schwachstellen; hohe Risiken benötigen eine ausdrücklich verantwortete, befristete Ausnahme mit Maßnahmenplan.
12. Sicherheitsrelevante Änderungen sind durch eine zweite qualifizierte Person geprüft.

### 10.20 Nicht verhandelbare Sicherheitsregeln

- Keine Sicherheitsentscheidung vertraut allein auf die Benutzeroberfläche.
- Keine organisationsübergreifende Abfrage ohne expliziten, getesteten Systemzweck.
- Keine produktiven Standardkennwörter oder fest eingebauten Schlüssel.
- Keine geschäftskritischen Daten ausschließlich in Valkey, Logs oder flüchtigen Queues.
- Kein Plugin und keine KI-Funktion mit pauschalem Vollzugriff.
- Keine Telemetrie oder Supportübertragung von Geschäftsdaten ohne transparente Zustimmung.
- Keine Sicherheitslücke wird durch Verbergen, stilles Akzeptieren oder eine undokumentierte Sonderlösung behandelt.
- Kein produktives Release ohne rückverfolgbare Herkunft, Security-Prüfung und definierten Updateweg.

### 10.21 Referenzgrundlagen

- [OWASP Application Security Verification Standard](https://owasp.org/www-project-application-security-verification-standard/)
- [NIST SP 800-218 – Secure Software Development Framework](https://csrc.nist.gov/pubs/sp/800/218/final)
- [BSI IT-Grundschutz-Kompendium](https://www.bsi.bund.de/DE/Themen/Unternehmen-und-Organisationen/Standards-und-Zertifizierung/IT-Grundschutz/IT-Grundschutz-Kompendium/it-grundschutz-kompendium_node.html)
- [Verordnung (EU) 2024/2847 – Cyber Resilience Act](https://eur-lex.europa.eu/eli/reg/2024/2847/oj/eng)

---

## 11. Betriebs- und Updateziel

Eine WERK-Installation muss von einem technisch qualifizierten Administrator reproduzierbar installiert, gesichert, aktualisiert, überwacht und wiederhergestellt werden können.

Das Startprofil bleibt eine einzelne Identity-Autorität. Mehrere Prozesse an
derselben PostgreSQL-Datenbank sind Prozessredundanz, keine zweite Autorität.
Ein späteres Active/Passive-Profil mit eigener Datenbankreplik benötigt für
automatischen Failover die Domain `identity-control` eines unabhängigen
QDevice-artigen Platform Witness, Lease, monotone Autoritätsgeneration,
nachgewiesene Replikationsschranke und
extern wirksames Fencing. Healthchecks allein dürfen niemals Schreibhoheit
vergeben. Der verbindliche Zielvertrag steht in
[`ADR-015`](adr/ADR-015-identity-authority-witness-und-failover.md).

Verpflichtende Betriebsfähigkeiten sind:

- dokumentierte Systemvoraussetzungen,
- automatisierbare Installation und Konfiguration,
- Health-, Readiness- und Diagnoseprüfungen,
- strukturierte Logs, Metriken und nachvollziehbare Fehlerkennungen,
- Backup vor risikoreichen Migrationen,
- geprüfte Datenbankmigrationen,
- Vorabprüfung der Update-Kompatibilität,
- klar definierte unterstützte Upgradepfade,
- Support-Bundles ohne unnötige Geschäftsdaten,
- langfristig unterstützte Release-Kanäle,
- Wiederherstellungsübungen und dokumentierte Notfallverfahren.

Updates dürfen keine manuelle Bearbeitung von Datenbanktabellen durch den Kunden voraussetzen.

---

## 12. Qualitäts- und Leistungsziele

### Funktionale Qualität

- Jeder Kernprozess besitzt automatisierte Tests auf Domain-, API- und Integrationsebene.
- Mandanten- und Berechtigungsgrenzen werden durch negative Tests nachgewiesen.
- Migrationen werden gegen realistische Datenstände getestet.
- API und Implementierung dürfen nicht unkontrolliert voneinander abweichen.

### Leistung

- Eine Standardinstallation bedient mindestens 50 regelmäßig aktive Mitarbeiter stabil.
- Für zentrale Arbeitsabläufe werden messbare Performancebudgets und realistische Referenzlasten definiert.
- Optimierungen erfolgen auf Grundlage von Messwerten, Traces und reproduzierbaren Lasttests.
- Cache-Ausfälle und kalte Caches werden ausdrücklich getestet.
- Größere Installationen dürfen durch dieselben fachlichen Verträge auf leistungsfähigere Betriebsformen migrieren können.

### Wartbarkeit

- Fachmodule besitzen klare Verantwortlichkeiten und Abhängigkeitsregeln.
- Dauerhafte Architekturentscheidungen werden als ADR dokumentiert.
- Öffentliche Verträge werden versioniert und besitzen definierte Ablösepfade.
- Abhängigkeiten werden regelmäßig aktualisiert, geprüft und ersetzbar gehalten.
- Kernfunktionen dürfen nicht von einem einzelnen Plugin oder externen Anbieter abhängen.

---

## 13. Entwicklungsprinzipien für Codex und menschliche Entwickler

1. Es wird immer ein vollständiger, überprüfbarer vertikaler Funktionsschnitt gebaut.
2. Keine neue Funktion ohne fachliche Verantwortung, Berechtigungsmodell und Mandantenkontext.
3. Keine Geschäftsfunktion speichert ihren maßgeblichen Zustand ausschließlich im Cache.
4. Keine direkte Kopplung von Fachmodulen über interne Tabellen; Kommunikation erfolgt über freigegebene Verträge.
5. Keine stille dauerhafte Architekturentscheidung; relevante Entscheidungen werden dokumentiert.
6. Keine Attrappen in als produktiv gekennzeichneten Arbeitsabläufen.
7. Jede Änderung umfasst angemessene Tests, Migrationen, Dokumentation und Betriebsfolgen.
8. Sicherheit, Audit, Export und Updatefähigkeit gehören zur Funktion und werden nicht nachträglich ergänzt.
9. Neue Infrastruktur wird nur aufgenommen, wenn sie einen gemessenen oder strategisch zwingenden Nutzen besitzt.
10. Kundenanpassungen erfolgen über Konfiguration, Erweiterungspunkte oder getrennte Module – nicht durch Forks des Plattformkerns.

---

## 14. Entwicklungsreihenfolge

### Phase 1 – Plattformfundament

- Repository-, Build- und Qualitätsstruktur,
- initiales Bedrohungsmodell und versionierter Security-Control-Katalog,
- sichere CI-/Release-Lieferkette mit SBOM, Scans und signierten Artefakten,
- Konfiguration und sicherer Betrieb,
- Organisationen, Benutzer und Sitzungen,
- Rollen, Berechtigungen und RLS,
- Audit, Outbox und grundlegende Ereignisse,
- API-Vertrag und generierter Client,
- gemeinsame UI-Grundmuster.

### Phase 2 – Erster vollständiger Geschäftsvorgang

- Kunden und Kontakte,
- Vorgänge oder Projekte,
- Aufgaben, Verantwortliche und Fristen,
- Dokumente und Aktivitäten,
- Status und Freigaben,
- globale Suche und Arbeitsbox,
- vollständiger End-to-End-Test.

### Phase 3 – Produktiver Pilotbetrieb

- Import und vollständiger Export,
- Backup und Restore,
- Update- und Migrationspfad,
- Monitoring und Support-Bundle,
- Last-, Sicherheits- und Wiederherstellungstests,
- strukturiertes Pilotfeedback und Usability-Auswertung.

### Phase 4 – Erweiterung der Fachdomänen

- CRM-, Projekt-, Personal- und Finanzprozesse nach priorisiertem Kundennutzen,
- konfigurierbare Workflows und Automatisierungen,
- Integrationen und kontrollierte Plugin-Schnittstellen,
- erweiterte Analyse- und Entscheidungsfunktionen.

### Phase 5 – Plattformökosystem

- App- und Plugin-Verteilung,
- SaaS-Betriebsmodell,
- kontrollierte KI-Akteure,
- Prozessintelligenz und Simulation,
- sichere WERK-zu-WERK-Kommunikation.

---

## 15. Messbare Definition des Projekterfolgs

WERK erreicht sein erstes wesentliches Produktziel, wenn:

- eine neue Installation reproduzierbar bereitgestellt werden kann,
- mindestens eine Organisation mit 50 oder mehr Mitarbeitern stabil arbeiten kann,
- Mandanten- und Berechtigungsgrenzen automatisiert nachgewiesen sind,
- die zutreffenden Security-Abnahmekriterien nach OWASP-ASVS-basierter Prüfbasis erfüllt sind,
- keine offenen kritischen Schwachstellen im freigegebenen Produktstand bestehen,
- ein realer Geschäftsvorgang vollständig ohne externe Zwischenlisten durchgeführt wird,
- Aufgaben, Dokumente, Freigaben und Historie in einem einheitlichen Kontext verfügbar sind,
- Standardabfragen und Arbeitsabläufe ihre definierten Performancebudgets einhalten,
- ein Ausfall des Cache-Dienstes keinen fachlichen Datenverlust verursacht,
- Backups erfolgreich in einer getrennten Testumgebung wiederhergestellt wurden,
- eine neue Version über einen dokumentierten und getesteten Upgradepfad eingespielt werden kann,
- sämtliche Kundendaten vollständig und strukturiert exportierbar sind,
- Pilotnutzer ihre wesentlichen täglichen Aufgaben ohne dauerhafte Entwicklereingriffe erledigen können.

Das langfristige Produktziel ist erreicht, wenn WERK als primärer digitaler Arbeitsraum eines Unternehmens eingesetzt wird, Fachmodule und Partner sicher erweitert werden können und Plattform, Daten sowie Bedienkonzept über mindestens zehn Jahre kontrolliert weiterentwickelbar bleiben.

---

## 16. Leitentscheidung

WERK konkurriert nicht dadurch, dass es bestehende Großsysteme vollständig kopiert. Die Differenzierung entsteht durch eine einheitliche, verständliche und ereignisbasierte Unternehmensplattform, die:

- Zusammenhänge statt Datensilos abbildet,
- aktuelle Arbeit statt bloßer Menüstrukturen organisiert,
- Entscheidungen und Auswirkungen nachvollziehbar macht,
- KI nur kontrolliert und prüfbar handeln lässt,
- Self-Hosting und Datenhoheit ernst nimmt,
- Erweiterbarkeit ohne Auflösung des Plattformkerns ermöglicht,
- und langfristige Upgradefähigkeit als Produktmerkmal behandelt.

Alle Architektur-, Produkt- und Entwicklungsentscheidungen werden an diesem Gesamtprojektziel gemessen.
