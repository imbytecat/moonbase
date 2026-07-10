package captcha

import (
	"net/http"
	"time"

	captchaint "github.com/imbytecat/moonbase/integrations/captcha"
	"github.com/imbytecat/moonbase/integrations/captcha/altcha"
	"github.com/imbytecat/moonbase/integrations/captcha/geetest"
	"github.com/imbytecat/moonbase/integrations/captcha/turnstile"
)

func NewRegistry(store Store) captchaint.Registry {
	client := &http.Client{Timeout: 10 * time.Second}
	return captchaint.MustRegistry(
		turnstile.New(client),
		geetest.New(client),
		altcha.New(store.CaptchaAltchaKey),
	)
}
