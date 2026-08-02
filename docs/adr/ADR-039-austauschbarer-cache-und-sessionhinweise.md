# ADR-039 – Austauschbarer Cache und kurzlebige Sessionhinweise

**Status:** Angenommen  
**Datum:** 2026-08-01

## Kontext

Die Auflösung ungültiger oder bereits abgemeldeter Browser-Sessions kann bei
wiederholten Anfragen unnötige PostgreSQL-Lesezugriffe erzeugen. Gleichzeitig
müssen Sperren, Rollenänderungen, Tenant-Status und Sessionwiderrufe sofort und
fail-closed wirken. Ein Infrastrukturprodukt darf deshalb keine zweite
Identity-Wahrheit werden.

## Entscheidung

Der Core definiert einen kleinen, herstellerneutralen `cache.Port` für opake
Werte mit TTL. Der erste Adapter verwendet den offiziellen Valkey-Go-Client;
die Fach- und Identity-Schichten importieren weder Valkey- noch Redis-Befehle.
Damit kann später ein Redis-, In-Memory- oder anderer Adapter eingesetzt
werden, ohne den fachlichen Vertrag zu ändern.

Die erste Identity-Nutzung speichert ausschließlich einen 15 Sekunden gültigen
negativen Hinweis, nachdem PostgreSQL eindeutig bestätigt hat, dass ein
SHA-256-gehashter Session-Token nicht auflösbar ist. Roh-Tokens werden niemals
Cache-Schlüssel oder -Werte. Nur der erwartete versionierte Negativmarker wird
als Treffer anerkannt; fremde oder beschädigte Werte führen zur erneuten
PostgreSQL-Prüfung. Cachefehler werden ignoriert und führen zum
normalen PostgreSQL-Pfad. Datenbank- oder Transaktionsfehler erzeugen keinen
negativen Cacheeintrag.

Positive Sessions, Actor-Daten, Rollen, Berechtigungen, Tenant-Zustände,
Session-Generationen, Widerrufe und Audit bleiben ausschließlich durch
PostgreSQL maßgeblich. Insbesondere wird die zwölfstündige Session-Laufzeit
nicht als Cache-TTL übernommen.

`WERK_CACHE_URL` aktiviert den optionalen Adapter. Produktion verlangt eine
TLS-URL mit Zertifikatsprüfung. Kurze Verbindungs- und Antwortzeiten sowie
deaktivierte Client-Retries begrenzen den Einfluss einer gestörten optionalen
Abhängigkeit. Nach einem Laufzeitfehler öffnet der Adapter lokal für fünf
Sekunden einen Fail-fast-Cooldown, damit nicht jede Anfrage erneut auf einen
gestörten Cache wartet. Der Verbindungsaufbau erfolgt verzögert bei der ersten
Nutzung. War der Cache beim Prozessstart oder später nicht erreichbar, versucht
der Adapter nach dem Cooldown erneut eine Verbindung; ein API-Neustart ist
nicht erforderlich. Readiness darf durch Cache-Ausfall nicht blockieren.

## Folgen

- Wiederholte Requests mit demselben ungültigen Token vermeiden kurzfristig
  Datenbankzugriffe.
- Cacheverlust und Adapterwechsel verändern keine korrekte Entscheidung.
- Ein positiver Session- oder Berechtigungscache ist ausdrücklich nicht Teil
  dieser Entscheidung. Dafür wären generationengebundene Invalidierung,
  Revocation-Latenz und Lasttests separat zu entscheiden.
- Ein produktiver Adaptertest benötigt eine ausdrücklich konfigurierte,
  wegwerfbare Cacheinstanz.
- Das Löschen abgelaufener PostgreSQL-Sessions ist kein Cache-Cleanup. Sessions
  werden von unveränderlichen Security-Audits referenziert, während der
  allgemeine Worker absichtlich keine Identity-Datenbankrolle besitzt. Dafür
  ist ein eigener Retention-Vertrag mit Audit-Aufbewahrung, möglicher
  Token-Hash-Anonymisierung und einer getrennten Identity-Maintenance-Grenze
  erforderlich; Request-Pfade führen keinen versteckten Session-Cleanup aus.
