# WERK Design System

**Status:** erste verbindliche Plattformgrundlage

Das WERK Design System gilt für Login, Workspace, Administration, native
Work-Clients und gemeinsam genutzte Core-Oberflächen. Fachmodule verwenden
dieselben Grundbausteine und ergänzen nur ihre fachlich notwendigen
Komponenten. Die Clientgrenzen beschreibt
[`ADR-013`](adr/ADR-013-native-clients-kotlin-compose-multiplatform.md).

## Grundhaltung

- ruhig, kompakt und desktop-orientiert
- klare Zustände statt dekorativer Effekte
- kleine Radien, feine Grenzen und zurückhaltende Schatten
- ein gemeinsamer Akzent für interaktive Elemente
- Kontoart, Mandant und Sicherheitszustand bleiben sichtbar

## Tokens

Die CSS-Custom-Properties in `dashboard/public/styles.css` sind derzeit die
technische Quelle für Farbe, Typografie, Abstände, Radien, Höhen und Schatten
der Weboberfläche. Vor dem ersten Compose-Client werden die semantischen Tokens
in einen plattformneutralen Vertrag überführt und daraus nach CSS und Compose
abgebildet. Bis dahin darf die Compose-Seite die Semantik dokumentiert
nachbilden, aber kein unabhängiges Farb- oder Abstandssystem definieren.

Die Abstände basieren auf einem 4-Pixel-Raster. Die Standarddichte heißt
`cozy`; spätere Dichten dürfen nur die globalen Größen-Tokens verändern.

## Globale Komponenten

### App-Kopf

Der App-Kopf enthält Produktmarke, aktuellen Sicherheitsbereich und globale
Kontofunktionen. Arbeits- und Administrationsbereich dürfen ähnlich aussehen,
aber nicht innerhalb derselben Session gewechselt werden.

### Globale Navigation

Die globale Desktop-Navigationsleiste verläuft links vom Inhalt von oben nach unten.
Core Identity bestimmt, ob sie Ziele des Arbeits- oder Administrationsbereichs
zeigt. Der aktuelle Bereich ist textlich und visuell markiert. Noch nicht
implementierte Ziele dürfen zur Orientierung deaktiviert erscheinen, führen
aber keine Aktion aus. Eine globale Navigation darf niemals einen Wechsel der
Kontoart innerhalb derselben Session anbieten.

Die Leiste kann pro Konto ein- oder ausgeklappt gespeichert werden. Diese
Präferenz gehört zum Konto und gilt deshalb auch in einem anderen Browser. Die
Wahl verändert nur die Darstellung, niemals Ziele oder Berechtigungen. Eine
horizontale Navigation oder ein Grid ist für lokale Ebenen innerhalb späterer
Fachanwendungen reserviert und dupliziert keine globale Navigation.

Auf Smartphone und Tablet darf dieselbe Informationsarchitektur adaptiv als
kompakte Leiste, Zielübersicht oder Master-Detail-Navigation erscheinen. Ziele,
Berechtigungswirkung und Benennung bleiben gleich; nur Darstellung und
Interaktion sind plattformspezifisch.

### Konto-Avatar und Profilmenü

Der Avatar zeigt bis zu zwei Initialen und daneben Anzeigename sowie Kontoart.
Das Profilmenü enthält die vollständige Identität, den Profilzugang und die
Abmeldung. Es schließt bei Außenklick und Escape. Ein späteres Profilbild ersetzt
nur die Initialenfläche, nicht Aufbau oder Bedienverhalten.

### Status und Hinweise

Grün kennzeichnet einen bestätigten positiven Zustand, Rot einen Fehler und Blau
eine neutrale Plattforminformation. Farbe ist nie die einzige Statusinformation.

### Formulare und Buttons

Primäraktionen verwenden den Akzent. Destruktive oder sitzungsbeendende Aktionen
werden klar benannt. Lade-, Fehler- und Erfolgsmeldungen werden in unmittelbarer
Nähe der ausgelösten Aktion dargestellt.

## Verbindliche Sicherheitsdarstellung

- Kontoart und serverseitiger Startbereich stammen aus Core Identity.
- Mandantenbindung wird niemals als frei behauptbare Browser- oder Clientoption
  dargestellt.
- Das Frontend blendet unzulässige Aktionen aus, ersetzt aber keine serverseitige
  Autorisierung.
- Admin-, Work- und Service-Konten erhalten keine gemeinsame wechselbare Rolle.

## Ausbau

Neue gemeinsame Komponenten werden zuerst hier beschrieben und anschließend in
der Web- beziehungsweise Compose-Komponentenbasis umgesetzt. Fachmodulspezifische
Muster bleiben beim jeweiligen Modul. Plattformübergreifend geteilt wird nur,
was Bedienbarkeit, Barrierefreiheit und native Konventionen nicht verschlechtert.
