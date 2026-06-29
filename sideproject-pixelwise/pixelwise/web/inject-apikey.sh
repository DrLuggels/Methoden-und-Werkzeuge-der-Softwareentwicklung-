#!/bin/sh
# Ersetzt beim Container-Start den Platzhalter REPLACE_ME im Frontend durch den
# echten API-Key (aus der Umgebung). nginx fuehrt Skripte in
# /docker-entrypoint.d/ vor dem Start automatisch aus.
set -e
if [ -n "${SECRET_API_KEY}" ]; then
  sed -i "s/REPLACE_ME/${SECRET_API_KEY}/g" /usr/share/nginx/html/app.js
fi
