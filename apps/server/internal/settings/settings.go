// Package settings provides typed access to the admin-managed key/value
// settings table (JSONB values). Every key ships defaults, so a fresh
// database needs no seeding: a missing row reads as the zero config.
//
// Infrastructure integrations share ONE shape — Integration[P]: named
// connection profiles plus purpose → profile bindings. Only the profile
// payload P differs per integration; each driver inside P keeps its own
// config struct (an SMTP server and a REST API have nothing in common but
// the seam).
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const (
	keyAuth    = "auth"
	keyStorage = "storage"
	// keyStorageSignKey holds the HMAC secret behind local-storage signed
	// URLs; generated once on first use so deployments need no extra config.
	keyStorageSignKey = "storageSignKey"
	// keyCaptchaAltchaKey holds the HMAC secret signing built-in ALTCHA
	// captcha challenges; generated once on first use like the storage sign
	// key.
	keyCaptchaAltchaKey       = "captchaAltchaKey"
	keyCaptcha                = "captcha"
	keyEmail                  = "email"
	keySms                    = "sms"
	keySite                   = "site"
	keyLlm                    = "llm"
	keyOauth                  = "oauth"
	keyPayment                = "payment"
	keyPaymentCheckoutSignKey = "paymentCheckoutSignKey"
)

const (
	SignupIdentifierUsername = "username"
	SignupIdentifierEmail    = "email"
	SignupIdentifierPhone    = "phone"
)

type Auth struct {
	RegistrationEnabled bool `json:"registrationEnabled"`
	// ISO 3166-1 alpha-2 codes; empty = any region.
	AllowedPhoneRegions []string `json:"allowedPhoneRegions"`
	// Subset of {username, email, phone}; empty means username-only (the one
	// identifier that needs no channel). Email and phone are always
	// code-verified at signup. Use EffectiveSignupIdentifiers.
	SignupIdentifiers []string `json:"signupIdentifiers"`
}

// EffectiveSignupIdentifiers applies the username-only default so callers
// never see an empty policy.
func (a Auth) EffectiveSignupIdentifiers() []string {
	if len(a.SignupIdentifiers) == 0 {
		return []string{SignupIdentifierUsername}
	}
	return a.SignupIdentifiers
}

// Site is the business-facing site identity: name, branding assets and the
// legal footer. Logo/favicon are file ledger references (file ids); the files
// stay alive via matching file_attachments rows (owner_type "site").
type Site struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	LogoFileID    string `json:"logoFileId"`
	FaviconFileID string `json:"faviconFileId"`
	Copyright     string `json:"copyright"`
	IcpBeian      string `json:"icpBeian"`
}

type Profile[P any] = kitsettings.Profile[P]

type Integration[P kitsettings.Profile[P]] = kitsettings.Integration[P]

type Storage = Integration[kitsettings.GenericProfile]

type Captcha = Integration[kitsettings.GenericProfile]

type Email = Integration[kitsettings.GenericProfile]

type Sms = Integration[kitsettings.GenericProfile]

type Llm = Integration[kitsettings.GenericProfile]

type OAuth = Integration[kitsettings.GenericProfile]

type Payment = Integration[kitsettings.GenericProfile]

const (
	PaymentAuthPublicKey    = "public_key"
	PaymentAuthCert         = "cert"
	PaymentAuthPlatformCert = "platform_cert"
)

func ProfileByKey(cfg OAuth, key string) (kitsettings.GenericProfile, bool) {
	for _, p := range cfg.Profiles {
		if profileKey(p) == key {
			return p, true
		}
	}
	return kitsettings.GenericProfile{}, false
}

func profileKey(p kitsettings.GenericProfile) string {
	s, _ := p.Config["key"].(string)
	return s
}

type Store struct {
	repo repository.Querier
}

func NewStore(repo repository.Querier) *Store {
	return &Store{repo: repo}
}

func (s *Store) Auth(ctx context.Context) (Auth, error) {
	var v Auth
	err := s.get(ctx, keyAuth, &v)
	return v, err
}

func (s *Store) SetAuth(ctx context.Context, v Auth) error {
	return s.set(ctx, keyAuth, v)
}

func (s *Store) Site(ctx context.Context) (Site, error) {
	var v Site
	err := s.get(ctx, keySite, &v)
	return v, err
}

// SetSite persists the site identity and transfers the logo/favicon
// attachments to the referenced files in one atomic statement, so a replaced
// brand asset's old file goes unattached for the sweep (ADR-0003).
func (s *Store) SetSite(ctx context.Context, v Site) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode site settings: %w", err)
	}
	return s.repo.SetSiteWithAssets(ctx, repository.SetSiteWithAssetsParams{
		Site:          raw,
		LogoFileID:    fileIDParam(v.LogoFileID),
		FaviconFileID: fileIDParam(v.FaviconFileID),
	})
}

// fileIDParam turns a file-id string into a nullable uuid: empty or malformed
// becomes NULL, clearing that brand slot's attachment.
func fileIDParam(id string) pgtype.UUID {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func (s *Store) Storage(ctx context.Context) (Storage, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keyStorage)
}

func (s *Store) SetStorage(ctx context.Context, v Storage) error {
	return s.set(ctx, keyStorage, v)
}

func (s *Store) Captcha(ctx context.Context) (Captcha, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keyCaptcha)
}

func (s *Store) SetCaptcha(ctx context.Context, v Captcha) error {
	return s.set(ctx, keyCaptcha, v)
}

func (s *Store) Email(ctx context.Context) (Email, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keyEmail)
}

func (s *Store) SetEmail(ctx context.Context, v Email) error {
	return s.set(ctx, keyEmail, v)
}

func (s *Store) Sms(ctx context.Context) (Sms, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keySms)
}

func (s *Store) SetSms(ctx context.Context, v Sms) error {
	return s.set(ctx, keySms, v)
}

func (s *Store) Llm(ctx context.Context) (Llm, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keyLlm)
}

func (s *Store) SetLlm(ctx context.Context, v Llm) error {
	return s.set(ctx, keyLlm, v)
}

func (s *Store) Oauth(ctx context.Context) (OAuth, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keyOauth)
}

func (s *Store) SetOauth(ctx context.Context, v OAuth) error {
	return s.set(ctx, keyOauth, v)
}

func (s *Store) Payment(ctx context.Context) (Payment, error) {
	return getIntegration[kitsettings.GenericProfile](ctx, s, keyPayment)
}

func (s *Store) SetPayment(ctx context.Context, v Payment) error {
	return s.set(ctx, keyPayment, v)
}

func getIntegration[P kitsettings.Profile[P]](ctx context.Context, s *Store, key string) (Integration[P], error) {
	var v Integration[P]
	if err := s.get(ctx, key, &v); err != nil {
		return v, err
	}
	if v.Bindings == nil {
		v.Bindings = map[string][]string{}
	}
	return v, nil
}

func (s *Store) get(ctx context.Context, key string, out any) error {
	row, err := s.repo.GetSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get setting %s: %w", key, err)
	}
	if err := json.Unmarshal(row.Value, out); err != nil {
		return fmt.Errorf("decode setting %s: %w", key, err)
	}
	return nil
}

func (s *Store) set(ctx context.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode setting %s: %w", key, err)
	}
	if err := s.repo.UpsertSetting(ctx, repository.UpsertSettingParams{Key: key, Value: raw}); err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}
