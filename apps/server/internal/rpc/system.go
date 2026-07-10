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

// systemBase carries the two dependencies every integration admin surface
// shares — the settings store it reads and writes, and the logger it reports
// internal failures through. Each per-integration handler (system_<x>.go)
// embeds it, so none of them re-declare settings/logger or the internal error
// helper.
type systemBase struct {
	settings *settings.Store
	logger   *slog.Logger
}

func (b *systemBase) internal(ctx context.Context, op string, err error) error {
	b.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// SystemService is a thin facade over the per-integration admin handlers. Each
// handler (system_<integration>.go) owns only its own dependencies; the facade
// composes them by embedding and satisfies the generated SystemServiceHandler
// through Go method promotion. The only behaviour it owns is the
// cross-integration settings snapshot — every other RPC is promoted from an
// embedded handler. Adding an integration = one more embedded field here plus
// its own file; it never widens a shared dependency list.
type SystemService struct {
	*systemStorage
	*systemCaptcha
	*systemEmail
	*systemSms
	*systemLlm
	*systemOauth
	*systemPayment
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
	base := systemBase{settings: store, logger: logger}
	return &SystemService{
		systemStorage: &systemStorage{systemBase: base, storageTester: storageTester, storageRegistry: storageRegistry},
		systemCaptcha: &systemCaptcha{systemBase: base, captchaRegistry: captchaRegistry},
		systemEmail:   &systemEmail{systemBase: base, emailRegistry: emailRegistry, mailer: mailer},
		systemSms:     &systemSms{systemBase: base, smsRegistry: smsRegistry, smser: smser},
		systemLlm:     &systemLlm{systemBase: base, llmRegistry: llmRegistry, chatter: chatter},
		systemOauth:   &systemOauth{systemBase: base, oauthRegistry: oauthRegistry, repo: repo},
		systemPayment: &systemPayment{systemBase: base, paymentRegistry: paymentRegistry},
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

// snapshot assembles the read-only view of every integration by delegating to
// each handler's own settings projection, then composes them in proto order.
// Each xxxSnapshot method is promoted from its embedded handler.
func (s *SystemService) snapshot(ctx context.Context) (*systemv1.GetSystemSettingsResponse, error) {
	storageSettings, err := s.storageSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	captchaSettings, err := s.captchaSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	emailSettings, err := s.emailSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	smsSettings, err := s.smsSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	llmSettings, err := s.llmSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	oauthSettings, err := s.oauthSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	paymentSettings, err := s.paymentSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &systemv1.GetSystemSettingsResponse{
		Storage: storageSettings,
		Captcha: captchaSettings,
		Email:   emailSettings,
		Sms:     smsSettings,
		Llm:     llmSettings,
		Oauth:   oauthSettings,
		Payment: paymentSettings,
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
