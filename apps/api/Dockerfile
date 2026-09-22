# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build
#
# One build produces all three binaries. The API and the worker share every
# package, so building them separately would compile the same code twice.
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies are copied first so that editing application code does not
# invalidate the module download layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# CGO is off so the binaries are static and can run on a distroless base.
# Everything the application needs is pure Go — pgx, argon2 and the rest — so
# there is nothing to link against.
#
# The migrations are embedded in the binary, which is why db/ does not have to
# be shipped to the runtime image.
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/migrate ./cmd/migrate

# ---------------------------------------------------------------------------
# Runtime
#
# Alpine rather than scratch: the image needs CA certificates to reach the
# provider over TLS, and a shell makes a production incident debuggable.
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -H -u 10001 xmail

WORKDIR /app

COPY --from=build /out/server  /app/server
COPY --from=build /out/worker  /app/worker
COPY --from=build /out/migrate /app/migrate

# Attachments when STORAGE_DRIVER=local. Owned by the run user, since the
# process is not root and could not create it at startup otherwise.
RUN mkdir -p /app/storage && chown -R xmail /app/storage
VOLUME ["/app/storage"]

# Running as root inside a container that parses untrusted email is a needless
# risk; nothing here requires privilege.
USER xmail

ENV APP_ENV=production \
    HTTP_ADDR=:8090 \
    WORKER_HTTP_ADDR=:8092 \
    STORAGE_LOCAL_PATH=/app/storage

EXPOSE 8090 8092

# The API applies pending migrations at startup, so a fresh container is never
# left running against an older schema.
HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=5 \
    CMD wget --quiet --tries=1 --output-document=/dev/null http://127.0.0.1:8090/readyz || exit 1

CMD ["/app/server"]
