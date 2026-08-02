# ADR-013: Native Clients mit Kotlin und Compose Multiplatform

- **Status:** Angenommen
- **Datum:** 2026-07-21
- **Betrifft:** Work-Clients, optionale Admin-Clients, API-Verträge, Core Identity,
  Design System, Offline-Synchronisation und Release Engineering

## Kontext

WERK besitzt eine eigenständige Weboberfläche, soll aber langfristig auch als
regulär installierbare Anwendung auf Smartphones, Tablets und Desktop-Systemen
verfügbar sein. Eine PWA oder eine WebView-Shell erfüllt dieses Ziel nicht: Sie
ist weiterhin vom Browsermodell abhängig und bietet für sicheren lokalen
Schlüsselspeicher, Biometrie, Hintergrundarbeit, Benachrichtigungen,
Dateiintegration und Betriebssystem-Lebenszyklen nur eine begrenzte oder
indirekte Plattformanbindung.

Die Clientstrategie darf gleichzeitig keine zweite Businesslogik, keine
abweichenden Sicherheitsentscheidungen und keinen direkten Datenzugriff neben
der versionierten WERK-API erzeugen. Mobile und Desktop benötigen
unterschiedliche Bedienmuster, sollen aber gemeinsame Modelle, Verträge und
Designgrundlagen verwenden.

## Entscheidung

WERK verwendet für seine installierbaren Smartphone-, Tablet- und späteren
Desktop-Clients **Kotlin Multiplatform** als gemeinsame Clientbasis und
**Compose Multiplatform** für gemeinsam nutzbare Oberflächen.

Die ersten Zielplattformen sind Android und iOS beziehungsweise iPadOS.
Windows, macOS und Linux werden auf derselben Architektur ergänzt, sobald ein
fachlicher Bedarf und ein tragfähiger Releasekanal vorliegen. Die Clients sind
regulär installierbare Plattformanwendungen; sie sind weder PWA noch eine
WebView-Verpackung der WERK-Weboberfläche.

Die Next.js-/React-Weboberfläche bleibt ein eigenständiger, voll unterstützter
Client. Sie wird nicht durch Compose for Web ersetzt. Web- und
Kotlin-Multiplatform-Clients teilen Verträge, Designsemantik und Fachregeln,
aber keinen erzwungen gemeinsamen UI-Quellcode.

## Clientaufbau

Der gemeinsame Kotlin-Code darf enthalten:

- aus OpenAPI-Verträgen abgeleitete API-Modelle und Transportlogik,
- Session- und Tenant-Kontext ohne clientseitig behauptete Kontoart,
- fachneutrale Validierung und Zustandsmodelle,
- verschlüsselte lokale Projektionen und Synchronisationslogik,
- gemeinsame ViewModels und geeignete adaptive Compose-Komponenten,
- Telemetrie-, Fehler- und Korrelationskontext ohne geheime Inhalte.

Plattformspezifische Adapter bleiben für folgende Fähigkeiten vorgesehen:

- Keychain beziehungsweise Keystore und biometrische Freigabe,
- Systembrowser, Redirects, Deep Links und App Links,
- Push-Benachrichtigungen und Hintergrundausführung,
- Kamera, Dateien, Teilen und Betriebssystemberechtigungen,
- Fenster, Tray, Dateizuordnungen und sichere Updates auf Desktop-Systemen.

Eine maximale Quote gemeinsam genutzten Codes ist kein Qualitätsziel. Wo
Plattformkonvention, Barrierefreiheit, Sicherheit oder Bedienbarkeit es
verlangen, erhält die jeweilige Plattform eine eigene Implementierung hinter
einem gemeinsamen Vertrag.

## Sicherheits- und Kontogrenzen

Alle Clients sprechen ausschließlich die versionierte Business-API an. Sie
greifen nie direkt auf PostgreSQL, Object Storage, Valkey oder interne
Go-Schnittstellen zu. Der Tenant und die Kontoart stammen ausschließlich aus
der serverseitig bestätigten Identität.

Der erste native Produktclient ist ein `work`-Client. Eine mobile
Administrationsoberfläche gehört nicht zum ersten Lieferumfang. Ein späterer
Desktop- oder Mobile-Admin-Client benötigt ein getrenntes Produktartefakt,
getrennte Session- und Tokenablage, die Audience `admin` und ausschließlich
`/admin/v1`. Er darf keinen Schalter zum Wechseln in den Arbeitsbereich bieten.
`service`-Konten erhalten keine interaktive Clientoberfläche.

Das bestehende Cookie-, Origin- und CSRF-Modell der Weboberfläche wird nicht
unverändert in native Clients übertragen. Vor der ersten Anmeldung eines
nativen Clients wird ein eigenes Authentifizierungs-ADR mit Bedrohungsmodell
angenommen. Es muss mindestens Systembrowser-basierte Anmeldung, PKCE und
Redirect-Bindung, kurzlebige beziehungsweise widerrufbare Sitzungsnachweise,
sicheren Plattformspeicher, MFA/Re-Authentifizierung, Geräteverlust und die
strikte Audience-Trennung behandeln. Eingebettete Login-WebViews und
deaktivierte TLS-Zertifikatsprüfung sind ausgeschlossen.

## Offline- und Synchronisationsgrenze

PostgreSQL bleibt auch bei Offline-Funktionen die fachliche Wahrheit. Lokale
Daten sind verschlüsselte, widerrufbare Projektionen und niemals ein zweites
System of Record.

- Lesecaches tragen Tenant, Konto, Vertragsversion, Datenklasse und Ablaufzeit.
- Schreibvorgänge werden nur für ausdrücklich offlinefähige Commands mit
  Idempotency Key, lokaler Zustandsanzeige und definierter Konfliktbehandlung
  vorgemerkt.
- Fachliche Freigaben, Rechteänderungen und andere hochriskante Aktionen
  benötigen standardmäßig eine aktuelle Online-Policy-Prüfung und gegebenenfalls
  Re-Authentifizierung.
- Push-Nachrichten enthalten höchstens einen knappen Hinweis oder eine
  Ressourcenreferenz; autorisierte Inhalte lädt der Client erneut über die API.
- Abmeldung, Kontosperre oder Gerätewiderruf müssen lokale Secrets und
  geschützte Projektionen unbrauchbar machen können.

## Oberfläche und Design System

Smartphone, Tablet und Desktop verwenden dieselbe Informationsarchitektur und
dieselben semantischen Design Tokens, aber adaptive Navigations- und
Arbeitsmuster. Eine Desktop-Tabelle wird nicht lediglich auf Telefonbreite
verkleinert; eine Telefonansicht wird auf dem Desktop nicht nur vergrößert.

Das WERK Design System erhält plattformspezifische Abbildungen für CSS und
Compose. Sicherheitszustände, Kontoart, Mandantenkontext, Berechtigungswirkung
und Barrierefreiheit müssen semantisch gleich bleiben. Fachmodule dürfen native
Oberflächen nur über registrierte, versionierte Clientbeiträge erweitern; ein
beliebiges WebView ist kein nativer Erweiterungspunkt.

## Build und Auslieferung

Jede Plattform besitzt reproduzierbare, signierte Artefakte und einen getrennten
Releasekanal. iOS-/iPadOS-Builds und Signierung benötigen macOS und Xcode.
Desktop-Clients erhalten keine weitergehenden Serverrechte allein aufgrund ihrer
Installation. SBOM, Abhängigkeitsprüfung, Signatur, Updateverfahren und
Kompatibilitätsmatrix werden vor dem ersten produktiven Release in den
Lieferprozess aufgenommen.

## Verworfene Alternativen

### Responsive Web oder PWA als einziger Client

Die Weboberfläche bleibt wichtig, ist aber kein Ersatz für den geforderten
installierbaren Mobile-/Tablet-Client und seine Plattformintegration.

### Flutter

Flutter ermöglicht ebenfalls installierbare plattformübergreifende Clients und
bleibt technisch grundsätzlich geeignet. Es wird nicht gewählt, weil WERK mit
Kotlin Multiplatform gemeinsame Logik und UI stufenweise teilen kann, ohne für
die Clientdomäne zusätzlich Dart und eine zweite, überwiegend
framework-gerenderte Integrationswelt einzuführen.

### Getrennte Kotlin-/Swift-/Desktop-Implementierungen

Vollständig getrennte Clients bieten maximale Plattformnähe, würden aber
Transport, Zustandsmodelle, Synchronisation und große Teile der Oberfläche
mehrfach implementieren. Plattformspezifischer Code bleibt innerhalb der
Multiplatform-Struktur trotzdem möglich.

### Tauri als allgemeiner Desktop-Client

Eine Web-Shell kann einzelne Desktopfälle schnell abdecken, würde jedoch eine
zweite offizielle Desktoparchitektur neben dem nativen Mobile-Stack schaffen.
Tauri ist deshalb nicht mehr der vorgesehene Standardweg. Ein späterer
Sonderfall benötigt ein eigenes ersetzendes ADR.

## Folgen

- Mobile und Desktop können auf einer gemeinsamen, typisierten Clientbasis
  wachsen.
- Web bleibt unabhängig auslieferbar und muss nicht auf Kotlin migriert werden.
- Native Plattformfunktionen bleiben erreichbar, verursachen aber weiterhin
  gezielten Kotlin-, Swift- oder Plattformcode.
- Das Repository benötigt künftig eine klare Clientmodul-, Versions- und
  Releasegrenze sowie Kotlin-/Gradle-Kompetenz.
- Ein macOS-Buildpfad ist für Apple-Ziele unvermeidbar.
- Native Authentifizierung und Offline-Synchronisation werden vor Umsetzung
  jeweils durch eigenes Bedrohungsmodell und konkretisierende ADRs begrenzt.

## Neubewertung

Die Entscheidung wird nach einem produktionsnahen Android-/iOS-Work-Pilot und
vor dem ersten Desktop-Release überprüft. Ein Wechsel des Frameworks erfordert
ein ersetzendes ADR mit Migrationspfad. Einzelne fehlende Bibliotheken oder
Plattformadapter rechtfertigen zunächst eine lokale native Implementierung und
keine parallele Clientarchitektur.

Die operative Zielstruktur und die Lieferstufen beschreibt
[`CLIENT-ARCHITEKTUR.md`](../CLIENT-ARCHITEKTUR.md).

## Referenzen

- [Kotlin Multiplatform: unterstützte Plattformen und Stabilität](https://kotlinlang.org/docs/multiplatform/supported-platforms.html)
- [Kotlin Multiplatform: Code zwischen Plattformen teilen](https://kotlinlang.org/docs/multiplatform/multiplatform-share-on-platforms.html)
- [Compose Multiplatform: Einstieg und Plattforminteroperabilität](https://kotlinlang.org/docs/multiplatform/compose-multiplatform.html)
