package pay

import (
	payment "github.com/imbytecat/moonbase/integrations/payment"
	"github.com/imbytecat/moonbase/integrations/payment/alipay"
	"github.com/imbytecat/moonbase/integrations/payment/wechat"
)

// NewRegistry is the server composition root for the payment providers. The
// same immutable instance is injected into the execution facade and system
// management surface.
func NewRegistry() payment.Registry {
	return payment.MustRegistry(
		alipay.New(),
		wechat.New(),
	)
}
