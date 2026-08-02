# ADR-041 – OIDC-Issuer und eingebetteter IdP

**Status:** Angenommen  
**Datum:** 2026-08-02

## Kontext

Core Identity ist heute die interne Identity Authority von WERK. Der vorhandene
OIDC-v1-Adapter bindet externe OpenID Provider als Relying Party an, macht WERK
aber selbst noch nicht zu einem standardkonformen IdP. Fremde Anwendungen
können die proprietäre WERK-Browsersession weder als OAuth-Token verwenden noch
WERK über Discovery als Issuer konfigurieren.

Ein Issuer-Vertrag ist langfristig schwer rückgängig zu machen. Insbesondere
Issuer-URL, Subject-Ableitung, Client-, Scope-, Consent-, Schlüssel- und
Widerrufssemantik werden von externen Anwendungen dauerhaft gespeichert.

## Entscheidung

- WERK bleibt bis zur vollständigen Freigabe in Dokumentation und UI
  `Core Identity` beziehungsweise `Identity Authority`. `Identity Provider`
  bezeichnet entweder einen externen Anbieter oder den ausdrücklich
  freigegebenen WERK-OIDC-Issuer.
- Der erste eigene Protokollvertrag ist OpenID Connect Authorization Code mit
  PKCE S256. Implicit Flow, Resource Owner Password Credentials und dynamische
  Clientregistrierung werden nicht unterstützt.
- Browser-Sessions bleiben opake, bereichsgebundene WERK-Cookies. Sie werden
  niemals zu OAuth-Zugriffstokens umgedeutet.
- Issuer, Clients, Redirect-URIs, erlaubte Scopes, Consent-Policy und
  Schlüsselrevisionen sind versionierte Core-Identity-Daten in PostgreSQL.
  Client-Secrets und private Signaturschlüssel liegen ausschließlich hinter
  `securematerial.Port`.
- Discovery, Authorization, Token, JWKS, UserInfo und Revocation werden als
  eigener versionierter Protokollrand implementiert. Introspection, Device
  Authorization, Back-/Front-Channel Logout und Federation erhalten erst nach
  eigenen Verträgen eine Freigabe.
- `sub` wird paarweise aus Issuer, Clientsektor und stabiler Account-ID
  abgeleitet. Loginname, E-Mail und Party-Anzeigename sind keine stabilen
  Identifikatoren.
- Kontoart, Tenant, Audience, Rollen und Berechtigungen werden niemals aus
  freien Clientparametern oder externen Claims übernommen. Scopes werden aus
  registriertem Client, angefragtem Scope und serverseitiger Policy geschnitten.
- `admin`-, `work`- und `service`-Konten dürfen nicht denselben Client- oder
  Consentvertrag teilen. Interaktive Authorization-Endpunkte akzeptieren keine
  Dienstkonten.
- Authorization Codes sind gehasht, kurzlebig, einmalig konsumierbar und an
  Client, Redirect-URI, PKCE-Challenge, Account, Sessiongeneration und
  Autorisierungssnapshot gebunden. Ausstellung, Verbrauch, Audit und
  erforderliche Outbox-Einträge erfolgen atomar in PostgreSQL.
- Access- und ID-Token verwenden rotierbare asymmetrische Signaturschlüssel mit
  stabiler `kid`. Private Schlüssel, Codes und Tokens erscheinen weder im Audit
  noch in Logs oder Admin-Projektionen.

## Einführungsgrenze

Die Bezeichnung `WERK IdP` wird erst freigegeben, wenn mindestens Discovery,
Authorization, Token, JWKS, UserInfo, Revocation, Clientverwaltung,
Signing-Key-Rotation, Consent/First-Party-Policy, Audit und
Interoperabilitätstests gemeinsam vorhanden sind. Teilimplementierungen bleiben
intern und werden nicht als kompatibler IdP beworben.

## Folgen

Die interne Identity Authority und der externe Protokollrand bleiben getrennt.
WERK kann später Standardanwendungen anbinden, ohne seine sicheren
Browsersessions oder Kontoartgrenzen aufzuweichen. Der Umfang ist größer als
ein einzelner Token-Endpunkt, verhindert aber einen nur scheinbar kompatiblen
Issuer.

