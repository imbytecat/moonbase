package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/nyaruka/phonenumbers"

	settingsv1 "github.com/imbytecat/moonbase/server/internal/gen/settings/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/settings/v1/settingsv1connect"
	"github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

const siteAssetURLTTL = time.Hour

// SettingsService is the BUSINESS settings surface (settings.*). The
// infrastructure channels live in SystemService behind system.*.
type SettingsService struct {
	settings *settings.Store
	objects  storage.ObjectStore
	logger   *slog.Logger
}

func NewSettingsService(store *settings.Store, objects storage.ObjectStore, logger *slog.Logger) *SettingsService {
	return &SettingsService{settings: store, objects: objects, logger: logger}
}

var _ settingsv1connect.SettingsServiceHandler = (*SettingsService)(nil)

func (s *SettingsService) GetSettings(
	ctx context.Context,
	_ *connect.Request[settingsv1.GetSettingsRequest],
) (*connect.Response[settingsv1.GetSettingsResponse], error) {
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load auth settings", err)
	}
	siteCfg, err := s.settings.Site(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load site settings", err)
	}
	return connect.NewResponse(&settingsv1.GetSettingsResponse{
		Auth: toProtoAuthSettings(authCfg),
		Site: toProtoSiteSettings(siteCfg),
	}), nil
}

func (s *SettingsService) UpdateSettings(
	ctx context.Context,
	req *connect.Request[settingsv1.UpdateSettingsRequest],
) (*connect.Response[settingsv1.UpdateSettingsResponse], error) {
	if in := req.Msg.GetAuth(); in != nil {
		regions, err := normalizeRegions(in.GetAllowedPhoneRegions())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		next := settings.Auth{
			RegistrationEnabled: in.GetRegistrationEnabled(),
			AllowedPhoneRegions: regions,
			SignupIdentifiers:   in.GetSignupIdentifiers(),
		}
		if err := s.validateAuthSettings(ctx, next); err != nil {
			return nil, err
		}
		if err := s.settings.SetAuth(ctx, next); err != nil {
			return nil, s.internal(ctx, "save auth settings", err)
		}
	}
	if in := req.Msg.GetSite(); in != nil {
		if err := s.settings.SetSite(ctx, settings.Site{
			Name:        strings.TrimSpace(in.GetName()),
			Description: strings.TrimSpace(in.GetDescription()),
			LogoKey:     in.GetLogoKey(),
			FaviconKey:  in.GetFaviconKey(),
			Copyright:   strings.TrimSpace(in.GetCopyright()),
			IcpBeian:    strings.TrimSpace(in.GetIcpBeian()),
		}); err != nil {
			return nil, s.internal(ctx, "save site settings", err)
		}
	}
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load auth settings", err)
	}
	siteCfg, err := s.settings.Site(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load site settings", err)
	}
	return connect.NewResponse(&settingsv1.UpdateSettingsResponse{
		Auth: toProtoAuthSettings(authCfg),
		Site: toProtoSiteSettings(siteCfg),
	}), nil
}

// GetSiteInfo is the PUBLIC site identity: login/register pages and the
// document head render it before any session exists. Asset keys resolve to
// fetchable URLs best-effort — storage being down just means no logo.
func (s *SettingsService) GetSiteInfo(
	ctx context.Context,
	_ *connect.Request[settingsv1.GetSiteInfoRequest],
) (*connect.Response[settingsv1.GetSiteInfoResponse], error) {
	siteCfg, err := s.settings.Site(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load site settings", err)
	}
	out := &settingsv1.GetSiteInfoResponse{
		Name:        siteCfg.Name,
		Description: siteCfg.Description,
		Copyright:   siteCfg.Copyright,
		IcpBeian:    siteCfg.IcpBeian,
	}
	if siteCfg.LogoKey != "" {
		if u, err := s.objects.ResolveURL(ctx, storage.PurposeSiteAssets, siteCfg.LogoKey, siteAssetURLTTL); err == nil {
			out.LogoUrl = u
		}
	}
	if siteCfg.FaviconKey != "" {
		if u, err := s.objects.ResolveURL(ctx, storage.PurposeSiteAssets, siteCfg.FaviconKey, siteAssetURLTTL); err == nil {
			out.FaviconUrl = u
		}
	}
	return connect.NewResponse(out), nil
}

// validateAuthSettings rejects policies the register flow could not honor
// (Logto's enabled_connector_not_found pattern): channel-backed identifiers
// are always code-verified at signup, so a form collecting phone with no SMS
// channel — or email with no email channel — could never submit successfully.
// Shape (allowed values, uniqueness) is already enforced by protovalidate.
func (s *SettingsService) validateAuthSettings(ctx context.Context, next settings.Auth) error {
	if slices.Contains(next.SignupIdentifiers, settings.SignupIdentifierPhone) {
		smsCfg, err := s.settings.Sms(ctx)
		if err != nil {
			return s.internal(ctx, "load sms settings", err)
		}
		if !sms.Usable(smsCfg, sms.PurposeVerification) {
			return connect.NewError(connect.CodeFailedPrecondition,
				errors.New("phone signup requires a configured SMS channel"))
		}
	}
	if slices.Contains(next.SignupIdentifiers, settings.SignupIdentifierEmail) {
		emailCfg, err := s.settings.Email(ctx)
		if err != nil {
			return s.internal(ctx, "load email settings", err)
		}
		if !mail.Usable(emailCfg, mail.PurposeAuth) {
			return connect.NewError(connect.CodeFailedPrecondition,
				errors.New("email signup requires a configured email channel"))
		}
	}
	return nil
}

// normalizeRegions uppercases and validates ISO region codes against
// libphonenumber's supported set, so a typo can't silently lock everyone out.
func normalizeRegions(in []string) ([]string, error) {
	supported := phonenumbers.GetSupportedRegions()
	out := make([]string, 0, len(in))
	for _, r := range in {
		code := strings.ToUpper(strings.TrimSpace(r))
		if code == "" {
			continue
		}
		if !supported[code] {
			return nil, fmt.Errorf("unknown phone region %q", r)
		}
		out = append(out, code)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func toProtoAuthSettings(a settings.Auth) *settingsv1.AuthSettings {
	regions := a.AllowedPhoneRegions
	if regions == nil {
		regions = []string{}
	}
	return &settingsv1.AuthSettings{
		RegistrationEnabled: a.RegistrationEnabled,
		AllowedPhoneRegions: regions,
		SignupIdentifiers:   a.EffectiveSignupIdentifiers(),
	}
}

func toProtoSiteSettings(v settings.Site) *settingsv1.SiteSettings {
	return &settingsv1.SiteSettings{
		Name:        v.Name,
		Description: v.Description,
		LogoKey:     v.LogoKey,
		FaviconKey:  v.FaviconKey,
		Copyright:   v.Copyright,
		IcpBeian:    v.IcpBeian,
	}
}

func (s *SettingsService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
