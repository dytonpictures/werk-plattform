# ADR-017 – EU-Compliance- und Datenverarbeitungsgrundlage

**Status:** Angenommen  
**Datum:** 2026-07-21

## Kontext

Eine erfolgreiche Autorisierung beantwortet, ob ein Akteur eine Aktion an einer
Ressource ausführen darf. Sie beantwortet nicht automatisch, ob die dabei
stattfindende Verarbeitung personenbezogener Daten rechtmäßig, zweckgebunden,
erforderlich oder fristgerecht löschbar ist. Diese zweite Entscheidung darf
weder aus einer Rolle abgeleitet noch von einer App stillschweigend umgangen
werden.

Die Plattform soll Betreiber bei der Einhaltung europäischer Anforderungen
technisch unterstützen. Software allein kann jedoch keine pauschale
Rechtskonformität herstellen: Anwendbarkeit, Rechtsgrundlage, Verträge,
Konfiguration, betriebliche Prozesse und tatsächliche Nutzung bleiben abhängig
vom Betreiber und seinem Einsatzfall.

## Entscheidung

### Hierarchie der Anforderungen

Anforderungen werden von oben nach unten ausgewertet:

```text
EU-Recht
  -> nationales Recht und Aufsichtspraxis
     -> sektor- und betreiberspezifische Pflichten
        -> unveränderliche Plattformgrenzen
           -> Tenant-Policy
              -> App-, Ressourcen- und Aktionsregeln
```

Eine untergeordnete Ebene darf eine übergeordnete Pflicht nur konkretisieren
oder verschärfen. Sie darf sie nicht aufheben. Welche Rechtsakte tatsächlich
anwendbar sind, wird nicht aus dem Ressourcentyp geraten, sondern außerhalb des
Autorisierungskerns verbindlich festgelegt und versioniert.

### Verbindliches Datenprofil je Ressourcentyp

Jeder autorisierbare Ressourcentyp benötigt in PostgreSQL ein aktives
`ResourceDataProfile`:

```text
ResourceDataProfile
  resource_kind
  personal_data_category: none | personal | special-category |
                          criminal-offence
  confidentiality_level: public | internal | confidential | restricted
  processing_activity_required
  contract_version
  status
```

Fehlt das Profil oder ist es deaktiviert, scheitert bereits die
Autorisierungsauflösung geschlossen. Sobald ein Profil personenbezogene Daten
ausweist, muss es außerdem einen Verarbeitungsvorgang verlangen. Das Profil ist
eine konservative Voreinstellung beziehungsweise Obergrenze für den
Ressourcentyp; spätere Feld-, Dokument- oder Tenant-Klassifikationen dürfen es
verschärfen.

Ein Datenprofil erklärt keine Verarbeitung für rechtmäßig und enthält bewusst
keine automatisch erfundene Rechtsgrundlage.

### Getrennter Verarbeitungskontext

Jede registrierte Kombination aus Berechtigung und Ressourcentyp benötigt in
PostgreSQL zusätzlich eine aktive `ProcessingPolicy`:

```text
ProcessingPolicy
  permission_key
  resource_kind
  processing_required
  context:
    activity_key
    purpose_key
    legal_basis_ref
  contract_version
  status
```

Fehlt Datenprofil oder Processing-Policy, wird die gemeinsame
Autorisierungsentscheidung geschlossen abgelehnt. Ein Ressourcendatenprofil
kann die Verarbeitung grundsätzlich verlangen; die Processing-Policy kann
diese Anforderung für eine konkrete Aktion nur bestätigen oder bei einem
ansonsten nicht personenbezogenen Kontrollziel zusätzlich verschärfen. Das ist
beispielsweise bei einer Erstellungsaktion wichtig, deren Autorisierungsziel die
Installation ist, deren Ergebnis aber personenbezogene Daten enthält.

Bei `processing_required = true` muss der Kontext strukturell vollständig sein.
Request-Daten dürfen weder Zweck noch Rechtsgrundlagenreferenz festlegen oder
erweitern. Der Identitätsdienst löst Profil und Policy gemeinsam mit Rolle,
Scope, Permission und Ressourcentyp serverseitig auf und übergibt sie einer
einzigen Core-Policyentscheidung.

Die heute registrierten `legal_basis_ref`-Werte zeigen lediglich auf einen
vorgesehenen betreiberseitigen Verarbeitungsnachweis. Sie behaupten weder, dass
dieser Nachweis bereits existiert, noch dass eine konkrete Rechtsgrundlage
zutrifft. Eine Registry mit Freigabestatus, Gültigkeit, Retention und
Verantwortung wird in einem eigenen Vertrag ergänzt.

### Regulatorischer Planungsrahmen

Der weitere Ausbau berücksichtigt mindestens:

- [DSGVO](https://eur-lex.europa.eu/eli/reg/2016/679/oj) und den Grundsatz
  Datenschutz durch Technikgestaltung und datenschutzfreundliche
  Voreinstellungen;
- die [NIS-2-Richtlinie](https://eur-lex.europa.eu/legal-content/DE/TXT/?uri=CELEX%3A32022L2555),
  sofern Betreiber, Sektor, Größe und nationale Umsetzung in ihren
  Anwendungsbereich fallen;
- den [Cyber Resilience Act](https://eur-lex.europa.eu/eli/reg/2024/2847/oj)
  für betroffene Produkte mit digitalen Elementen und dessen gestaffelte
  Geltung;
- die [KI-Verordnung](https://eur-lex.europa.eu/legal-content/DE/TXT/?uri=CELEX%3A32024R1689)
  für spätere KI-Funktionen abhängig von Rolle und Risikoklasse;
- den [Data Act](https://eur-lex.europa.eu/eli/reg/2023/2854/oj) sowie
  sektorspezifisch beispielsweise
  [DORA](https://eur-lex.europa.eu/legal-content/DE/TXT/?uri=CELEX%3A32022R2554),
  wenn der konkrete Betrieb in den jeweiligen Anwendungsbereich fällt.

Die Liste ist ein Architektur- und Prüfrahmen, keine abschließende
Rechtsberatung oder automatische Feststellung der Anwendbarkeit.

## Noch nicht Teil dieses Ausbauschritts

- betreiberseitiges Verzeichnis von Verarbeitungstätigkeiten und
  Freigabeworkflow,
- Lösch- und Aufbewahrungsläufe einschließlich Legal Hold,
- Betroffenenanfragen, Export, Berichtigung und Löschung,
- Transfer- und Auftragsverarbeitungsregister,
- Datenschutz-Folgenabschätzungen,
- Incident- und Behördenmeldeprozesse,
- SBOM-, Schwachstellen- und CRA-Meldeprozesse,
- KI-Systeminventar und Risikoklassifizierung.

Diese Funktionen bauen auf dem Datenprofil auf, werden aber nicht als
Scheinimplementierung vorgezogen.

## Folgen

- Apps können keinen autorisierbaren Ressourcentyp ohne sichtbare
  Datenklassifikation betreiben.
- Zugriffsrecht und strukturelle Verarbeitungs-Policy bleiben getrennte,
  gemeinsam erforderliche Prüfschichten derselben Gesamtentscheidung.
- Die Plattform kann spätere Retention-, Audit-, Export- und Löschverträge an
  stabile Ressourcentypen und Verarbeitungstätigkeiten binden.
- Betreiber müssen die rechtliche und organisatorische Ausgestaltung weiterhin
  selbst beziehungsweise mit geeigneter Fachberatung vornehmen.

## Änderbarkeit

Dies ist der angenommene Planungs- und Implementierungsstand. Kategorien,
Pflichtenkatalog, Rechtsakt-Mapping und Governance-Abläufe werden mit den
weiteren Core-Phasen versioniert verfeinert; Änderungen vorbehalten. Unverändert
bleiben die fail-closed Registrierung, die Trennung von Autorisierung und
Verarbeitungszulässigkeit sowie das Verbot automatisch angenommener
Rechtsgrundlagen.
