# syntax=docker/dockerfile:1.7

FROM node:20-bullseye AS web-builder

WORKDIR /build/src/webapp

COPY src/webapp/package.json src/webapp/yarn.lock src/webapp/tsconfig.json ./
COPY src/webapp/public ./public
COPY src/webapp/src ./src

RUN corepack enable && \
    yarn install --frozen-lockfile && \
    yarn build

FROM golang:1.25 AS go-builder

ARG VERSION=dev-docker
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum build.go ./
COPY src ./src
COPY --from=web-builder /build/src/webapp/build ./src/webapp/build

RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64) go_arch=amd64 ;; \
        arm64) go_arch=arm64 ;; \
        arm) go_arch=arm ;; \
        386) go_arch=386 ;; \
        *) echo "Unsupported TARGETARCH: ${TARGETARCH}"; exit 1 ;; \
    esac; \
    APP_VERSION="${VERSION}" NO_TELEMETRY=1 PLATFORM=linux ARCH="${go_arch}" go run ./build.go release; \
    cp "bin/bililive-linux-${go_arch}" /tmp/bililive-go

FROM ubuntu:22.04

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ENV IS_DOCKER=true
ENV WORKDIR="/srv/bililive"
ENV OUTPUT_DIR="/srv/bililive" \
    CONF_DIR="/etc/bililive-go" \
    PORT=8080

ENV PUID=0 PGID=0 UMASK=022

RUN mkdir -p $OUTPUT_DIR && \
    mkdir -p $CONF_DIR && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    curl \
    tzdata \
    ca-certificates \
    libatomic1 && \
    sh -c '\
    if [ "$TARGETARCH" = "arm" ]; then \
    echo "skip gosu for arm (armv7/armhf)"; \
    else \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends gosu; \
    fi' && \
    sh -c '\
    if [ "$TARGETARCH" = "amd64" ] || [ "$TARGETARCH" = "arm64" ]; then \
    echo "skip apt ffmpeg for $TARGETARCH"; \
    else \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ffmpeg; \
    fi' && \
    cp -r -f /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /tmp/bililive-go /usr/bin/bililive-go
COPY config.docker.yml $CONF_DIR/config.yml

RUN chmod +x /usr/bin/bililive-go && \
    mkdir -p /opt/bililive/tools

RUN --mount=type=cache,id=bililive-tools-$TARGETOS-$TARGETARCH$TARGETVARIANT,sharing=locked,target=/cache/bililive/tools \
    set -eux; \
    echo "Preparing bililive tools cache for bililive-tools-$TARGETOS-$TARGETARCH$TARGETVARIANT..."; \
    mkdir -p /opt/bililive/tools /cache/bililive/tools; \
    ls -lR /cache/bililive/tools || true; \
    /usr/bin/bililive-go --sync-built-in-tools-to-path /cache/bililive/tools || true; \
    cp -a /cache/bililive/tools/. /opt/bililive/tools/

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME $OUTPUT_DIR

EXPOSE $PORT

WORKDIR ${WORKDIR}
ENTRYPOINT [ "sh" ]
CMD [ "/entrypoint.sh" ]
