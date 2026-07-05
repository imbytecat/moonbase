# AGENTS.md

moonrepo monorepo. `proto/` (Protobuf + Buf + ConnectRPC) is the single source of truth: `moon run proto:generate` regenerates BOTH a Go server (`apps/server`) and a TS client (`packages/api-client`) consumed by a React 19 SPA (`apps/web`). A field/RPC mismatch is a **compile error**, not a runtime surprise. Toolchain pinned by **proto** (`.prototools`: go/node/pnpm/moon), tasks by **moon** v2, package manager **pnpm**.

It ships as an admin-system **TEMPLATE, not a framework**: downstream projects copy the repo, diverge, and cherry-pick fixes back via `git remote add template`. Consequences that change how you work: keep channel packages free of business-code imports (portable diffs); do NOT extract shared Go libraries, split microservices, or add semver/backcompat shims — nothing external depends on this code, so settings-struct changes may legitimately zero-read old rows. The driver registries ARE the plugin system (compile-time, `database/sql`-style).

## Commands (from repo root)

- `proto install` — installs go/node/pnpm/moon from `.prototools`; proto activation puts them on PATH (even non-interactive shells), no `export` needed.
- `pnpm install` — JS deps (workspaces `@moonbase/web`, `@moonbase/api-client`).
- `docker compose up -d` — Postgres 18 (matches the default DSN) + optional SeaweedFS (S3 demo) + mailpit (SMTP, inbox :8025). Seed runs ONLY against an empty users table — if integration tests fail "invalid credentials", reset: `docker compose down && docker volume rm moonbase_pgdata && docker compose up -d`.
- `moon run :dev` — web :5173 + server :8080; migrations + seed (`admin`/`admin123`) auto-apply on startup.
- Invocation is `moon run <project>:<task>`; projects: `proto`, `server`, `web`, `api-client`.
- `moon run :check` = repo-wide lint/format gate; `moon run :fix` = auto-fix twin; `moon ci` = build/test/check affected.
- Single Go test: `cd apps/server && go test ./... -run TestName`. Single web test: `cd apps/web && pnpm run test <file>`.
- Go integration tests need `MOONBASE_DATABASE_URL='postgres://postgres:postgres@localhost:5432/app?sslmode=disable'` (skip silently without it); email-flow tests also need mailpit (skip when down).

## Generated code is git-ignored — generate first or builds fail

Fresh clone: these don't exist until generated; never hand-edit them.
- `apps/server/internal/gen/` + `packages/api-client/src/gen/` ← `moon run proto:generate` (buf). Depended on by server:build/dev/test/check/fix/release + web:build/typecheck/dev via a `proto:generate` task dep.
- `apps/server/internal/systemcodec/` ← `moon run proto:generate` (`protoc-gen-settings`, a repo-local buf plugin): channel profile storage structs + write-only-secret `Mask`/`FromProto`/`Merge` codecs, one per `option (moonbase.v1.profile)` message. `settings.*Profile` are aliases into it; the drivers import it directly.
- `apps/server/internal/repository/` ← `moon run server:generate` (sqlc).
- `apps/web/src/routeTree.gen.ts` ← `moon run web:gen` (TanStack Router).
- `apps/web/src/paraglide/` ← `moon run web:gen-i18n` (Paraglide, from `messages/*.json`; the Vite plugin also regenerates on dev/build).
- **air does NOT watch `proto/`** — after editing a `.proto`, run `moon run proto:generate` (moon caches it, ~free when unchanged).
- **sqlc does NOT clean up**: deleting a `db/query/*.sql` leaves a stale `internal/repository/*.sql.go` and the build breaks on dropped types — `rm` the generated twin, then regenerate.

## Guardrail tests — a red test means fix the side you forgot, never weaken the test

- `internal/server/authz_test.go` — every registered RPC needs an authz rule. New proto service ⇒ generated blank import + path prefix here, rules in `authz.go`.
- `internal/rpc/providers_test.go` — every provider in a proto `provider` `in:` list needs a Go driver, and vice versa; `TestPaymentMethodsMatchContract` + `TestPaymentProfileMethodsMatchContract` align the per-order method and the profile's signed-products list with `pay.Methods()` (the union of driver catalogs).
- `internal/rpc/secrets_test.go` — every write-only secret must survive an empty-value update; a missing `keepSecrets` branch = wiped credential.
- `apps/web/src/lib/messages.test.ts` — `zh-CN.json` / `en.json` keys must stay in parity.
- `internal/config/config_test.go` (`TestLoadEnvOverrides`) — every config key needs a viper default or its `MOONBASE_*` env is silently ignored.

## Add a new API domain

1. `proto/<domain>/v1/<domain>.proto` (protovalidate rules inline) → `moon run proto:generate`.
2. Re-export the generated `_pb` + `_connectquery` from `packages/api-client/src/index.ts` — forgetting this breaks web imports.
3. Migrations (`moon run server:migrate-new -- <name> sql`) + `db/query/*.sql` → `moon run server:generate`.
4. Service impl in `internal/rpc/` with a `var _ <x>connect.Handler = (*Svc)(nil)` assertion; register in `internal/server/router.go`.
5. Authz rule for EVERY procedure in `internal/server/authz.go` + the generated blank import AND path prefix in `authz_test.go`.
6. New permissions: add a value to the `Permission` enum (`proto/auth/v1/permission.proto`) AND a matching `auth.Catalog` entry (`internal/auth/permissions.go`) — `TestPermissionEnumMatchesCatalog` fails if they drift.
7. Web: route under `src/routes/_authed/` with `requirePermission` in beforeLoad; leaf in `NAV_TREE` (`src/lib/navigation.tsx`); messages in BOTH `messages/{zh-CN,en}.json`; a `permission_*` message pair in `src/lib/permissions.ts`.

Removing a domain = reverse + a migration mapping stored `role_permissions` to a successor key (keys live in role rows) + `rm` the orphaned sqlc output.

## Add a provider to an existing channel

1. New config message in `proto/system/v1/system.proto` (its OWN field shape, never reuse another driver's) + the value in that `*Profile.provider` `in:` list. Mark each write-only credential field `[(moonbase.v1.secret) = true]` and give it a sibling `bool <field>_set` (the read-side mask flag; the `<field>_set` name is a HARD convention `protoc-gen-settings` matches on). → regenerate.
2. NO hand-written mapper. `moon run proto:generate` runs `protoc-gen-settings` (`apps/server/cmd/protoc-gen-settings`, wired in `buf.gen.yaml`) which, for every message tagged `option (moonbase.v1.profile) = true`, emits into `internal/systemcodec` (git-ignored): the storage struct (the `<field>_set` flags dropped — they're wire-only), its `ProfileID`/`ProviderName`/`WithID`, and a `<Channel>Codec` with `FromProto`/`Mask`/`Merge`. `Mask` blanks secrets + sets `<field>_set`; `Merge` keeps a stored secret on an empty update and keeps `[(moonbase.v1.immutable) = true]` fields (e.g. oauth `key`). `channelOps.keepSecrets` is just `systemcodec.<Channel>Codec.Merge`; handlers call `Codec.FromProto`/`.Mask`.
3. Driver entry in the channel package's `drivers` registry (`channel.Registry`); driver code addresses `systemcodec.<Channel>Profile` (NOT a `settings.*Profile` — those are now generated aliases, and the Go field names follow proto's CamelCase: `AccessKeyId`, `ApiKey`, `OpAppId`, not `AccessKeyID`/`APIKey`/`OpAppID`).
4. Web: a `ProviderOption` card + a config-fields branch keyed on the provider in `src/components/system/<channel>-profile-drawer.tsx` (or `<channel>-panel.tsx` for captcha/llm) + messages. The drawer submits ONLY the picked provider's values; the generated `Merge` backfills the other providers' stored configs so credentials survive. Read the mask flag as `<field>Set` (e.g. `smtp.passwordSet`), matching the proto `<field>_set`.

New PURPOSE instead = one constant + `Purposes` catalog entry in the channel package + a `PURPOSE_LABELS` entry + web messages.

Adding a whole new channel = the above, plus a message tagged `option (moonbase.v1.profile) = true` (the generator finds it automatically), a `settings.<Channel>` type alias + `Store` getter/setter, and a `system_<channel>.go` with the standard `channelOps` wiring.

## Settings = TWO backend surfaces (the web shows them as ONE)

- `settings.v1` = business toggles (`settings.*` perms): registration policy, signup identifiers, phone regions, SITE identity. `GetSiteInfo` is PUBLIC (login page renders it logged-out). New product toggle → here.
- `system.v1` = infra channels with secrets (`system.*` perms): storage/captcha/email/sms/llm/oauth/payment. New channel → here. There is NO generic UpdateSystemSettings — only GetSystemSettings + per-channel profile CRUD/Bind/Test.
- The web presents both under `/settings/*` (permission-filtered groups); the split lives in proto/permissions, not navigation.
- **Secrets are write-only over the wire**: reads mask (`secret_set`); an empty value on update keeps the stored secret (each channel's `keepSecrets`).
- **Settings storage is JSONB with NO migrations** (`internal/settings`): a struct-shape change silently zero-reads old rows — reset the dev volume or re-enter config (expect the same for real deploys). A missing row reads as the zero config, so nothing needs seeding.
- **One unified channel model**: `settings.Channel[P]` (Profiles + Bindings `map[string][]string`); `channelOps[P]` (`internal/rpc/system_channel.go`) is the single lifecycle behind every channel's Create/Update/Delete/Bind. Per-channel files only do proto⇆settings mapping.
- **Binding IS activation** — there is NO per-profile `enabled` flag anywhere (it would create a bound-but-disabled state with undefined semantics, e.g. a silently disabled CAPTCHA). Unbind to pause.
- **Purpose catalogs are CODE**: each channel exports a `channel.Catalog` (`storage.Purposes`, `mail.Purposes`, …). Business code addresses channels by purpose, never a profile id: `mail.Sender.Send(ctx, purpose, …)`. An unbound purpose → `ErrNotConfigured` (except CAPTCHA = pass-through, so a fresh install stays loginable); deleting a bound profile = FailedPrecondition. Multi-valued purposes (oauth `login`, all payment) carry `profile_ids` and fan out; the rest are single-valued.
- **Drivers = per-provider config shapes behind one seam**: a tagged-union profile (one sub-message per provider); ALL provider configs persist side by side so switching never loses credentials. Never flatten provider params into shared fields — only the seam (Send/Verify/Complete/…) is shared.
- OAuth profile `key` = the slug in `user_identities.provider` and the flow URL `/api/oauth/{key}/...`; IMMUTABLE after create; deleting one with identity rows = FailedPrecondition.
- Web: every channel reuses `ProfileManager` + `ProfileFormDrawer`; every form goes through `FormDrawer` (dirty-guards close) — never mount a raw antd Drawer around a form. New channel = new panel in `src/components/system/` + a `src/lib/settings-nav.tsx` entry.
- Keep tool/library names out of user-facing text (proto comments, UI copy, error strings) — describe the protocol ("SMTP", "S3-compatible"), not the impl. Errors show a generic translated message, never raw `err.message`.

## Capability = control plane + optional data plane

Separate WHAT CONNECTION (control plane: the profiles/bindings/registry model above — JSONB, no migrations, no state) from WHAT HAPPENED (data plane — pick the weakest tier, don't invent a generic abstraction): Tier 1 short-lived secrets → `verification_tokens`; Tier 2 append-only ledger → the audit pattern; Tier 3 domain state machine → real migrated tables (`payment_orders`, DBOS checkpoints). Settings JSONB never stores state; state tables never store credentials. Seam signatures are deliberately per-domain — no generic channel interface.

- **Audit** (`internal/audit` + audit.v1): ONE interceptor seam records every mutating unary RPC — handlers never write audit rows. Request payloads are NEVER stored (secrets stay write-only even to the trail); read-only RPC surface; hourly retention janitor via `MOONBASE_AUDIT_RETENTION_DAYS` (default 180, 0 = forever).
- **Workflows** (`internal/workflow` + workflow.v1): DBOS is a LIBRARY that checkpoints into the `dbos` schema of the SAME Postgres and resumes interrupted runs on startup. Workflows are registered CODE; a nil engine (unit tests) makes workflow RPCs answer FailedPrecondition.
- **Payments** are the one channel with a data plane: the migrated `payment_orders` table owns the state machine; every settlement write is SQL status-guarded (`WHERE status IN (...)`) so replayed provider callbacks and concurrent syncs are idempotent. Callbacks are plain `POST /api/payment/notify/{provider}/{profile}` authenticated by driver signature verification (no session). **Methods are provider-scoped official product ids** (Alipay API method `precreate`/`page_pay`/`wap_pay`/`create`/`app_pay`; WeChat trade_type `native`/`h5`/`jsapi`/`app`) — NOT a shared triple: each driver declares a `pay.Method` catalog (id + `CredentialKind` qr/redirect/params + required `Inputs`), a profile signs for a subset (`PaymentProfile.Methods`, empty = all → `pay.Offered`), the checkout offers only those, and the frontend mirrors the catalog in `src/lib/payments.ts` while rendering by `order.credentialKind`. Alipay `create` (小程序 JSAPI) needs `op_app_id` or the order fails.
- **Notifications + outbound i18n** (`internal/notification` + notification.v1; `internal/i18n`): the per-user 站内信 inbox. Business code notifies via the `notification.Publisher` seam — `Publish(userID,…)` / `PublishToPermission(perm,…)` (fan-out to holders of a permission) — NEVER by writing `notifications` rows; the read side is self-scoped RPCs (authz `{}` + `IdentityFromContext`, so a user only sees their own). Outbound text (inbox title/body, verify/reset/code emails) is localized through `internal/i18n` (`Resolve`: `user.locale` → request `Accept-Language` → default `zh-CN`) and stored/sent already-rendered PER RECIPIENT — but RPC error messages are NOT localized server-side (they stay codes the SPA humanizes). `users.locale` (`CurrentUser.locale`) is the account language; the SPA applies it on login via `setLocale` (reload-convergent), and public auth pages carry an anonymous switcher.

## Auth & RBAC

- **Two edge layers, zero auth in business code** (`internal/auth`): authn (`NewMiddleware`, on `connectrpc.com/authn`) resolves the `session` cookie → `*auth.Identity`; it NEVER rejects (anonymous proceeds with nil). authz (`NewAuthzInterceptor` + the rule table in `internal/server/authz.go`): every RPC → `{Public} | {} (any session) | {Permission}`. **Unknown procedures are denied by default.**
- Handlers read `auth.IdentityFromContext(ctx)`; unit tests inject `auth.WithIdentity(ctx, &Identity{…})` — no HTTP, login, or mocks.
- **Permission catalog: the proto `Permission` enum is the source of truth** (`proto/auth/v1/permission.proto`), shared type-safely by Go, web and (future) mobile — a permission typo is a compile error on every end. It's kept 1:1 with the Go `auth.Catalog` (key + description) by `TestPermissionEnumMatchesCatalog`. The DB (`role_permissions.permission`), `Identity`, and the `authz.go` rule table stay on the dotted string keys (`user.read`); the enum maps to them at the wire boundary only (`internal/rpc/permissions.go`, `permissionKey`/`permissionEnum`, mirrored on the frontend in `src/lib/permissions.ts`). ADD enum values + catalog entries, never RENAME (stored `role_permissions` break). `PERMISSION_ALL` is the `admin` wildcard `*`, IMMUTABLE (`isAdminRole` guard); system roles `admin`/`user` can't be renamed or deleted.
- **Sessions are DB-backed** (opaque 32-byte token, only its SHA-256 stored) on purpose: single binary + Postgres gives instant revocation — don't switch to JWT. Browsers use the httpOnly cookie (`session`, or `__Host-session` under `secure_cookie`); native apps send `Authorization: Bearer`. Password hashing is argon2id. Set `MOONBASE_AUTH_SECURE_COOKIE=true` behind TLS.
- Login routes by identifier shape: `@`→email, `+`/digits→phone (E.164), else username (`^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`); the shapes are disjoint — keep the routing exact. Password login has no enable/disable switch (could lock everyone out) and is timing-equalized against `auth.DummyHash` (keep it). Brute-force defense = CAPTCHA, not rate-limiting.
- **Third-party login** (`internal/oauth`; browser HTTP not RPC: `/api/oauth/{key}/authorize|callback`): the `oidc` driver runs on `coreos/go-oidc/v3` + `golang.org/x/oauth2` (discovery, JWKS ID-token verification, nonce, PKCE) — don't re-hand-roll the token exchange; the `wechat` driver is a hand-rolled 3-call QR-login ON PURPOSE (no ID token / no signature exists to verify, so an SDK buys nothing — silenceper/PowerWeChat evaluated and rejected). The `Flow` seam mints `FlowSecrets{Nonce,Verifier}` at authorize time and `auth_oauth_http.go` round-trips them through the httpOnly `oauth_state` cookie (base64 JSON of `state`+nonce+verifier): `state` guards CSRF (compared on callback), nonce/verifier stay client-side for OIDC verification + PKCE.
- Short-lived secrets (email verify, password reset, phone/email bind, sms login, register codes) ALL live in ONE `verification_tokens` table keyed by a `Purpose` constant — add a Purpose, not a new table. Channel-backed identifiers (email/phone) are always code-verified pre-account; public request RPCs answer ok/already_exists per the enumeration policy; ResetPassword revokes ALL sessions.
- **Seed** (`auth.Seed`, after migrations): idempotent roles `admin`(`*`) / `user`(report.read); the initial admin is created ONLY when the users table is empty. The admin has NO email by design (binds one, code-verified, via the profile page).
- **Uploads never proxy bytes through RPC**: `storage.v1` presign returns a signed PUT URL + a server-chosen key; the browser PUTs directly (S3 presigned, or the local driver's `/api/files/...` HMAC endpoint), then saves the key via the owning domain RPC.
- Integration tests (`internal/rpc/integration_test.go`) build the real router and skip without `MOONBASE_DATABASE_URL`.

## API contract: proto + Buf + ConnectRPC

- **Validation is protovalidate** (inline `(buf.validate.field)…`, message-level CEL), enforced by a server interceptor — don't add go-playground/validator or duplicate rules in Go.
- **Managed-mode override (do not remove)**: `buf.gen.yaml` maps module `buf.build/bufbuild/protovalidate` → `buf.build/gen/go/…/protocolbuffers/go`; without it the generated Go imports a non-existent local `…/internal/gen/buf/validate` and the build breaks. `go mod tidy` resolves the real path.
- Plugins run pinned, no install: Go via `local: ['go','run','…@ver']`, TS via `local: ['pnpm','exec',…]`. **The TS plugin devDeps (`@bufbuild/protoc-gen-es`, `@connectrpc/protoc-gen-connect-query`) live in the ROOT `package.json`** so `pnpm exec` (invoked by buf from the repo root) resolves them from root `node_modules/.bin`. Generator↔runtime versions are catalog-pinned in lock-step in `pnpm-workspace.yaml` (Buf requires plugin and runtime to match) — bump both catalog entries together, never one.
- Handlers live on a std `http.ServeMux` at canonical `/pkg.v1.Service/…`, mounted under `/api` via `StripPrefix`. Connect protocol over HTTP/1.1 + JSON (unary is curl-able; no h2c needed).

## Database & migrations

- Postgres 18+ (`uuidv7()` defaults). No `.env`, no config file — defaults + `MOONBASE_*` env only (`internal/config`, viper). **Every config key MUST have a `v.SetDefault`** or its env var is silently ignored (AutomaticEnv+Unmarshal only sees known keys; guarded by `TestLoadEnvOverrides` — extend it when adding keys).
- **Migrations auto-apply on server startup** (goose SQL in `db/migrations/`, embedded, advisory-locked so replicas can't race). The `migrate-*` moon tasks are manual ops only.
- sqlc parses only the Up sections of the migrations as its schema; queries in `db/query/*.sql`; partial updates use `sqlc.narg('x')` + `coalesce` paired with proto3 `optional`.

## moon v2 config gotchas

- Project `moon.yml` uses `layer:` + `stack:` (NOT `type:`/`platform:`/`toolchain:` — they error). Workspace uses `vcs.client` (NOT `vcs.manager`).
- v2 renamed the toolchain file to plural `.moon/toolchains.yml`, but its `$schema` stays SINGULAR `…/toolchain.json`. It manages no language toolchains (proto owns versions); tasks run through moon's `system` toolchain.
- The website JSON schemas lag the 2.3.5 binary — trust moon's actual parse errors, not the site.
- Shared tasks live once in `.moon/tasks/{go,typescript}.yml` (inherited by `language:` match); a same-named project task MERGES (appends deps), e.g. server's `test`/`check` add `deps: ['proto:generate','~:generate']`. `proto` sets no `language` and defines its own tasks.

## Lint & git hooks

- Every project's read-only gate is `check` with writer twin `fix`. Backend = golangci-lint v2 (`.golangci.yml`) via `go run`; frontend = Biome (`biome.json`: 2-space, single quotes, no semicolons, width 100; formatter + linter + import-sorter; auto-skips generated files). Proto = buf lint + format.
- **Pre-commit hook** (auto-synced; install once with `moon sync hooks`): `moon run :fix` → `git update-index --again` (re-stage fixes) → `moon run :check`, all `--affected --status=staged`. Only unfixable errors block.
- **Commit-msg hook**: `pnpm exec commitlint` enforces Conventional Commits with a **Chinese subject** (`subject-zh` custom rule) + a `scope-enum` (project names + `deps`/`ci`/`agents`), read dynamically from `.moon/workspace.yml` `projects` in `commitlint.config.mjs`.
- golangci-lint reporting wrong file paths after a repo move = stale cache: `go run …/golangci-lint/v2/cmd/golangci-lint@<v> cache clean`.

## Frontend (apps/web)

- **Path alias `#*` → `./src/*`** via package.json `imports` (Node subpath imports, NOT tsconfig `paths`). The `["./src/*","./src/*.ts","./src/*.tsx"]` fallback array is REQUIRED (TS resolves `imports` literally, no extension guessing). `#messages/*` → `./messages/*`. Use `#lib/…`, `#components/…`; never `../`.
- **Data layer is ConnectRPC**, not hand-written fetch: transport in `src/lib/transport.ts` (`baseUrl: '/api'`); import generated method refs from `@moonbase/api-client`, call `useSuspenseQuery`/`useMutation` from `@connectrpc/connect-query`. Proto `Timestamp` is a message — render via `timestampDate()` from `@bufbuild/protobuf/wkt`, not `Date`/string.
- `@connectrpc/connect` must be a DIRECT dep of `apps/web` (transitive-only breaks `tsc`). Route search-param interfaces must be `export`ed (unexported → TS4023 in `routeTree.gen.ts`).
- `vite.config.ts`: `tanstackRouter()` MUST come before `react()`. Dev server proxies `/api` → `:8080`.
- **tsconfig**: `allowJs: true` is REQUIRED (Paraglide emits JSDoc-typed `.js`; removing it degrades all `m.*()` to `any`). `noUncheckedIndexedAccess` + `verbatimModuleSyntax` are on.
- **i18n = Paraglide** (compiler-based): catalogs `messages/{zh-CN,en}.json` (flat keys, `{param}`). All strings are message-function refs: `m.nav_dashboard()`; unknown key or typo'd param = compile error. `label`s in nav are message REFERENCES (`m.nav_users`, not called). The `--strategy` list lives in BOTH `vite.config.ts` and the `gen:i18n` script — keep them identical. Map backend errors with `humanizeError` (`src/lib/errors.ts`), never raw `err.message`.
- Session state = the GetMe query (`src/lib/session.ts`), no store/context; login/logout call `queryClient.clear()`. Auth-flow UI is capability-driven off `GetAuthConfig` (public). Navigation is the declarative `NAV_TREE` (`src/lib/navigation.tsx`); settings has its own catalog (`src/lib/settings-nav.tsx`).
- **antd v6 + Tailwind v4** coexist via the `@layer` order in `src/styles.css` + `<StyleProvider layer>` — changing either breaks antd styling. Theme tokens live once in `AntThemeBridge`; dark mode via `ThemeModeProvider` drives BOTH Tailwind `.dark` and antd `darkAlgorithm`. Plain antd Table/Drawer/Form (no ProComponents). antd v6 API renames to honor: `Drawer width`→`size`, `Alert message`→`title`, `Dropdown dropdownRender`→`popupRender`, `List`→semantic `<ul>/<li>`.
- Charts are `@ant-design/plots`; theme follows `useThemeMode()`. Backend time series are SPARSE (zero days omitted) — fill gaps client-side (`fillDaily`). Phone inputs: `<PhoneInput allowedRegions>` + `phoneRule()`; value is E.164 end-to-end.

## SPA embedding: dev proxy vs prod single binary

- Dev: Vite proxies `/api` → :8080; SPA NOT embedded (`//go:build !embed` stub in `internal/web/stub.go`).
- Prod: `pnpm run release` = `moon run server:release` → `-tags embed`. `web:build` is a TASK dep (not a project `dependsOn` — moon's `enforceLayerRelationships` rejects app→app edges) so the SPA builds first, into `apps/server/internal/web/dist/`, embedded via `//go:build embed`. One binary, same-origin, no CORS.

## Backend layout (apps/server, single Go module)

- Module `github.com/imbytecat/moonbase/server`, entrypoint `cmd/server`; cross-language type alignment is proto's job. ConnectRPC owns its paths (no router framework).
- Route the ONE `internal/logging` slog logger into every library (pgx/goose/DBOS) — never hand a library its own logger. Handlers return typed `connect.NewError`; internal failures log via slog + return generic `CodeInternal` (no leak); `pgx.ErrNoRows` → `CodeNotFound`.
- **Observability = logs + metrics + traces, one seam each.** Metrics (`internal/metrics`, Prometheus): a Connect interceptor placed OUTERMOST (so it also counts authz rejections) records `moonbase_rpc_*` (procedure/code labels — bounded), plus pgxpool/Go-runtime/`build_info` collectors, served at `/metrics` on the OUTER mux (outside `/api` authn — a scraper has no session; restrict at the network layer). Tracing (`internal/tracing`, OpenTelemetry) is DORMANT by default: no `otel.trace_endpoint` → no provider, and the otelconnect interceptor + `NewSlogHandler` (injects `trace_id` when a span is live) are cheap no-ops. Both wired only when their config toggles are on. Build/version (`internal/buildinfo`) comes from `runtime/debug` (ldflags `-X …/buildinfo.version` overrides for releases), surfaced in `/health` and the startup log.
- `CGO_ENABLED=0` is set in `apps/server/moon.yml` `env:` — keep the server pure-Go (goose uses modernc sqlite). sqlc/goose/buf/air/golangci-lint all run via pinned `go run …@<ver>` — bump versions in `moon.yml`, not a package manager.
- **Testing**: services depend on the sqlc `repository.Querier` interface → unit tests use a struct-embedding fake (no DB, no mock framework). Integration tests build the real stack and skip without `MOONBASE_DATABASE_URL`.
- Hot reload `server:dev` = air (`.air.toml`), watches `.go`+`.sql`, `pre_cmd` regenerates sqlc. Run via `moon run server:dev` so CGO/env are inherited. Keep the `.air.toml` `[build.windows]` `.exe` block — air only auto-adds `.exe` to its default cmd, so removing it silently breaks Windows.

## CI, Docker, conventions

- `.gitea/workflows/ci.yml`: `moon ci` on push to `main` + PRs (`moonrepo/setup-toolchain` + `pnpm install`); checkout `fetch-depth: 0` for affected diffing. PRs also run `moon run proto:breaking` vs `origin/main`. A dedicated `moon run server:vuln` step runs `govulncheck` (reachable-vuln scan; kept out of the `moon ci` graph — network-dependent, `runInCI:false`). A `postgres:18-alpine` service backs integration tests (`MOONBASE_DATABASE_URL` host = `postgres`, the service id).
- **Dependency updates** — local one-shot, no CI needed: `proto outdated --update --latest` (toolchain go/node/pnpm/moon → `.prototools`), `pnpm update -Lr` (all workspace JS), and in `apps/server` `go get -u ./... && go mod tidy` (Go modules); then `moon run :check && moon run :test`. The `go run <module>@<ver>` tool pins (in every `moon.yml`, `.moon/tasks/*.yml`, `buf.gen.yaml`) have no standard local updater — edit the string when one drifts (rare); `renovate.json` (custom go manager, datasource=go, needs the `'module@vX.Y.Z'` single-quoted shape) can automate those + everything else if a runner is ever added. `go run @version` stays (buf runs plugins from the repo root, not the app module, so go.mod `tool` directives don't fit). Reachable CVEs (optional): `moon run server:vuln` (govulncheck).
- `Dockerfile`: proto → `.prototools` toolchain → `moon run server:release` → distroless static (CGO off). `compose.yaml` PG18 mounts the volume at `/var/lib/postgresql` (NOT `…/data` — PG18 moved the data dir).
- Commits: Conventional Commits, **Chinese subject**, scope = a project name or `deps`/`ci`/`agents` (enforced by the hook). Remote is Gitea, default branch `main`.
- `.gitignore` is hybrid: root holds only workspace-global rules; each project owns its build/generated ignores. `apps/web/src/paraglide/` self-ignores (Paraglide emits its own). Add new ignore rules to the owning project. No inline `#` comments (must start their own line).
- **README vs AGENTS**: README is the visitor pitch (~90 lines); design rationale, invariants, gotchas, and checklists go HERE. Never duplicate a fact in both files.
