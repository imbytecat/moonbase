// Package server builds the HTTP handler chain and wires services to the
// repository. Routing is std net/http (Go 1.22+ method patterns) — ConnectRPC
// mounts its own canonical paths, so a router framework adds nothing here.
package server

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	connectcors "connectrpc.com/cors"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"

	"github.com/imbytecat/moonbase/server/internal/audit"
	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/captcha"
	"github.com/imbytecat/moonbase/server/internal/config"
	"github.com/imbytecat/moonbase/server/internal/gen/audit/v1/auditv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/notification/v1/notificationv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/payment/v1/paymentv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/report/v1/reportv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/role/v1/rolev1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/settings/v1/settingsv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/system/v1/systemv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/user/v1/userv1connect"
	"github.com/imbytecat/moonbase/server/internal/gen/workflow/v1/workflowv1connect"
	"github.com/imbytecat/moonbase/server/internal/handler"
	"github.com/imbytecat/moonbase/server/internal/llm"
	mail "github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/metrics"
	"github.com/imbytecat/moonbase/server/internal/notification"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/rpc"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
	"github.com/imbytecat/moonbase/server/internal/storage"
	"github.com/imbytecat/moonbase/server/internal/verify"
	"github.com/imbytecat/moonbase/server/internal/web"
	"github.com/imbytecat/moonbase/server/internal/workflow"
)

// NewRouter builds the full HTTP handler chain. engine may be nil (tests
// without a workflow executor); the workflow RPCs then answer
// FailedPrecondition.
func NewRouter(cfg *config.Config, pool *pgxpool.Pool, engine *workflow.Engine, logger *slog.Logger) http.Handler {
	repo := repository.New(pool)
	settingsStore := settings.NewStore(repo)
	s3 := storage.NewClient(settingsStore)
	mailer := mail.NewClient(settingsStore.Email)
	smser := sms.NewClient(settingsStore.Sms)
	chatter := llm.NewClient(settingsStore.Llm)
	captchaVerifier := captcha.NewClient(settingsStore)
	oauthFlow := oauth.NewClient(settingsStore.Oauth)
	notifier := notification.NewProducer(repo)
	payGateway := pay.NewClient(settingsStore, cfg.Server.PublicURL)

	healthH := handler.NewHealthHandler(pool)
	policy := auth.SessionPolicy{
		TTL:         cfg.Auth.SessionTTL(),
		MaxLifetime: cfg.Auth.SessionMaxLifetime(),
	}
	reportSvc := rpc.NewReportService(repo, engine, logger)
	authSvc := rpc.NewAuthService(rpc.AuthServiceDeps{
		Repo:         repo,
		Settings:     settingsStore,
		Captcha:      captchaVerifier,
		Mailer:       mailer,
		Smser:        smser,
		Oauth:        oauthFlow,
		Verifier:     verify.NewService(repo),
		Logger:       logger,
		Policy:       policy,
		SecureCookie: cfg.Auth.SecureCookie,
		PublicURL:    cfg.Server.PublicURL,
	})
	userSvc := rpc.NewUserService(repo, notifier, logger)
	roleSvc := rpc.NewRoleService(repo, logger)
	settingsSvc := rpc.NewSettingsService(settingsStore, repo, logger)
	systemSvc := rpc.NewSystemService(settingsStore, repo, s3, mailer, smser, chatter, logger)
	storageSvc := rpc.NewStorageService(repo, s3, logger)
	workflowSvc := rpc.NewWorkflowService(engine, logger)
	auditSvc := rpc.NewAuditService(repo, logger)
	checkoutManager := pay.NewCheckoutIssuer(repo, settingsStore, cfg.Server.PublicURL)
	paymentSvc := rpc.NewPaymentService(repo, payGateway, checkoutManager, logger)
	paymentCheckoutSvc := rpc.NewPaymentCheckoutService(repo, payGateway, checkoutManager, logger)
	notificationSvc := rpc.NewNotificationService(repo, logger)

	// Interceptor order: metrics and tracing are outermost so they observe the
	// full RPC — including authz rejections — then authz decides per-procedure
	// (using the identity the authn middleware put in the context), then
	// protovalidate checks inputs, then the audit interceptor records the
	// mutation's outcome. Metrics/tracing are wired only when enabled.
	chain := make([]connect.Interceptor, 0, 5)
	var appMetrics *metrics.Metrics
	if cfg.Metrics.Enabled {
		var statter metrics.PoolStatter
		if pool != nil {
			statter = poolStatAdapter{pool: pool}
		}
		appMetrics = metrics.New(statter)
		chain = append(chain, appMetrics.Interceptor())
	}
	if cfg.Otel.Enabled() {
		if ic, err := otelconnect.NewInterceptor(); err != nil {
			logger.Error("otel interceptor init failed; tracing disabled for RPCs", "error", err)
		} else {
			chain = append(chain, ic)
		}
	}
	chain = append(chain,
		auth.NewAuthzInterceptor(authzRules),
		validate.NewInterceptor(),
		audit.NewInterceptor(repo, logger),
	)
	interceptors := connect.WithInterceptors(chain...)
	api := http.NewServeMux()
	api.HandleFunc("GET /health", healthH.Health)
	// OAuth browser flow is plain HTTP (redirects), not RPC; it sits inside
	// the authn-wrapped mux so the callback can see a live session (bind).
	api.HandleFunc("GET /oauth/{provider}/authorize", authSvc.OauthAuthorize)
	api.HandleFunc("GET /oauth/{provider}/callback", authSvc.OauthCallback)
	// Local-storage signed URLs (issued by the "local" storage driver); the
	// HMAC signature is the authorization, no session required.
	api.Handle("/files/{purpose}/{key...}", storage.NewHandler(settingsStore, logger))
	// Built-in ALTCHA captcha challenges are public plain HTTP (the widget
	// fetches them before any session exists); solutions verify via the same
	// captcha token fields as every other provider.
	api.HandleFunc("GET /captcha/altcha/challenge", captchaVerifier.ServeAltchaChallenge)
	// Payment async notifications are plain HTTP from the provider's servers;
	// the driver's signature verification is the authentication.
	api.HandleFunc("POST /payment/notify/{provider}/{profile}", paymentSvc.PaymentNotify)
	api.HandleFunc("GET /payment/hosted-flow/{session}", paymentSvc.HostedFlow)
	api.Handle(reportv1connect.NewReportServiceHandler(reportSvc, interceptors))
	api.Handle(authv1connect.NewAuthServiceHandler(authSvc, interceptors))
	api.Handle(userv1connect.NewUserServiceHandler(userSvc, interceptors))
	api.Handle(rolev1connect.NewRoleServiceHandler(roleSvc, interceptors))
	api.Handle(settingsv1connect.NewSettingsServiceHandler(settingsSvc, interceptors))
	api.Handle(systemv1connect.NewSystemServiceHandler(systemSvc, interceptors))
	api.Handle(storagev1connect.NewStorageServiceHandler(storageSvc, interceptors))
	api.Handle(workflowv1connect.NewWorkflowServiceHandler(workflowSvc, interceptors))
	api.Handle(auditv1connect.NewAuditServiceHandler(auditSvc, interceptors))
	api.Handle(paymentv1connect.NewPaymentServiceHandler(paymentSvc, interceptors))
	api.Handle(paymentv1connect.NewPaymentCheckoutServiceHandler(paymentCheckoutSvc, interceptors))
	api.Handle(notificationv1connect.NewNotificationServiceHandler(notificationSvc, interceptors))

	// authn middleware wraps the API mux: it resolves the session cookie to an
	// Identity for every /api request (anonymous when absent) — the HTTP layer
	// is the only place with cookie access.
	authn := auth.NewMiddleware(repo, logger, policy)
	authed := authn.Wrap(api)

	// Mount the API mux under /api with the prefix stripped, so the Connect
	// handler sees its canonical path. This keeps the Vite dev proxy (/api ->
	// :8080) and the same-origin prod embed working with one wiring.
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", authed))

	// Permanent file URLs (ADR-0004). Public purposes must be reachable with
	// no session (the login page loads the logo through here); the authn wrap
	// only resolves an identity — never rejects — so private purposes can
	// authorize per-request.
	fileH := storage.NewFileHandler(settingsStore, s3, repo, logger)
	mux.Handle("GET /f/{file_id}", authn.Wrap(fileH))
	// /metrics sits on the outer mux, outside the /api authn chain, so a
	// Prometheus scraper (which carries no session) can reach it. Restrict
	// access at the network layer.
	if appMetrics != nil {
		mux.Handle("GET /metrics", appMetrics.Handler())
	}

	if web.Enabled {
		mux.Handle("/", web.Handler())
	}

	// CORS matters only for cross-origin callers; dev proxy and prod embed are
	// same-origin. The header/method lists come from connectrpc.com/cors so the
	// Connect protocol's headers are always allowed.
	c := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   connectcors.AllowedMethods(),
		AllowedHeaders:   connectcors.AllowedHeaders(),
		ExposedHeaders:   connectcors.ExposedHeaders(),
		AllowCredentials: true,
		MaxAge:           7200,
	})

	// Outermost first: access log sees the final status (incl. recovered 500s).
	return accessLog(logger, recoverer(logger, securityHeaders(cfg.Auth.SecureCookie, c.Handler(mux))))
}

// poolStatAdapter bridges *pgxpool.Pool to metrics.PoolStatter (which stays pgx-free).
type poolStatAdapter struct{ pool *pgxpool.Pool }

func (a poolStatAdapter) Stat() metrics.PoolStat {
	s := a.pool.Stat()
	return metrics.PoolStat{
		Acquired: s.AcquiredConns(),
		Idle:     s.IdleConns(),
		Total:    s.TotalConns(),
		Max:      s.MaxConns(),
	}
}
