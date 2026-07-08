package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	mail "github.com/imbytecat/moonbase/server/integrations/email"
	"github.com/imbytecat/moonbase/server/integrations/sms"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/system/v1/systemv1connect"
	"github.com/imbytecat/moonbase/server/internal/llm"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
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
	settings      *settings.Store
	repo          repository.Querier
	storageTester storage.ConnectionTester
	mailer        mail.Sender
	smser         sms.Sender
	chatter       llm.Chatter
	logger        *slog.Logger
}

func NewSystemService(
	store *settings.Store,
	repo repository.Querier,
	storageTester storage.ConnectionTester,
	mailer mail.Sender,
	smser sms.Sender,
	chatter llm.Chatter,
	logger *slog.Logger,
) *SystemService {
	return &SystemService{
		settings:      store,
		repo:          repo,
		storageTester: storageTester,
		mailer:        mailer,
		smser:         smser,
		chatter:       chatter,
		logger:        logger,
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
		Storage: toProtoStorage(st),
		Captcha: toProtoCaptcha(captchaCfg),
		Email:   toProtoEmail(emailCfg),
		Sms:     toProtoSms(smsCfg),
		Llm:     toProtoLlm(llmCfg),
		Oauth:   toProtoOauth(oauthCfg),
		Payment: toProtoPayment(paymentCfg),
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
