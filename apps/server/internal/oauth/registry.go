package oauth

import (
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
	"github.com/imbytecat/moonbase/integrations/oauth/oidc"
	"github.com/imbytecat/moonbase/integrations/oauth/wechat"
)

func NewRegistry() oauthint.Registry { return oauthint.MustRegistry(oidc.New(), wechat.New()) }
