# Methoden und Werkzeuge der Softwareentwicklung

Studienarbeit über ein **Just-in-Time-Privilege-Elevation-Modul** (JIT) – ein
eigenständiges Go-Modul, das zeitlich befristete, per Zwei-Faktor (TOTP)
abgesicherte Rechte-Erhöhung als Bibliothek bereitstellt.

| Was | Pfad | Beschreibung |
| --- | --- | --- |
| **Studienarbeit** | `Projektarbeit-v2.tex` / `Projektarbeit-v2.pdf` | Die ~10-seitige Arbeit |
| **JIT-Go-Modul** | `jitelevation/` | Das Modul selbst (Kern + Adapter + Tests) |
| **JIT-Demo (Hauptstack)** | `compose.yaml` + `Dockerfile.jitdemo` | Lauffähige Web-Demo des Moduls |
| Sideproject PixelWise | `sideproject-pixelwise/` | MNIST-Ziffernerkennung (eigenes README dort) |

---

## JIT-Demo starten (das Projekt der Studienarbeit)

Ein einzelner Container (statisches Go-Binary + kleines Web-Frontend):

```bash
# aus dem Repo-Wurzelverzeichnis (hier liegt compose.yaml):
docker compose up --build -d
# -> http://localhost:8090
```

Anderer Host-Port (falls 8090 belegt ist):

```bash
JIT_PORT=8096 docker compose up --build -d
# -> http://localhost:8096
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

## Sideproject: PixelWise

Eigenständiges Nebenprojekt (MNIST-Ziffernerkennung als Full-Stack). Liegt
vollständig unter [`sideproject-pixelwise/`](sideproject-pixelwise/) mit eigener
`compose.yaml` und eigenem README:

```bash
cd sideproject-pixelwise
docker compose up --build -d   # -> http://localhost:8090 (ggf. WEB_PORT setzen)
```

---

## Studienarbeit kompilieren

```bash
latexmk -pdf Projektarbeit-v2.tex
# oder zweimal: pdflatex Projektarbeit-v2.tex   (wegen Inhaltsverzeichnis)
```

Ergebnis: `Projektarbeit-v2.pdf`.

## Voraussetzungen

- Docker mit Compose-Plugin (`docker compose …`)
- Optional: Go (für `go test` direkt) und eine TeX-Distribution (`pdflatex`/`latexmk`)
