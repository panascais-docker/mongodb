# syntax=docker/dockerfile:1
ARG MONGODB_IMAGE=mongodb/mongodb-community-server:9.0-ubi10-slim

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS entrypoint

ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY entrypoint ./entrypoint
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/mongodb-entrypoint ./entrypoint

FROM ${MONGODB_IMAGE} AS upstream

FROM --platform=$BUILDPLATFORM alpine:3.24 AS strip

ARG TARGETARCH

RUN target=$(case $TARGETARCH in amd64) echo x86_64 ;; arm64) echo aarch64 ;; esac) && \
    if [ "$target" = "$(apk --print-arch)" ]; then \
        apk add --no-cache binutils; \
    else \
        apk add --no-cache binutils-$target && ln -s /usr/bin/$target-alpine-linux-musl-strip /usr/local/bin/strip; \
    fi

FROM strip AS mongod

COPY --from=upstream /usr/bin/mongod /usr/bin/mongod
RUN strip /usr/bin/mongod

FROM strip AS mongos

COPY --from=upstream /usr/bin/mongos /usr/bin/mongos
RUN strip /usr/bin/mongos

# upstream's ubi-micro filesystem without mongos, mongosh and its entrypoint
FROM scratch AS base

COPY --from=upstream \
    --exclude=usr/bin/mongod \
    --exclude=usr/bin/mongos \
    --exclude=usr/bin/mongosh \
    --exclude=usr/local/bin/docker-entrypoint \
    / /
COPY --from=mongod /usr/bin/mongod /usr/bin/mongod
COPY --from=entrypoint /out/mongodb-entrypoint /usr/local/bin/mongodb-entrypoint

ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    HOME=/data/db

USER mongod
EXPOSE 27017
VOLUME ["/data/db", "/data/configdb"]
HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --start-interval=1s \
    CMD ["/usr/local/bin/mongodb-entrypoint", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/mongodb-entrypoint"]
CMD ["mongod"]

ARG BUILD_DATE
ARG MONGODB_VERSION
ARG VCS_REF

LABEL org.label-schema.vcs-url="https://github.com/panascais-docker/mongodb.git" \
    org.label-schema.build-date=$BUILD_DATE \
    org.label-schema.vcs-ref=$VCS_REF \
    org.label-schema.name="MongoDB Image" \
    org.label-schema.description="Panascais MongoDB Image" \
    org.label-schema.vendor="Panascais ehf." \
    org.label-schema.schema-version="1.0.0" \
    org.opencontainers.image.created=$BUILD_DATE \
    org.opencontainers.image.revision=$VCS_REF \
    org.opencontainers.image.version=$MONGODB_VERSION \
    org.opencontainers.image.title="MongoDB Image" \
    org.opencontainers.image.description="Panascais MongoDB Image" \
    org.opencontainers.image.url=https://www.mongodb.com \
    org.opencontainers.image.documentation="https://github.com/panascais-docker/mongodb" \
    org.opencontainers.image.vendor="Panascais ehf." \
    org.opencontainers.image.licenses=SSPL-1.0 \
    org.opencontainers.image.source="https://github.com/panascais-docker/mongodb" \
    maintainer="Panascais Open Source <oss@panascais.net>"

FROM base AS replica

CMD ["replica"]

FROM base AS cluster

COPY --from=mongos /usr/bin/mongos /usr/bin/mongos

CMD ["cluster"]

# last, so a plain build is the standalone image
FROM base AS standalone
