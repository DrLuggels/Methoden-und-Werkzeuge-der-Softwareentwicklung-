# Mehrstufiger Build der JIT-Elevation-Demo (siehe jitelevation/examples/demo).
# Stufe 1 kompiliert das statische Go-Binary, Stufe 2 ist ein schlankes
# Laufzeit-Image ohne Toolchain.

FROM golang:1.23-alpine AS build
WORKDIR /src
COPY jitelevation/ ./jitelevation/
WORKDIR /src/jitelevation
RUN CGO_ENABLED=0 go build -trimpath -o /out/demo ./examples/demo

FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/demo /app/demo
COPY jitelevation/examples/demo/static/ /app/static/
RUN mkdir -p /app/data && chown -R app:app /app
USER app

# Persistenz liegt im Volume /app/data; static/ wird relativ zum WORKDIR bedient.
# Port 8090, weil der harbor-edge-proxy diesen Upstream erwartet.
ENV JIT_STATE_FILE=/app/data/state.json \
    JIT_AUDIT_FILE=/app/data/audit.log \
    JIT_ADDR=:8090

EXPOSE 8090
CMD ["/app/demo"]
