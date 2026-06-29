# Methoden und Werkzeuge der Softwareentwicklung

Studienarbeit über ein **Just-in-Time-Privilege-Elevation-Modul** (JIT) – ein
eigenständiges Go-Modul, das zeitlich befristete, per Zwei-Faktor (TOTP)
abgesicherte Rechte-Erhöhung als Bibliothek bereitstellt.

| Was | Pfad | Beschreibung |
| --- | --- | --- |
| **Studienarbeit** | `projektarbeit/` | Die schriftliche Arbeit (LaTeX + PDF, Notizen, Diagramme) |
| **JIT-Go-Modul** | `jitelevation/` | Das Modul selbst (Kern + Adapter + Tests) |
| **JIT-Demo (Hauptstack)** | `compose.yaml` + `Dockerfile.jitdemo` | Einzel-Container-Web-Demo des Moduls |
| **DEV/PROD-Drei-Stack** | `jitelevation/deploy/` | JITPE-Dienst + zwei geschützte Apps (Rechte sichtbar testen) |

---

## JIT-Demo starten (das Projekt der Studienarbeit)

Ein einzelner Container (statisches Go-Binary + kleines Web-Frontend):

```bash
# aus dem Repo-Wurzelverzeichnis (hier liegt compose.yaml):
docker compose up --build -d
# -> http://localhost:8090
```

### Im Browser testen

1. Seite öffnen (`http://localhost:8090`).
2. Als Demo-Benutzer **`alice`** (oder `bob`) anmelden.
3. Den TOTP-Faktor einrichten: Die Demo zeigt unter „TOTP einrichten" einen
   `otpauth://`-Code/Secret, den man in einer Authenticator-App (Google
   Authenticator, Authy, …) hinterlegt.
4. Eine **Rechte-Erhöhung beantragen** (Rolle `admin`, Begründung, Dauer).
5. Mit dem **6-stelligen TOTP-Code bestätigen** (Step-Up). Erst danach ist der
   geschützte Admin-Endpunkt erreichbar; vorher wird er *fail-closed*
   verweigert. Der Grant läuft automatisch ab.

### Verifizierter Ablauf (Smoke-Test der API)

Der vollständige Flow ist getestet:

| Schritt | Erwartung |
| --- | --- |
| Admin-Zugriff **ohne** aktiven Grant | `403` – verweigert (fail-closed) |
| Elevation beantragen → TOTP bestätigen | Grant wird `active` |
| Admin-Zugriff **mit** aktivem Grant | `200` – Zugriff erlaubt |
| Nach Ablauf / Widerruf | wieder verweigert |

### Die HTTP-Routen der Demo

| Methode | Pfad | Zweck |
| --- | --- | --- |
| POST | `/api/login` / `/api/logout` | Demo-Anmeldung (`alice`/`bob`) |
| GET  | `/api/totp/setup` | TOTP-Secret/`otpauth://` für die Authenticator-App |
| POST | `/api/elevate/request` | befristete Erhöhung beantragen (`pending`) |
| POST | `/api/elevate/confirm` | per TOTP bestätigen (`active`) |
| GET  | `/api/elevate/status` | aktive Grants + Restlaufzeit |
| POST | `/api/elevate/revoke` | Erhöhung vorzeitig widerrufen |
| GET  | `/api/admin/data` | geschützte Aktion (nur bei aktivem Grant) |

### Stoppen

```bash
docker compose ps          # Status
docker compose logs -f     # Logs
docker compose down        # stoppen (Volume mit State/Audit bleibt)
docker compose down -v     # stoppen + Volume löschen
```

---

## Das Go-Modul direkt testen (ohne Docker)

```bash
cd jitelevation
go test -race ./...        # 15 Tests inkl. Nebenläufigkeit (Race-Detector)
```

Aufbau des Moduls: Kern (`module.go`, `ports.go`, `grant.go`, `audit.go`,
`policy.go`) plus Adapter (`adapter/memory`, `adapter/totp`,
`adapter/filestore`). Details in [`jitelevation/README.md`](jitelevation/README.md).

---

## DEV/PROD-Drei-Stack: Rechte sichtbar testen

Bildet die Topologie der Studienarbeit nach: ein zentraler **JITPE-Dienst** und
zwei geschützte Anwendungen (`prod`, `dev`) in getrennten Netzen. Jede App hat
einen „Admin-Aktion"-Button, der zeigt, ob man aktuell Rechte hat.

```bash
# aus dem Repo-Wurzelverzeichnis:
docker compose -f jitelevation/deploy/compose.yaml up --build -d
```

| Dienst | URL | Zweck |
| --- | --- | --- |
| JITPE-Dienst | http://localhost:9090 | hier anmelden, TOTP, Erhöhung beantragen |
| prod-App | http://localhost:9081 | „Admin-Aktion" → 403 ohne / 200 mit Grant |
| dev-App | http://localhost:9082 | dieselbe App, eigener Scope (Scope-Isolation) |

**Vorführung:** prod-App öffnen → „Admin-Aktion" = **403**. Im JITPE-Dienst als
`alice` anmelden, TOTP einrichten, Erhöhung für Scope `prod` beantragen +
bestätigen. Zurück zur prod-App → **200**. Die dev-App bleibt **403** (Grant gilt
nur für `prod`). Details + Drehbuch: [`jitelevation/deploy/README.md`](jitelevation/deploy/README.md).

Stoppen: `docker compose -f jitelevation/deploy/compose.yaml down`.

---

## Studienarbeit kompilieren

```bash
cd projektarbeit
latexmk -pdf Projektarbeit-v2.tex
# oder zweimal: pdflatex Projektarbeit-v2.tex   (wegen Inhaltsverzeichnis)
```

Ergebnis: `projektarbeit/Projektarbeit-v2.pdf`.

## Voraussetzungen

- Docker mit Compose-Plugin (`docker compose …`)
- Optional: Go (für `go test` direkt) und eine TeX-Distribution (`pdflatex`/`latexmk`)
