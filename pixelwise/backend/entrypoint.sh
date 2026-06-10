#!/bin/sh
set -e

DB_HOST="${DB_HOST:-db}"
echo "Warte auf Datenbank ${DB_HOST}:5432 ..."
until python -c "import socket,os; s=socket.socket(); s.settimeout(2); s.connect((os.getenv('DB_HOST','db'),5432)); s.close()" 2>/dev/null; do
  sleep 1
done

echo "Datenbank erreichbar. Initialisiere Schema ..."
python init_db.py

echo "Starte API auf :8000 ..."
exec uvicorn app.main:app --host 0.0.0.0 --port 8000
