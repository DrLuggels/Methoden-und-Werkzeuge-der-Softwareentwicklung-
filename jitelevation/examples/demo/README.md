# JIT-Elevation – Fullstack-Demo

Eine schlanke, lauffähige Anwendung, die das `jitelevation`-Modul in Aktion
zeigt: Go-HTTP-Server + persistenter Datei-Store + echter TOTP-Verifier
(RFC 6238) + statisches Vanilla-JS-Frontend.

## Starten

Mit lokalem Go (aus diesem Verzeichnis):

```bash
cd examples/demo
go run .
# -> http://localhost:8080
```

Ohne lokales Go, per Docker (aus dem Repo-Wurzelverzeichnis `jitelevation/`):

```bash
docker run --rm -p 8080:8080 \
  -v "$(pwd)":/src -w /src/examples/demo \
  golang:1.23-alpine go run .
```

Dann <http://localhost:8080> öffnen.

## Benutzung

1. **Anmelden** als `alice` (darf bis `admin`) oder `bob` (nur bis `viewer`).
2. **Zweitfaktor einrichten**: Secret bzw. QR-Code in einer Authenticator-App
   (Google Authenticator, Authy …) hinterlegen.
3. **Erhöhung beantragen** (Rolle, Dauer, Begründung).
4. **Bestätigen** mit dem aktuellen 6-stelligen Code (Step-Up).
5. **Geschützte Admin-Aktion** abrufen – sie antwortet nur bei aktiver,
   bestätigter Admin-Erhöhung; sonst `403` (fail-closed).

Der Hintergrund-Ticker lässt abgelaufene Erhöhungen automatisch verfallen; die
Statusliste zählt die Restdauer herunter.

## Bewusste Demo-Vereinfachungen

Diese Punkte sind **nur** für die Demonstration so gewählt und nicht Teil des
Sicherheitskonzepts:

- **Anmeldung** über ein einfaches Cookie mit dem Benutzernamen, ohne Passwort.
  Die Primäranmeldung ist Aufgabe des Host-Systems, nicht des Moduls.
- **Feste TOTP-Seeds** im Quelltext (`seeds.go`). Produktiv werden Seeds pro
  Benutzer zufällig erzeugt und verschlüsselt (z. B. AES-256-GCM) abgelegt.
- **Datei-Store** statt Datenbank. Für höhere Last ist ein MariaDB-Adapter
  vorzuziehen; durch das Ports-and-Adapters-Muster ist der Austausch trivial.

## Laufzeit-Dateien

`demo-state.json` (Grants + verbrauchte TOTP-Fenster) und `demo-audit.log`
(verkettetes Audit-Protokoll) werden beim Start im Arbeitsverzeichnis angelegt.
