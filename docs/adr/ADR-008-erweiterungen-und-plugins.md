# ADR-008 – Erweiterungen, Plugins und Capabilities

**Status:** Angenommen  
**Datum:** 2026-07-19

## Kontext

WERK soll langfristig erweiterbar sein, ohne den stabilen Core mit beliebigem
Code, direkten Datenbankzugriffen oder unversionierten Interna zu belasten. Ein
App Store ist optional und wird nicht für den Plattformstart vorausgesetzt.

## Entscheidung

- Erweiterungen integrieren sich über versionierte HTTP-, Event- und
  Extension-Point-Verträge.
- Für Low-Trust-Logik wird zuerst eine WASM-Sandbox vorgesehen. Integrationen mit
  eigener Laufzeit laufen als isolierte Sidecars über HTTP oder gRPC.
- Eine native Go-Plugin-ABI und dynamisches Laden fremder Go-Binaries im Core
  sind ausgeschlossen.
- Jedes Plugin besitzt ein signiertes Manifest mit Version, Publisher,
  angeforderten Capabilities, Ressourcenlimits und Kompatibilitätsbereich.
- Installation erteilt keine Datenrechte automatisch. Ein Betreiber vergibt
  widerrufbare Capability-Grants je Tenant, Version und Ressourcenscope.
- Plugins handeln als eigenes Service-Subjekt. Sie übernehmen nie still die
  Berechtigungen eines angemeldeten Benutzers und greifen nie direkt auf
  PostgreSQL, Secrets oder interne Modul-Tabellen zu.
- Timeouts, CPU/RAM-Limits, Netzwerkregeln, Audit und ein deaktivierbarer
  Lebenszyklus sind Pflicht. Fehlende oder widerrufene Capabilities führen zu
  einer sicheren Ablehnung.

## Grenzen

Ein späterer kuratierter Marketplace ist eine Produkt- und Lieferkettenfunktion,
keine Voraussetzung für interne Erweiterungen. Er benötigt zusätzliche
Signatur-, Prüf- und Updateprozesse und wird erst nach dem Plattformkern geplant.

## Nachweis

Tests müssen Signaturprüfung, Capability- und Tenant-Grenzen, Widerruf,
Ressourcenlimits, Versionkompatibilität, Audit und das Fehlen direkter
Datenbankzugriffe nachweisen.
