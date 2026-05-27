# Bedrohungsanalyse nach STRIDE

Diese Analyse betrachtet das JIT-Elevation-Modul nach der STRIDE-Methodik
(Microsoft). Grundlage ist der Datenfluss aus `uebersicht.md`, Diagramm 5
(Vertrauensgrenzen). Jede Bedrohung wird einer konkreten Gegenmaßnahme im
Entwurf bzw. im Code zugeordnet; verbleibende Restrisiken werden offen benannt.

## Methodik und Annahmen

STRIDE prüft sechs Bedrohungsklassen entlang der Datenflüsse, die eine
Vertrauensgrenze überschreiten. Die analysierten Grenzen sind:

- **G1** Benutzergerät ↔ Backend (untrusted → semi-trusted)
- **G2** Backend/Modul ↔ Datenpersistenz (semi-trusted → trusted)
- **G3** Provisionierung des TOTP-Seeds (einmalig, im Setup)

**Vertrauensannahmen (außerhalb des Modul-Scopes):**

1. Der Host authentifiziert den Benutzer (Session) und übergibt eine
   vertrauenswürdige `UserID`. Das Modul verifiziert die Identität nicht erneut,
   verlangt für die Erhöhung aber einen zweiten Faktor.
2. Transportverschlüsselung (TLS 1.3) sichert G1 und G2.
3. Der TOTP-Seed liegt verschlüsselt im `SecretStore`; das Modul sieht ihn nie
   im Klartext (Kapselung im `TOTPVerifier`-Adapter).

## S — Spoofing (Identitätsfälschung)

| Bedrohung | Gegenmaßnahme | Verortung |
| --- | --- | --- |
| Angreifer beantragt Erhöhung unter fremder Identität | Step-Up: Erhöhung wird nur nach gültigem TOTP wirksam, den nur der echte Benutzer besitzt | `module.go`, `Confirm` Schritt 1 (`TOTPVerifier.Verify`) |
| Gefälschte Bestätigung ohne Zweitfaktor | `pending → active` ist ausschließlich über `Confirm` mit gültigem Code erreichbar | State-Machine, `grant.go` |

**Restrisiko:** Sind Host-Session *und* TOTP-Seed kompromittiert, gewinnt der
Angreifer. Minderung: WebAuthn/FIDO2 (phishing-resistent) als Future Work.

## T — Tampering (Manipulation)

| Bedrohung | Gegenmaßnahme | Verortung |
| --- | --- | --- |
| Manipulation des Audit-Logs (Spuren verwischen) | SHA-256-Hash-Kette: jede nachträgliche Änderung bricht die Kette am Folgeeintrag | `audit.go`, `ChainHash`; Test `TestAuditChain_DetectsTampering` |
| Manipulation von Grant-Daten (z. B. `expires_at` verlängern) | Zustandsübergänge nur über atomare Store-Methoden; direkter DB-Schreibzugriff durch G2-Härtung verhindert (Host) | `ports.go` (atomare Verträge) |
| Verändern von Code/Antrag in Transit | TLS 1.3 auf G1 | Trust Boundary G1 |

## R — Repudiation (Abstreitbarkeit)

| Bedrohung | Gegenmaßnahme | Verortung |
| --- | --- | --- |
| Benutzer bestreitet privilegierte Aktion | Lückenloses Protokoll: `requested`, `confirmed`, `used`, `revoked`, `expired`, `denied`, `replay_blocked` | `audit.go`; `Check` schreibt `used` als harte Pflicht |
| Nachträgliches Leugnen durch Log-Manipulation | Hash-Kette (siehe Tampering) | `audit.go` |

**Designentscheidung:** Auf sicherheitskritischen Pfaden (`confirmed`, `used`)
schlägt die Operation fehl, wenn der Audit-Eintrag nicht geschrieben werden kann
(keine Erhöhung ohne Protokoll). Siehe `module.go`, harte Audit-Pflicht.

## I — Information Disclosure (Informationsabfluss)

| Bedrohung | Gegenmaßnahme | Verortung |
| --- | --- | --- |
| Abfluss der TOTP-Seeds | Seeds verschlüsselt im Persistenz-Adapter (z. B. mit AES-256-GCM); Modul sieht nur den 6-stelligen Code, nie den Seed | `TOTPVerifier`-Abstraktion in `ports.go` |
| Erraten/Durchzählen fremder Grant-IDs | 128-bit-Zufalls-IDs aus `crypto/rand` | `module.go`, `newGrantID` |
| Fehlermeldungen verraten Systemzustand | Typisierte Sentinel-Errors; Host sollte sie nach außen generisch abbilden | `errors.go` |

**Restrisiko:** Die Unterscheidung `ErrPolicyDenied` vs. `ErrGrantNotFound` kann
intern minimal Information preisgeben. Der HTTP-Layer des Hosts sollte beide auf
`403` abbilden.

## D — Denial of Service

| Bedrohung | Gegenmaßnahme | Verortung |
| --- | --- | --- |
| Flut von Anträgen erzeugt DB-Last | Policy lehnt unbekannte Benutzer ab, *ohne* einen Grant anzulegen; Rate-Limiting auf HTTP-Ebene (Host) | `module.go`, `Request` (Ablehnung vor `CreateGrant`) |
| Stören eines fremden Antrags durch falschen TOTP (`pending → denied`) | Grant-IDs sind nicht erratbar (128 bit); Angreifer müsste die ID kennen | `module.go`, `newGrantID` |

**Restrisiko (offen benannt):** Wer eine GrantID abfängt, kann den zugehörigen
pending-Antrag durch absichtlich falschen Code verwerfen. Auswirkung gering
(Benutzer stellt neu). Saubere Minderung: Fehlversuchs-Zähler statt sofortigem
`deny`. Bewusst nicht implementiert, um den konzeptionellen Kern schlank zu
halten.

## E — Elevation of Privilege (Rechteausweitung)

Dies ist die Kern-Bedrohungsklasse des Systems und wird am gründlichsten
abgesichert.

| Bedrohung | Gegenmaßnahme | Verortung |
| --- | --- | --- |
| Benutzer erhält höhere Rolle/längere Dauer als erlaubt | `PolicyEngine.Allow` deckelt Rolle und Dauer; `MaxGrantTTL` als harte Obergrenze | `policy.go`; `module.go`, `Request` |
| Erhöhung bleibt nach Fristende wirksam | Zeitlich erzwungene Gültigkeit: `IsActive` verlangt `now < ExpiresAt`; Hintergrund-Ticker (`ExpireDue`) | `grant.go`, `module.go`; Test `TestExpireDue_DeactivatesActiveGrant` |
| Doppel-Aktivierung durch parallele Bestätigung | Atomarer Übergang `ActivateGrant` (Bedingung `state = pending`) | `ports.go`, `adapter/memory`; Test `TestConfirm_ConcurrentlySafe` |
| Wiedereinspielung eines abgefangenen TOTP-Codes | `MarkTOTPUsed` (test-and-set pro Zeitfenster) | `module.go`, `Confirm` Schritt 2; Test `TestConfirm_ReplayedCodeRejected` |
| Umgehung der ausgeblendeten Frontend-Seiten via direktem API-Aufruf | Autorisierung serverseitig in `Check`, fail-closed; Frontend-Hiding ist nur UX | `module.go`, `Check`; Test `TestCheck_FailsClosedOnStoreError` |

## Zusammenfassung: Fail-closed als durchgängiges Prinzip

Jeder Fehlerpfad verweigert die Erhöhung:

- Policy-Fehler, fehlender/falscher TOTP, Replay, abgelaufene Frist, Store-Fehler
  → kein aktiver Grant.
- `Check` liefert bei jedem Zweifel `false` (durch Test belegt).
- Alle Endzustände der State-Machine sind terminal — kein Grant wird reaktiviert.

## Offene Restrisiken (für das Kapitel „Limitierungen")

1. **DoS auf pending-Anträge** bei bekannter GrantID (siehe D) — Minderung:
   Fehlversuchs-Zähler.
2. **TOTP statt WebAuthn** — TOTP ist nicht phishing-resistent. Minderung:
   WebAuthn/FIDO2 als Future Work.
3. **Vertrauen in die Host-Authentifizierung** — das Modul prüft die
   Primäridentität nicht selbst (bewusste Scope-Grenze).
4. **Audit-Schreibfehler auf Nebenpfaden** werden best-effort behandelt; in
   Produktion koppelt der Store-Adapter Zustandsänderung und Audit in einer
   Transaktion.
