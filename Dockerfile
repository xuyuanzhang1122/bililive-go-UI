# syntax=docker/dockerfile:1.7

FROM node:20-bullseye AS web-builder

ARG NPM_REGISTRY_FALLBACK=https://registry.npmmirror.com
ARG YARN_NETWORK_TIMEOUT=600000

WORKDIR /build/src/webapp

COPY src/webapp/package.json src/webapp/yarn.lock src/webapp/tsconfig.json ./
COPY src/webapp/public ./public
COPY src/webapp/src ./src

RUN --mount=type=cache,id=bililive-yarn-cache,target=/usr/local/share/.cache/yarn \
    set -eux; \
    corepack enable; \
    yarn config set network-timeout "${YARN_NETWORK_TIMEOUT}"; \
    yarn install --frozen-lockfile --network-timeout "${YARN_NETWORK_TIMEOUT}" || ( \
        echo "yarn install failed, retrying with fallback registry ${NPM_REGISTRY_FALLBACK}"; \
        yarn config set registry "${NPM_REGISTRY_FALLBACK}"; \
        sed -i "s#https://registry.yarnpkg.com#${NPM_REGISTRY_FALLBACK}#g" yarn.lock; \
        yarn install --frozen-lockfile --network-timeout "${YARN_NETWORK_TIMEOUT}" \
    ); \
    yarn build

FROM golang:1.25 AS go-builder

ARG VERSION=dev-docker

WORKDIR /build

COPY go.mod go.sum build.go ./
COPY src ./src
COPY --from=web-builder /build/src/webapp/build ./src/webapp/build

RUN set -eux; \
    APP_VERSION="${VERSION}" NO_TELEMETRY=1 PLATFORM=linux ARCH=amd64 go run ./build.go release; \
    cp bin/bililive-linux-amd64 /tmp/bililive-go

FROM ubuntu:22.04

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
    libatomic1 \
    gosu && \
    cp -r -f /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /tmp/bililive-go /usr/bin/bililive-go
COPY config.docker.yml $CONF_DIR/config.yml

RUN chmod +x /usr/bin/bililive-go && \
    mkdir -p /opt/bililive/tools

RUN --mount=type=cache,id=bililive-tools-amd64,sharing=locked,target=/cache/bililive/tools \
    set -eux; \
    echo "Preparing bililive tools cache..."; \
    mkdir -p /opt/bililive/tools /cache/bililive/tools; \
    /usr/bin/bililive-go --sync-built-in-tools-to-path /cache/bililive/tools || true; \
    cp -a /cache/bililive/tools/. /opt/bililive/tools/

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME $OUTPUT_DIR
VOLUME /var/lib/bililive

EXPOSE $PORT

WORKDIR ${WORKDIR}
ENTRYPOINT [ "sh" ]
CMD [ "/entrypoint.sh" ]
