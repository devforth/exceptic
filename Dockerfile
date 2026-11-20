# syntax=docker/dockerfile:1

# The single-page app: static files, the same for every platform.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/patches patches
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build


FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /web/dist/spa web/dist/spa
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/exceptic ./cmd/exceptic

# Data dir owned by the unprivileged runtime user (named volumes inherit this ownership).
# /tmp: scratch has none; it is there for temporary files of the runtime and libraries.
RUN mkdir -p /out/data /out/tmp && chown 65534:65534 /out/data && chmod 1777 /out/tmp


FROM scratch

# CA certificates for outgoing TLS (SMTP, Telegram, Slack).
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/exceptic /exceptic
COPY --from=build --chown=65534:65534 /out/data /data
COPY --from=build /out/tmp /tmp

USER 65534:65534
ENV PORT=8000 \
    DATA_DIR=/data
VOLUME /data
EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/exceptic", "healthcheck"]

ENTRYPOINT ["/exceptic"]
