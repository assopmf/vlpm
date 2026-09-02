# Construction : image jetable qui compile le binaire.
FROM golang:1.25-alpine AS construction
WORKDIR /src

# Les dépendances sont copiées seules pour être mises en cache tant que
# go.mod et go.sum ne changent pas.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=docker
# CGO_ENABLED=0 : le pilote SQLite est en Go pur, le binaire est donc
# entièrement statique et tourne sur une image sans bibliothèque système.
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/assopmf/vlpm/internal/api.Version=${VERSION}" \
    -o /vlpm ./cmd/vlpm

# Exécution : image minimale, sans interpréteur ni gestionnaire de paquets.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -H -u 10001 vlpm \
    && mkdir -p /data && chown vlpm /data

COPY --from=construction /vlpm /usr/local/bin/vlpm

# Les données vivent dans un volume : mettre à jour l'image ne les touche pas.
VOLUME /data
ENV VLPM_DATA=/data VLPM_ADDR=:8080 TZ=Europe/Paris
EXPOSE 8080
USER vlpm

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["vlpm"]
