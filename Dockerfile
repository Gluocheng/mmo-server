# syntax=docker/dockerfile:1

FROM golang:1.24-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY cherry-framework ./cherry-framework
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/master ./cmd/master && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/login ./cmd/login && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/game ./cmd/game && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gm ./cmd/gm && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/import-config ./gameconfig/cmd/import

FROM debian:bookworm-slim AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/master /out/login /out/game /out/gateway /out/gm /out/import-config /app/
COPY configs/mmo-docker.json /app/configs/mmo-docker.json
COPY gameconfig/gen/data /app/gameconfig/gen/data

ENV MMO_PROFILE=/app/configs/mmo-docker.json

EXPOSE 10100 9080
