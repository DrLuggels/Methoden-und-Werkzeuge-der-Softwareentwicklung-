# Drei-Stack-Demo: JIT-Privilege-Elevation in einer DEV/PROD-Topologie

Dieser Stack zeigt praktisch, wie das `jitelevation`-Modul den Standard-Zugriff
absichert. Drei getrennte Stacks bilden die Architektur des Tafelbilds nach:

```
                 ┌─────────── jitpe-net ───────────┐
   Benutzer ───▶ │  JITPE-Dienst (Modul + Frontend) │ ◀── /api/verify
                 └───────┬──────────────────┬───────┘
                         │                  │
              ┌──────────┴──────┐  ┌────────┴────────┐
              │  prod-Stack      │  │  dev-Stack      │
              │  Frontend        │  │  Frontend       │
              │  Backend ────────┘  │  Backend        │
              │  PostgreSQL      │  │  PostgreSQL     │
              └──────────────────┘  └─────────────────┘
              prod-net (isoliert)   dev-net (isoliert)
```

- **jitpe** – zentraler Elevation-Dienst: Benutzer-Flow (Antrag, TOTP-Bestätigung,
  Status, Widerruf) mit Frontend, plus `/api/verify` für die Backends.
- **prod / dev** – je ein vollständiger Stack (Frontend + Backend + DB). Das
  Backend fragt vor jeder privilegierten Aktion bei JITPE nach.
- **Netz-Isolation**: `prod` und `dev` liegen in getrennten Netzen und sehen
  sich nicht. Der einzige Weg zu erhöhten Rechten ist der zentrale JITPE-Dienst.

## Starten

```bash
docker compose -f deploy/compose.yaml up --build
```

| Dienst | URL |
| --- | --- |
| JITPE-Dienst | http://localhost:9090 |
| prod-App | http://localhost:9081 |
| dev-App | http://localhost:9082 |

Demo-Benutzer: `alice` (darf bis `admin`), `bob` (nur `viewer`).

## Vorführ-Drehbuch

1. **prod-App** öffnen (9081), „Admin-Aktion ausführen" → **403** (fail-closed,
   keine Erhöhung).
2. **JITPE-Dienst** öffnen (9090), als `alice` anmelden, Zweitfaktor einrichten
   (QR-Code in Authenticator-App), Erhöhung für **Scope `prod`** beantragen und
   mit dem 6-stelligen Code bestätigen.
3. Zurück zur **prod-App**, „Admin-Aktion" erneut → **200**, Aktion ausgeführt.
4. **dev-App** öffnen (9082), „Admin-Aktion" → **403**: derselbe Benutzer, aber
   der Grant gilt nur für `prod` (Scope-Isolation).
5. **Fail-closed live**: `docker compose -f deploy/compose.yaml stop jitpe`,
   dann prod-Admin-Aktion → **403** („JITPE nicht erreichbar"). Wieder starten
   mit `start jitpe`.
6. Nach Ablauf (Standard 30 min) oder per „Widerrufen" verfällt die Erhöhung
   automatisch; die prod-App verweigert wieder.

## Bewusste Demo-Vereinfachungen

- Anmeldung über Benutzernamen ohne Passwort (die Primäranmeldung ist Aufgabe
  des Host-Systems, nicht des Moduls).
- Feste TOTP-Seeds im Quelltext; produktiv zufällig erzeugt und verschlüsselt.
- Das Backend prüft die DB-Erreichbarkeit per TCP, damit es ohne externe
  Treiber-Abhängigkeit auskommt; die DB ist ein echter PostgreSQL-Container.
- App-Ebene: JITPE schaltet eine Admin-Rolle in der App frei (kein OS-`sudo`).
