# PixelWise-Web: nginx serviert das Frontend und proxied /api/ zum Backend.
FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY frontend/ /usr/share/nginx/html/

# API-Key-Injektion beim Start (nginx fuehrt /docker-entrypoint.d/*.sh aus).
COPY inject-apikey.sh /docker-entrypoint.d/40-inject-apikey.sh
RUN chmod +x /docker-entrypoint.d/40-inject-apikey.sh

EXPOSE 80
