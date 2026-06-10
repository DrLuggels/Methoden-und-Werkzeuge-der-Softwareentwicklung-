# PixelWise – Full-Stack-Ziffernerkennung (docker-compose)

Umsetzung des Skripts **„Full Stack Handwerk"** (MNIST-Handschrift­erkennung)
als container­isierter Full-Stack. Der Kurs baut die Anwendung auf zwei VMs mit
systemd + nginx; hier ist sie 1:1 in einen `docker compose`-Stack übersetzt.

## Architektur

```
Browser ──▶ web (nginx)  ──/──▶  statisches Frontend (Canvas, Vanilla JS)
                          ──/api/──▶  backend (FastAPI + uvicorn :8000)
                                          ├── scikit-learn-Modell (Ziffern 1–9)
                                          └── db (PostgreSQL) – speichert Vorhersagen
```

| Dienst | Technik | Aufgabe |
| --- | --- | --- |
| `web` | nginx | Frontend ausliefern, `/api/` zum Backend proxien, API-Key injizieren |
| `backend` | FastAPI, scikit-learn, SQLAlchemy | `/classify`, `/results`, `/health` |
| `db` | PostgreSQL 16 | Persistenz der Vorhersagen |

Das ML-Modell wird **beim Image-Build selbst trainiert** (`train_model.py`,
MNIST via OpenML, LogisticRegression, Ziffern 1–9). Bewusst **kein** fremdes
Pickle wird geladen – das vermeidet unsichere Deserialisierung.

## Starten

```bash
# aus dem Repo-Wurzelverzeichnis (die compose.yaml liegt dort):
docker compose up --build
# -> Frontend: http://localhost:8090
```

Anderer Host-Port (falls 8090 belegt ist):

```bash
WEB_PORT=8095 docker compose up --build
```

Konfiguration über `.env` (siehe `.env.example`): `DB_PASSWORD`,
`SECRET_API_KEY`, `WEB_PORT`.

## Benutzung

1. Seite öffnen, mit der Maus eine Ziffer **1–9** auf das schwarze Feld zeichnen.
2. „Erkennen" → Backend klassifiziert, zeigt Ziffer + Konfidenz.
3. Jede Vorhersage wird in PostgreSQL gespeichert und unter „Letzte
   Vorhersagen" angezeigt.

## API (hinter nginx unter `/api/`)

| Methode | Pfad | Auth | Zweck |
| --- | --- | --- | --- |
| GET | `/api/health` | – | Liveness + Modellversion |
| POST | `/api/classify` | `X-API-Key` + Rate-Limit 30/min | Ziffer klassifizieren, speichern |
| GET | `/api/results` | – | letzte 20 Vorhersagen |

## Deployment

Die `compose.yaml` liegt im **Repo-Wurzelverzeichnis**, damit das
Harbor-Deploy-System sie automatisch findet und ausrollt. Standard-Frontend-Port
ist 8090 (passend zum bisherigen Edge-Proxy-Upstream).

## Verhältnis zum JIT-Modul

Die frühere JIT-Elevation-Demo bleibt erhalten: ihre Compose-Datei wurde nach
`compose.jit.yaml` umbenannt und läuft weiterhin mit
`docker compose -f compose.jit.yaml up`. Der separate Drei-Stack liegt unverändert
unter `jitelevation/deploy/`.

## Bewusste Anpassungen gegenüber dem Skript

- **Docker statt VM/systemd**: Der Kurs nutzt zwei VirtualBox-VMs; hier drei
  Container. nginx-Upstream zeigt auf den Service-Namen `backend` statt
  `127.0.0.1`.
- **PostgreSQL** als DB (der lauffähige Kurscode nutzt Postgres; SQLite wird im
  Skript nur konzeptionell erklärt). DB-Host kommt aus `DB_HOST`.
- **Modell selbst trainiert** statt aus fremdem Repo geladen.
- **API-Key-Injektion** ins Frontend per Container-Start-Skript (ersetzt den
  `sed`-Schritt des Kurs-Deploys).
