# ADR-027 – Typisierte Certificate-, Key- und Secret-Provider

**Status:** Angenommen
**Datum:** 2026-07-22

## Kontext

Die globale Service-/Provider-Registry aus ADR-025 beschreibt ausschließlich
nichtgeheime Metadaten. Der native TLS-Server verwendet heute vollständige
Dateisätze, während Core Identity seine MFA-Schlüssel aus einem eigenen
Keyring bezieht. Spätere Installationen sollen dafür auch sichere Mounts,
Secret Stores, KMS oder HSM verwenden können, ohne Fachcode an ein Produkt zu
binden oder einen allgemeinen Geheimniszugriff in den Core einzubauen.

Eine gemeinsame `Get(any)`-Schnittstelle wäre dafür zu mächtig. Zertifikate,
nicht exportierbare Signaturschlüssel und Secrets haben unterschiedliche
Lebenszyklen, Rückgabewerte und Auditgrenzen. Außerdem ist eine erfolgreiche
Registry-Auflösung weder Berechtigung noch Health- oder Authority-Nachweis.

## Entscheidung

### Drei getrennte Ports

Der erste Schnitt besteht nur aus einem puren, typisierten Go-Vertrag:

1. `CertificateProvider` lädt eine öffentliche, versionierte Zertifikatskette.
2. `SigningKeyProvider` führt öffentliche Schlüsselabfragen und Signaturen
   jeweils mit der vollständigen, frischen Anfrage aus und exportiert keine
   privaten Schlüsselbytes.
3. `SecretProvider` stellt ein begrenztes Byte-Secret ausschließlich innerhalb
   eines Callbacks bereit.

Es gibt keinen gemeinsamen Superprovider, kein freies Lesen beliebiger
Materialarten und keine dynamische Go-Plugin-ABI. Provideradapter werden später
typisiert an der Composition Root zusammengesetzt.

### Registry- und Operationsbindung

Jede Anfrage trägt eine zuvor exakt aufgelöste
`providerregistry.Resolution`, eine providerlokale opake `MaterialRef` mit
ausdrücklicher Version sowie einen serverseitig erzeugten Operationskontext.
Der Kontext bindet:

- Installations- oder Tenant-Grenze und gegebenenfalls Tenant-ID,
- einen stabilen Zweckschlüssel,
- Request- und Correlation-ID.

Es existiert kein implizites `latest`, kein Fallback und keine automatische
Providerwahl. Der Material-Handle ist sensible Metadaten, bleibt außerhalb der
globalen Registry und darf nicht in Logs, Events oder allgemeine Fehlertexte
gelangen.

Eine `Resolution` ist ein frei konstruierbarer Snapshot und damit kein
Capability-Token. Ihre Struktur, Provider-Revision und Binding-Revision werden
mitgeführt, ersetzen aber keine Frischeprüfung: Vor jeder sicherheitsrelevanten
Nutzung muss der vollständige Vertrag aus Service, Capability, Provider und
Binding erneut aufgelöst werden. Ein reiner Revisionsvergleich genügt nicht,
weil auch Service oder Capability deaktiviert worden sein können. Policy,
Tenant-Ableitung und gegebenenfalls Authority-Prüfung bleiben getrennt.

### Getrennte Service- und Capability-Namensräume

Die vorgesehenen logischen Dienste sind:

```text
core.platform.service.certificate
core.platform.service.cryptographic-key
core.platform.service.secret
```

Für den späteren ersten TLS-Verbraucher sind ausschließlich folgende
installationsgebundene Fähigkeiten vorgesehen:

```text
core.platform.service.certificate.capability.server-chain.load
core.platform.service.certificate.capability.client-trust-bundle.load
core.platform.service.cryptographic-key.capability.tls-handshake.sign
```

Für Secrets wird absichtlich keine allgemeine `secret.read`-Capability
definiert. Eine spätere Fähigkeit wird pro registriertem Zweck als
`core.platform.service.secret.capability.<purpose>.use` angelegt und erhält
ihre eigene Operationsgrenze.

### Material- und Speichergrenzen

- Referenzen, Zertifikatsanzahl und DER-/Secretgrößen besitzen harte Grenzen,
  die vor dem Lesen oder Dekodieren geprüft werden.
- Öffentliche Zertifikatsketten werden defensiv kopiert.
- Öffentliche Schlüssel werden ausschließlich als begrenztes PKIX-Material
  zurückgegeben; private und unbekannte Schlüsseltypen scheitern geschlossen.
- Signaturen werden nicht über speicherbare Langzeit-Signer ausgeführt. Jede
  Operation trägt erneut Registry-Auflösung, Materialversion und Zweck. Ein
  begrenzter Eingabetyp erlaubt nur die TLS-Hashfamilien SHA-256, SHA-384,
  SHA-512 sowie Ed25519 ohne Vorhash; RSA-PSS verwendet Hashlängen-Salz.
- Signaturergebnisse sind größenbegrenzt, defensiv kopiert und an die exakte
  Anfrage gebunden.
- Secrets werden als Bytes, niemals als `string`, verwendet. Der Provider
  löscht seinen eigenen Puffer nach dem Callback bestmöglich.
- Go kann wegen Garbage Collector, Compileroptimierungen und möglichen Kopien
  im Consumer keine garantierte Zeroization zusagen. Der Vertrag macht deshalb
  keine falsche Garantie und verbietet insbesondere langlebige Strings,
  Buffer-Pools und allgemeine Caches für Material.
- Jede Provideroperation erhält `context.Context`; externe Adapter benötigen
  später Deadlines, begrenzte Antworten und begrenzte Parallelität.

### Rotation und Mehrinstanzbetrieb

Registry-Lifecycle und Material-Lifecycle bleiben getrennt. Eine Rotation
erzeugt eine neue unveränderliche Materialversion, prüft sie vollständig und
schaltet erst danach eine domäneneigene Referenz atomar um. Eine Provider-ID
ändert sich nur bei einer neuen logischen Providerinstanz, nicht bei jeder
Materialrotation.

Registrystatus oder Provider-Health erteilen keine Schreibhoheit. Ausstellung,
Rotation, Sperrung oder andere authority-geschützte Mutationen benötigen im
späteren HA-Profil zusätzlich Lease, Generation und Fencing der zuständigen
Authority-Domain. Der Platform Witness speichert kein Schlüsselmaterial.

## Erster Umsetzungsschnitt

Dieser ADR führt die Core-Verträge und ihre Validierung ein und härtet den
bestehenden mTLS-CA-Lader, sodass ein teilweise ungültiges Trust-Bundle
vollständig scheitert. Er koppelt noch keinen Provider an den laufenden TLS-,
Kafka- oder MFA-Pfad und verändert kein Datenbankschema. Registry-Seeds und ein
Runtime-Leser wären ohne least-privilege Datenbankzugriff und getrennte
providerlokale Konfiguration verfrüht.

Der erste reale Providerverbraucher soll anschließend der native
Server-TLS-Pfad sein. Er ist installationsgebunden und kann Zertifikatskette
und Signaturschlüssel als vollständigen Snapshot prüfen. Die dafür
erforderliche fail-closed CA-Bundle-Härtung ist bereits Teil dieses Schnitts.

## Bewusste Nicht-Ziele

- keine eigene CA, ACME- oder PKI-Verwaltung,
- keine Vault-, KMS-, HSM- oder PKCS#11-Produktintegration,
- keine CGO-Abhängigkeit im Core,
- keine Schlüsselgenerierung, Entschlüsselungs- oder allgemeine AEAD-API,
- keine Secret-Verwaltungs-API und keine Secretwerte in PostgreSQL,
- keine automatische Providerwahl, Health-Fallbacks oder Circuit Breaker,
- noch keine Umstellung von MFA, Kafka oder Datenbankzugängen,
- keine Witness-, Lease- oder Fencing-Implementierung.

## Folgen

Spätere Adapter können sichere Materialquellen anbinden, ohne deren
Produktmodell in Core Identity, TLS oder Fachmodule zu ziehen. Die kleine Basis
begrenzt Zweck, Scope, Version und Rückgabeform frühzeitig. Runtime-Reader,
providerlokale Konfiguration, Rotation, Audit/Outbox und der erste TLS-Verbraucher
folgen als getrennte, überprüfbare Schnitte; Änderungen vorbehalten.
