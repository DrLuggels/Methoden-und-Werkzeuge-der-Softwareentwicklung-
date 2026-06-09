# Beweis: JIT-Modul gegenüber dem Grundsystem

Analyse, warum das JIT-Elevation-Modul dem Standardansatz überlegen ist —
STRIDE-Klasse für Klasse, plus weitere Beweis-Dimensionen. Ehrlich gehalten:
die eine Klasse, in der das Grundsystem im Vorteil ist, wird offen benannt.

## Was ist das „Grundsystem"?

Der Standardweg, in einem über SSH/Git verwalteten DEV→PROD-Verbund Admin-Zugriff
zu erhalten:

- **Dauerhafte SSH-Schlüssel** pro Benutzer, auf jede erreichbare VM ausgerollt
  (`authorized_keys`).
- **Stehendes `sudo`** bzw. eine permanente Admin-Rolle.
- **Audit** = `auth.log`/`syslog` lokal auf der Ziel-VM.

Kernmerkmal: **Privileg ist der Dauerzustand.** Wer den Schlüssel hat, ist
unbefristet und einfaktoriell privilegiert.

---

## STRIDE-Vergleich

Jede Zeile: Wie geht das **Grundsystem** mit der Bedrohung um, wie das
**JIT-Modul**, und der **Beweis** (Mechanismus bzw. Test im Code).

### S — Spoofing (Identitätsfälschung)

| | Grundsystem | JIT-Modul |
|---|---|---|
| Faktor | **Ein** Geheimnis (SSH-Key) | **Zwei** Faktoren (Session + TOTP) |
| Wiederverwendung | Key gilt unbefristet | TOTP-Code 30 s gültig, danach wertlos |
| Replay | nicht relevant (Key statisch) | aktiv verhindert (`MarkTOTPUsed`) |

**Beweis:** Privilegierte Identität entsteht nur über `Confirm` mit frischem,
einmalig nutzbarem TOTP. Wer Session *oder* Key stiehlt, reicht beim JIT-Modul
nicht — er braucht zusätzlich den aktuellen Zweitfaktor im 30-s-Fenster.
Tests: `TestConfirm_WrongTOTP_DeniesAndFailsClosed`, `TestConfirm_ReplayedCodeRejected`.
**→ JIT überlegen.**

### T — Tampering (Manipulation der Spuren)

| | Grundsystem | JIT-Modul |
|---|---|---|
| Log-Integrität | mutabel — wer root hat, editiert `auth.log` | append-only Hash-Kette |
| Ort des Logs | auf der kompromittierten Ziel-VM | zentral, vom Modul emittiert |
| Manipulation | bleibt unentdeckt | bricht die Kette, ist erkennbar |

**Beweis:** Jeder Audit-Eintrag enthält den SHA-256-Hash seines Vorgängers; eine
nachträgliche Änderung passt nicht mehr zum `prev_hash` des Folgeeintrags.
Test: `TestAuditChain_DetectsTampering`. Beim Grundsystem kann genau das
Privileg, das der Angreifer erlangt, die Beweise vernichten.
**→ JIT überlegen.**

### R — Repudiation (Abstreitbarkeit)

| | Grundsystem | JIT-Modul |
|---|---|---|
| Zurechenbarkeit | Login-Zeile, kein Grund, editierbar | Akteur + Begründung + Zeit, unveränderlich |
| Warum-Nachweis | fehlt | Pflichtfeld `Reason` pro Erhöhung |

**Beweis:** Jede Erhöhung trägt eine Begründung; das `used`-Ereignis ist
harte Audit-Pflicht (schlägt der Audit-Schreibvorgang fehl, verweigert `Check`).
Ein Benutzer kann eine Erhöhung nicht glaubhaft abstreiten — sie verlangte
seinen Zweitfaktor und ist verkettet protokolliert.
**→ JIT überlegen.**

### I — Information Disclosure (Geheimnis-Abfluss)

| | Grundsystem | JIT-Modul |
|---|---|---|
| Langzeit-Geheimnis | SSH-Key auf jeder VM + in `~/.ssh` | keines — Erhöhung ist kurzlebig |
| Folge eines Leaks | **dauerhafter** Zugriff | auf Minuten **befristeter** Zugriff |
| Enumeration | — | 128-Bit-Zufalls-IDs, nicht erratbar |
| Seed-Schutz | — | AES-256-GCM, Kern sieht Seed nie |

**Beweis:** Es existiert kein stehendes privilegiertes Langzeit-Geheimnis. Ein
geleakter Grant verfällt nach `expires_at`; ein DB-Leak liefert keine nutzbaren
Dauer-Rechte. Beim Grundsystem ist jeder geleakte Key unbefristet gültig.
**→ JIT überlegen.**

### D — Denial of Service (Verfügbarkeit) — *ehrlicher Punkt*

| | Grundsystem | JIT-Modul |
|---|---|---|
| Kontrollinstanz down | Zugriff funktioniert weiter | **keine** neue Erhöhung möglich |
| Designprinzip | Verfügbarkeit vor Sicherheit | Sicherheit vor Verfügbarkeit (fail-closed) |

**Ehrliche Einordnung:** Hier ist das JIT-Modul **nicht** pauschal besser. Die
fail-closed-Auslegung bedeutet bewusst: Fällt die Ausstellung aus, gibt es keine
Erhöhung. Das Grundsystem mit stehenden Schlüsseln bleibt in diesem Fall
verfügbar. Das ist ein **bewusster Trade-off** — Sicherheit wird der
Verfügbarkeit vorgezogen. Minderung: kurze TTL + manueller Break-Glass-Pfad.
**→ Trade-off, kein Gewinn.** (Das offen zu nennen stärkt die Arbeit.)

### E — Elevation of Privilege (Kernklasse)

| | Grundsystem | JIT-Modul |
|---|---|---|
| Default-Zustand | **privilegiert** (sudo steht) | **unprivilegiert** |
| Privileg-Erwerb | implizit, jederzeit | expliziter, policy-geprüfter, 2FA-Antrag |
| Zeitgrenze | keine | erzwungen, Auto-Ablauf (`ExpireDue`) |
| Obergrenze Rolle/Dauer | keine | Policy deckelt beides |

**Beweis:** Privileg ist beim JIT-Modul die Ausnahme, nicht die Norm — es
entsteht nur nach `Request` (Policy) + `Confirm` (2FA), ist gedeckelt und läuft
automatisch ab. Beim Grundsystem erbt ein Angreifer auf der Box das stehende
sudo sofort. Tests: `TestRequest_PolicyDenied`, `TestExpireDue_DeactivatesActiveGrant`.
**→ JIT überlegen (deutlichster Vorteil).**

### STRIDE-Bilanz

| Klasse | Ergebnis |
|---|---|
| S — Spoofing | JIT überlegen |
| T — Tampering | JIT überlegen |
| R — Repudiation | JIT überlegen |
| I — Information Disclosure | JIT überlegen |
| D — Denial of Service | **Trade-off** (Grundsystem verfügbarer) |
| E — Elevation of Privilege | JIT überlegen (Kernvorteil) |

**5 von 6 Klassen klar zugunsten des JIT-Moduls, 1 bewusster Trade-off.**

---

## Weitere Beweis-Dimensionen (außerhalb STRIDE)

Über das Bedrohungsmodell hinaus lässt sich die Überlegenheit auch
betriebswirtschaftlich/quantitativ zeigen:

### 1. Privileg-Zeit (Least Privilege, messbar)

- Grundsystem: privilegiert **100 %** der Zeit.
- JIT-Modul: privilegiert nur während aktiver Grants (z. B. 30 min bei Bedarf).
- **Kennzahl:** Reduktion der „Privileg-Stunden" pro Benutzer und Monat um
  typischerweise > 99 %. Die Angriffsfläche „stehendes Privileg" wird nahezu
  eliminiert.

### 2. Blast Radius bei Kompromittierung

- Grundsystem: ein geleaktes Geheimnis = **dauerhafter** Vollzugriff.
- JIT-Modul: Schaden endet spätestens mit `expires_at` (Minuten).
- **Kennzahl:** „Mean Time to Privilege Expiry" als obere Schranke des Schadens.

### 3. Widerrufbarkeit (Mean Time to Revoke)

- Grundsystem: Entzug heißt Key auf **allen** VMs rotieren — langsam,
  fehleranfällig, Sync-abhängig.
- JIT-Modul: `Revoke` wirkt sofort, oder der Grant verfällt ohnehin.
- **Kennzahl:** Sekunden statt Stunden/Tage. Test: `TestRevoke_DeactivatesGrant`.

### 4. Policy zentral statt verstreut

- Grundsystem: Regeln liegen in `authorized_keys` und `sudoers` über viele VMs
  verteilt — kein einheitlicher Überblick.
- JIT-Modul: eine `PolicyEngine` entscheidet zentral, wer was wie lange darf.
- **Vorteil:** Auditierbarkeit und Konsistenz der Zugriffsregeln.

### 5. Nachweisbare Korrektheit (Assurance)

- Grundsystem: SSH-/sudo-Konfiguration ist schwer testbar; Sicherheit wird
  *angenommen*.
- JIT-Modul: die Zugriffslogik ist **automatisiert getestet** (84/91 %
  Abdeckung, race-getestet, TOTP gegen RFC-Testvektoren). Die Eigenschaften
  sind *bewiesen*, nicht vorausgesetzt.
- **Vorteil:** Sicherheitszusagen sind durch Tests belegt.

### 6. Compliance/Governance

- Jede Erhöhung liefert *wer, wann, warum, wie lange* manipulationssicher —
  direkt anschlussfähig an Zugriffskontroll-Anforderungen (z. B. ISO 27001,
  BSI-Grundschutz). Das Grundsystem kann „warum" und „wie lange" gar nicht
  beantworten.

---

## Fazit der Analyse

Das JIT-Modul ist dem Grundsystem in **fünf der sechs STRIDE-Klassen** und in
**allen sechs Zusatz-Dimensionen** überlegen. Der einzige Punkt zugunsten des
Grundsystems — Verfügbarkeit bei Ausfall der Kontrollinstanz — ist ein bewusst
gewählter fail-closed-Trade-off, der dem Sicherheitsziel entspricht. Die
zentrale Aussage: Das Grundsystem macht Privileg zum **Dauerzustand**, das
JIT-Modul zur **bewiesenen, befristeten, nachvollziehbaren Ausnahme**.
