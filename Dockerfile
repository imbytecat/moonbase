# Single-binary image: SPA embedded via `moon run server:release`, then the
# static binary dropped into distroless. `docker build .` from the repo root.
FROM debian:trixie-slim AS builder

RUN apt-get update && apt-get install -y --no-install-recommends curl git ca-certificates unzip xz-utils && rm -rf /var/lib/apt/lists/*

# proto installs the exact toolchain from .prototools (go/node/pnpm/moon) — the same
# versions used everywhere else in this repo.
RUN curl -fsSL https://moonrepo.dev/install/proto.sh | bash -s -- --yes
ENV PATH="/root/.proto/shims:/root/.proto/bin:$PATH"

WORKDIR /build
COPY .prototools ./
RUN proto install

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/api-client/package.json packages/api-client/
RUN pnpm install --frozen-lockfile

COPY . .
RUN moon run server:release

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/apps/server/bin/server /server

EXPOSE 8080
ENTRYPOINT ["/server"]
