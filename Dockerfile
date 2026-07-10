# Single-binary image: SPA embedded via `moon run server:release`, then the
# static binary dropped into distroless. `docker build .` from the repo root.
FROM debian:trixie-slim AS builder

ARG MISE_VERSION=2026.7.0

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update && apt-get install -y --no-install-recommends curl git ca-certificates libatomic1 unzip xz-utils && rm -rf /var/lib/apt/lists/*

ENV MISE_DATA_DIR="/mise" \
    MISE_CONFIG_DIR="/mise" \
    MISE_CACHE_DIR="/mise/cache" \
    MISE_INSTALL_PATH="/usr/local/bin/mise" \
    MISE_VERSION="${MISE_VERSION}" \
    PATH="/mise/shims:${PATH}"

RUN curl -fsSL https://mise.run | sh

WORKDIR /build
COPY .mise.toml mise.lock ./
RUN mise trust && mise install --locked

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/api-client/package.json packages/api-client/
RUN mise exec -- pnpm install --frozen-lockfile

COPY . .
RUN mise exec -- moon run server:release

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/apps/server/bin/server /server

EXPOSE 8080
ENTRYPOINT ["/server"]
