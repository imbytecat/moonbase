package mail

import (
	"net/http"

	"github.com/imbytecat/moonbase/integrations/email"
	"github.com/imbytecat/moonbase/integrations/email/cloudflare"
	"github.com/imbytecat/moonbase/integrations/email/smtp"
)

func NewRegistry(httpClient *http.Client) email.Registry {
	return email.MustRegistry(
		smtp.New(),
		cloudflare.New(httpClient),
	)
}
