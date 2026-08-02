# WERK – Offene Fragen aus der Dokumentationsinventur

**Stand:** 29.07.2026  
**Status:** gesammelt, noch nicht gestellt und nicht entschieden

Diese Datei hält Fragen zurück, die für die erste Bestandsaufnahme nicht
beantwortet werden müssen. Keine der aufgeführten Möglichkeiten gilt als
bestätigt. Die Fragen werden erst nach dem Grundgerüst und in der vom Nutzer
gewünschten Form mit drei Antwortmöglichkeiten gestellt.

## A. Dokumentenhoheit

1. Welche verbindliche Rolle soll `WERK_GESAMTPROJEKTZIEL.md` gegenüber
   `vision.md`, `DATENMODELL.md` und `ROADMAP.md` besitzen?
2. Welches Dokument soll den aktuellen Implementierungsstand führen:
   `ROADMAP.md`, `SESSION_STATUS.md` oder
   `BACKEND-IMPLEMENTIERUNGSSTAND.md`?
3. Sollen Zielbild, verbindlicher Vertrag und aktueller Ist-Stand dauerhaft in
   denselben Dokumenten stehen oder als getrennte Dokumentarten geführt werden?

## B. Zielstruktur

4. Soll die bestehende flache Struktur unter `docs/` erhalten bleiben oder
   später nach Dokumentarten beziehungsweise Domänen gegliedert werden?
5. Soll es einen zentralen Dokumentationsindex mit Owner, Status, Zielgruppe,
   Gültigkeit und letzter Prüfung geben?
6. Soll der leere Ordner `wiki/` künftig verwendet, bewusst reserviert oder
   entfernt werden?

## C. Status und Lebenszyklus

7. Welche einheitlichen Statuswerte sollen für Visionen, Verträge, Pläne,
   Betriebsdokumente und ADRs gelten?
8. Wie sollen veraltete oder ersetzte Dokumente kenntlich gemacht werden?
9. Soll ein Dokument einen verantwortlichen Owner und einen festen
   Überprüfungszeitpunkt ausweisen?

## D. Tiefer Abgleich

10. Wie tief soll der spätere Abgleich mit Code, Migrationen, OpenAPI und Tests
    reichen?
11. Sollen widersprüchliche Aussagen nur gemeldet oder nach Bestätigung direkt
    bereinigt werden?
12. Welche externen Referenzen und Normen sollen auf Aktualität und Gültigkeit
    geprüft werden?

## E. Sprache und Zielgruppen

13. Soll die gesamte Projektdokumentation deutsch bleiben, zweisprachig werden
    oder technische Paketdokumentation weiterhin englisch sein dürfen?
14. Welche getrennten Einstiege werden für Betreiber, Entwickler,
    Modulautoren und Sicherheitsprüfer benötigt?
15. Soll `README.md` ausschließlich Schnellstart sein oder zugleich der
    verbindliche Einstieg in die gesamte Dokumentationslandschaft bleiben?

## F. Technische Collaboration-Mechanismen

Diese Fragen entstanden aus
[`COLLABORATION-MECHANISMEN-INVENTUR.md`](COLLABORATION-MECHANISMEN-INVENTUR.md)
und sind nicht entschieden:

16. Für welche Datenform soll ein späterer technischer Collaboration-Nachweis
    zuerst geführt werden: strukturierter Baum, Rich Text, Whiteboard oder
    Formularzustand?
17. Welche Latenz-, Teilnehmer-, Dokumentgrößen- und Offline-Grenzen muss ein
    technischer Mechanismus nachweislich tragen?
18. Muss der erste Mechanismus vollständig selbst gehostet und ohne externen
    Relay-Dienst funktionieren?
19. Soll eine veränderliche Arbeitskopie ausschließlich aus einem
    Operationsjournal rekonstruierbar sein oder zusätzlich einen eigenen
    aktuellen Snapshot besitzen?
20. Welche Operationen müssen bei einem Rechteentzug sofort unterbrochen und
    welche bereits bestätigten Operationen noch abgeschlossen werden?
21. Welche Presence-Daten dürfen flüchtig übertragen werden und welche dürfen
    wegen Datenschutz oder Sicherheitswirkung überhaupt nicht entstehen?
22. Soll Offline-Bearbeitung zum ersten Collaboration-Schnitt gehören oder ein
    ausdrücklich späterer, separat abgenommener Mechanismus bleiben?
23. Welcher degradierte Zustand ist bei Ausfall des Collaboration-Dienstes
    zulässig: read-only, Einzelbearbeitung oder vollständige Sperre?
24. Welche technische Evidenz wäre vor einer Architekturentscheidung
    erforderlich: Prototyp, deterministische Simulation, Lasttest,
    Netztrennungstest oder Kombination daraus?

## G. Einordnung der Collaboration-Grundlagen

Diese Fragen entstanden aus
[`COLLABORATION-ARCHITEKTUREINORDNUNG.md`](COLLABORATION-ARCHITEKTUREINORDNUNG.md)
und sind nicht entschieden:

25. Soll die technische Laufzeit ausschließlich dem späteren
    Dokument-Collaboration-Dienst gehören oder als fachneutrale Capability
    abgegrenzt werden?
26. Soll ihre Grundlage in Roadmap-Phase 2, vollständig in Phase 3 oder erst
    nach dem ersten Dokument-/Pilotpfad untersucht werden?
27. Ist das Operationsjournal selbst eine dauerhaft wiederherstellbare
    PostgreSQL-Wahrheit oder dient ein anderer persistenter Pfad als
    rekonstruierbare technische Wahrheit?
28. Welche Rolle besitzt ein Snapshot: reine Startbeschleunigung,
    Wiederherstellungsanker oder beides?
29. Welche kleinste Grenze bildet einen Synchronisationsraum, ohne eine
    unkontrollierte Zahl langlebiger Räume oder Hotspots zu erzeugen?
30. Welche bestätigte Operation darf nach Session- oder Rechtewiderruf noch in
    einen Snapshot einfließen?
31. Welche Aufbewahrungs- und Verdichtungsregel gilt für Operationen nach einer
    erfolgreichen Veröffentlichung?
32. Muss der erste technische Nachweis bereits mehrere Serverprozesse und einen
    Prozessausfall abdecken oder zunächst nur deterministische Konvergenz in
    einem Prozess?
33. Welche der drei möglichen Roadmap-Positionen soll später zur Entscheidung
    ausgearbeitet werden?

## H. Dienstereife und erster Phase-2-Schnitt

Diese Fragen entstanden aus
[`DIENSTEREIFE-INVENTUR.md`](DIENSTEREIFE-INVENTUR.md) und sind nicht
entschieden:

34. Welcher bestehende Ressourcentyp soll den ersten
    `BusinessObjectView`-End-to-End-Schnitt bilden?
35. Speichert der gemeinsame Dienst eine ownerseitig gelieferte Projektion oder
    ruft er die Ansicht bei jeder Query über einen Owner-Vertrag ab?
36. Welche minimalen Felder darf eine gemeinsame Objektansicht dauerhaft
    besitzen, ohne eine zweite fachliche Wahrheit zu erzeugen?
37. Darf die erste Ansicht nur Navigation und Anzeige bedienen oder bereits als
    Grundlage für Suche und Relationen gelten?
38. Welche Work-Permission schützt die gemeinsame Ansicht, und benötigt jeder
    Ressourcentyp zusätzlich eine eigene Leseberechtigung?
39. Wie wird eine Projektion bei Archivierung, Löschung, Rechteentzug oder
    fehlgeschlagener Aktualisierung geschlossen unsichtbar?
40. Muss die erste Projektion synchron in derselben PostgreSQL-Transaktion wie
    das Owner-Objekt entstehen oder darf sie aus der Outbox idempotent und
    zeitversetzt aufgebaut werden?
41. Welcher Frischezustand muss für Clients sichtbar werden, falls Projektion
    und Owner-Zustand vorübergehend voneinander abweichen?

## I. Organisation und Add-on-Verknüpfungen

Diese Fragen betreffen die spätere Konkretisierung der bereits vorhandenen
Organisations-, Access-Gruppen- und App-Entitlement-Grundlage. Sie entscheiden
weder die Reichweite eines Add-ons noch Rollen, Vererbung oder Ausnahmen vorab.

### Zuerst in der Breite

42. Welche grundsätzliche Reichweite soll ein aktiviertes Add-on im Unternehmen
    zunächst besitzen?
43. Über welche organisatorischen Ziele soll ein Add-on hauptsächlich
    freigeschaltet werden: Organisationseinheiten, Access-Gruppen oder einzelne
    Arbeitskonten?
44. Welches grundsätzliche Verhältnis sollen Add-on-Freischaltung, Rollen und
    Fachberechtigungen in der Nutzerführung haben?

### Spätere Tiefenfragen nach bestätigtem Grundgerüst

45. Soll eine Freischaltung für eine Organisationseinheit ausschließlich diese
    Einheit oder ausdrücklich auch ihre untergeordneten Einheiten erfassen?
46. Wie sollen direkte Kontofreischaltungen, Organisationsfreischaltungen und
    Access-Gruppen zusammenwirken, wenn mehrere davon dasselbe Konto betreffen?
47. Darf eine Add-on-Freischaltung jemals eine Rolle oder Rollenvorlage
    vorschlagen beziehungsweise zuweisen, oder bleiben beide Schritte vollständig
    getrennt?
48. Was geschieht mit wirksamen Freischaltungen, wenn eine Abteilung verschoben,
    deaktiviert oder archiviert wird?
49. Welche zeitliche Gültigkeit, Befristung und Widerrufslogik benötigen
    Freischaltungen und Gruppenmitgliedschaften, und wie erzwingt der spätere
    Runtime-Store pro Prüfung einen frischen Snapshot mit servereigener Zeit?
50. Welche Zuständigkeiten dürfen später an Leitungen von Bereichen oder
    Abteilungen delegiert werden, ohne daraus Plattformadministratoren zu machen?
51. Wie müssen Oberfläche und Audit erklären, über welche Einheit, Gruppe oder
    direkte Ausnahme ein Konto Zugang zu einem Add-on erhalten hat?
52. Welches reale Add-on soll als erster End-to-End-Verbraucher die
    Verknüpfungsregeln, Fehlerfälle und Reorganisation nachweisen?

## J. Historische Fragen zum Single-Company-Startprofil

**Status:** durch die bestätigte Korrektur „WERK ist kein Single-Company-
Produkt“ überholt; nicht mehr zur Entscheidung zu stellen. Die Fragen bleiben
nur als nachvollziehbare Inventurhistorie erhalten. Die neue Breitenprüfung
beginnt in Abschnitt K.

Bestätigt sind die sichtbare Bezeichnung **Unternehmen**, die Einordnung von
Abteilungen als innere Organisationseinheiten und der weiterhin
mandantenfähige Sicherheitskern. Noch nicht entschieden ist, ob das aktuelle
Startprofil die Anzahl operativer Unternehmen zusätzlich serverseitig
begrenzen soll. Bis zu dieser Entscheidung behandelt die Oberfläche einen
unbekannten Bestand fail-closed und bleibt bei einem vorgefundenen
Mehrfachbestand im sichtbaren Auswahlmodus. Ist darin genau ein Eintrag aktiv,
wird dieser lediglich vorgewählt; bei mehreren aktiven Einträgen wird nichts
angenommen. Der versionierte Tenant-Vertrag wird nicht stillschweigend verengt.

### Zuerst in der Breite

53. Soll „Single Company“ ausschließlich das aktuelle Produkt- und UX-Profil
    beschreiben oder als eigene serverseitige Betriebsprofil-Invariante gelten?
54. Wie soll eine Installation grundsätzlich reagieren, wenn durch Import,
    Wiederherstellung oder ältere Daten bereits mehrere Unternehmenswelten
    vorhanden sind?
55. Soll ein späteres Mehr-Unternehmen-Profil durch eine explizite
    Betreiberkonfiguration aktiviert werden oder ausschließlich Gegenstand
    eines neuen Architekturentscheids sein?

### Spätere Tiefenfragen nach bestätigtem Grundgerüst

56. Falls das Startprofil serverseitig begrenzt wird: Welche atomare Sperre und
    welcher stabile API-Fehler verhindern zwei parallele Erstanlagen?
57. Welche Systempfade dürfen eine solche Grenze für Restore, Migration oder
    kontrollierten Profilwechsel überschreiten, und wie wird das auditiert?

## K. Unternehmensdatenbanken, Gruppen und Benutzerzuordnung

Diese Fragen entstanden aus
[`UNTERNEHMENS-DATENBANK-INVENTUR.md`](UNTERNEHMENS-DATENBANK-INVENTUR.md).
Die neue Arbeitsrichtung betrachtet WERK nicht als Single-Company-Produkt und
prüft eine eigene logisch verwaltete PostgreSQL-Datenbank je Unternehmen für
Self-hosted-, Cloud- und Hybridbetrieb. Die folgenden Punkte sind noch keine
angenommene Architekturentscheidung.

### Zuerst in der Breite

58. Umfasst eine WERK-Installation künftig genau eine Control Plane mit einem
    gemeinsamen Identity-Realm und ein bis vielen Unternehmensdatenbanken, oder
    bleiben einzelne Unternehmen vollständige, nur betrieblich gruppierte
    Installationen?
59. Ist eine Unternehmensgruppe ausschließlich ein Verwaltungsverbund ohne
    automatische Fachdatensicht, oder soll sie einen eigenen gemeinsamen
    Daten- und Arbeitsraum besitzen?
60. Soll eine Person einen gemeinsamen `WorkPrincipal` mit getrennten lokalen
    WorkAccounts je Unternehmen besitzen, oder beginnt jede Anmeldung bereits
    in einem vollständig companylokalen Identity-Realm?
61. Bleiben Plattformadministratoren installationsweit, während fachliche
    Unternehmens- und Gruppenverantwortung ausschließlich über lokale
    WorkAccounts und Rollen abgebildet wird?
62. Ist die eigene logische Company-DB für jedes Unternehmen verpflichtend,
    während gemeinsamer Cluster, dedizierter Cluster und externer Provider nur
    verschiedene Placements desselben Vertrags sind?

### Spätere Tiefenfragen nach bestätigtem Grundgerüst

63. Welche Control-Plane-Daten sind kanonisch, welche Company-Daten lokal, und
    welche globalen Vertragskataloge werden nur versioniert lokal projiziert?
64. Wie werden Company-ID, Data-Plane-ID, Realm, Binding-Generation und
    Schemaversion beim Öffnen jeder Connection fail-closed attestiert?
65. Erzeugt WERK lokale Datenbanken selbst, übernimmt es betreiberseitig
    bereitgestellte Datenbanken oder unterstützt der Lifecycle-Port beide Wege?
66. Verwenden Company-DBs auf einem gemeinsamen Cluster eigene Login-Rollen und
    Credentials oder zeitlich begrenzte Providerzugänge?
67. Wie werden Connection-Budget, Pool-Lebenszeit, Credential-Rotation und
    Pool-Drain bei vielen Unternehmensdatenbanken begrenzt?
68. Welche Zwischenzustände, Fristen und Rücknahmen gelten für Join, Leave und
    Company-Wechsel eines vorhandenen Principals?
69. Darf ein Unternehmenswechsel eine kurze Zugriffslücke verursachen, oder ist
    eine ausdrücklich befristete und auditierte Überlappung erforderlich?
70. Wie werden lokale Passwörter, Passkeys, installationsweite und
    companygebundene OIDC-/SAML-/LDAP-Provider sowie spätere SCIM-
    Provisionierung gemeinsam geroutet?
71. Welche gruppenweiten Benutzer-, Add-on- und Konfigurationsaktionen sind nur
    Templates und welche besitzen einen dauerhaften Desired-State-Vertrag?
72. Wie werden Teilfortschritt, nicht erreichbare Company-DBs und veraltete
    Projektionen im Admin- und Betriebsdashboard dargestellt?
73. Welche API- und Schemaversionen dürfen während Canary- und Wellenmigrationen
    gleichzeitig aktiv sein?
74. Wie werden Control-DB, Company-DB, Object Storage und globale
    Identity-Daten für Backup, Restore und einen Route-Generation-Swap
    koordiniert?
75. Was bedeutet „Unternehmen entfernen“ getrennt für Gruppenmitgliedschaft,
    Archivierung, Retention, Export, Legal Hold, Credential-Widerruf,
    Object-Storage-Löschung und irreversible DB-Deprovisionierung?
76. Wie wird ein bestehender gemeinsamer Mehrtenant-Datenbestand in einzelne
    Company-DBs exportiert, geprüft und umgeschaltet, ohne angewendete
    Migrationen zu verändern?
77. Welche minimale Control-Plane-Funktion bleibt bei Ausfall einer Company-DB
    verfügbar, und welches Verhalten gilt umgekehrt bei Ausfall der Control
    Plane?
