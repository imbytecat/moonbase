package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	captchaint "github.com/imbytecat/moonbase/integrations/captcha"
	emailint "github.com/imbytecat/moonbase/integrations/email"
	llmint "github.com/imbytecat/moonbase/integrations/llm"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
	paymentint "github.com/imbytecat/moonbase/integrations/payment"
	smsint "github.com/imbytecat/moonbase/integrations/sms"
	storageint "github.com/imbytecat/moonbase/integrations/storage"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/system/v1/systemv1connect"
	"github.com/imbytecat/moonbase/server/internal/llm"
	mail "github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// SystemService manages infrastructure integrations (system.*). Every
// integration is profile-based and shares the integrationOps lifecycle
// (system_integration.go) with per-integration proto mapping in
// system_<integration>.go; every integration has purpose bindings (oauth login
// and payment purposes are multi-valued, the rest single-valued). Secrets are
// write-only over the wire; testable integrations have a test RPC so operators
// can validate config before relying on it.
type SystemService struct {
	settings        *settings.Store
	repo            repository.Querier
	storageTester   storage.ConnectionTester
	storageRegistry storageint.Registry
	captchaRegistry captchaint.Registry
	llmRegistry     llmint.Registry
	mailer          mail.ProfileSender
	emailRegistry   emailint.Registry
	oauthRegistry   oauthint.Registry
	paymentRegistry paymentint.Registry
	smsRegistry     smsint.Registry
	smser           sms.ProfileSender
	chatter         llm.Chatter
	logger          *slog.Logger
}

func NewSystemService(
	store *settings.Store,
	repo repository.Querier,
	storageTester storage.ConnectionTester,
	storageRegistry storageint.Registry,
	captchaRegistry captchaint.Registry,
	llmRegistry llmint.Registry,
	emailRegistry emailint.Registry,
	oauthRegistry oauthint.Registry,
	paymentRegistry paymentint.Registry,
	smsRegistry smsint.Registry,
	mailer mail.ProfileSender,
	smser sms.ProfileSender,
	chatter llm.Chatter,
	logger *slog.Logger,
) *SystemService {
	return &SystemService{
		settings:        store,
		repo:            repo,
		storageTester:   storageTester,
		storageRegistry: storageRegistry,
		captchaRegistry: captchaRegistry,
		llmRegistry:     llmRegistry,
		emailRegistry:   emailRegistry,
		oauthRegistry:   oauthRegistry,
		paymentRegistry: paymentRegistry,
		smsRegistry:     smsRegistry,
		mailer:          mailer,
		smser:           smser,
		chatter:         chatter,
		logger:          logger,
	}
}

var _ systemv1connect.SystemServiceHandler = (*SystemService)(nil)

func (s *SystemService) GetSystemSettings(
	ctx context.Context,
	_ *connect.Request[systemv1.GetSystemSettingsRequest],
) (*connect.Response[systemv1.GetSystemSettingsResponse], error) {
	out, err := s.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

func (s *SystemService) snapshot(ctx context.Context) (*systemv1.GetSystemSettingsResponse, error) {
	st, err := s.settings.Storage(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load storage settings", err)
	}
	captchaCfg, err := s.settings.Captcha(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load captcha settings", err)
	}
	emailCfg, err := s.settings.Email(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load email settings", err)
	}
	smsCfg, err := s.settings.Sms(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load sms settings", err)
	}
	llmCfg, err := s.settings.Llm(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load llm settings", err)
	}
	oauthCfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	paymentCfg, err := s.settings.Payment(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load payment settings", err)
	}
	return &systemv1.GetSystemSettingsResponse{
		Storage: s.toProtoStorage(st),
		Captcha: s.toProtoCaptcha(captchaCfg),
		Email:   s.toProtoEmail(emailCfg),
		Sms:     s.toProtoSms(smsCfg),
		Llm:     s.toProtoLlm(llmCfg),
		Oauth:   s.toProtoOauth(oauthCfg),
		Payment: s.toProtoPayment(paymentCfg),
	}, nil
}

// testFailureMessage keeps "not configured" friendly and passes real
// integration errors through (they're operator-facing diagnostics, not
// secrets).
func testFailureMessage(err, notConfigured error, friendly string) string {
	if errors.Is(err, notConfigured) {
		return friendly
	}
	return err.Error()
}

func (s *SystemService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
