package pay

import (
	"fmt"
	"maps"
	"slices"

	"github.com/imbytecat/moonbase/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

func Schemas() map[string]schema.Schema {
	out := make(map[string]schema.Schema, len(drivers))
	for name, d := range drivers {
		out[name] = d.schema
	}
	return out
}

type methodDefinition struct {
	provider string
	method   Method
}

var methodDefinitions = []methodDefinition{
	{provider: "alipay", method: Method{ID: "precreate", Kind: CredentialQR}},
	{provider: "alipay", method: Method{ID: "page_pay", Kind: CredentialRedirect, Inputs: []Input{InputReturnURL}}},
	{provider: "alipay", method: Method{ID: "wap_pay", Kind: CredentialRedirect, Inputs: []Input{InputReturnURL}}},
	{provider: "alipay", method: Method{ID: "create", Kind: CredentialParams, Inputs: []Input{InputPayerID}}},
	{provider: "alipay", method: Method{ID: "app_pay", Kind: CredentialParams}},
	{provider: "wechat", method: Method{ID: "native", Kind: CredentialQR}},
	{provider: "wechat", method: Method{ID: "h5", Kind: CredentialRedirect}},
	{provider: "wechat", method: Method{ID: "jsapi", Kind: CredentialParams, Inputs: []Input{InputPayerID}}},
	{provider: "wechat", method: Method{ID: "app", Kind: CredentialParams}},
}

// methodCatalog builds a provider's product list in checkout display order.
func methodCatalog(provider string) []Method {
	var out []Method
	for _, d := range methodDefinitions {
		if d.provider != provider {
			continue
		}
		out = append(out, d.method)
	}
	return out
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return slices.Sorted(maps.Keys(drivers))
}

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate every Gateway call enforces.
func ProfileUsable(p kitsettings.GenericProfile) bool {
	d, ok := drivers[p.Provider]
	return ok && d.schema.Usable(p.Config) && paymentAuthUsable(p)
}

// Catalog lists a provider's products in display order.
func Catalog(provider string) []Method {
	return drivers[provider].ops.catalog
}

// Methods lists every product id across all drivers, sorted — the union the
// proto method `in:` constraint mirrors (TestPaymentMethodsMatchContract).
func Methods() []string {
	var out []string
	for _, name := range Providers() {
		for _, m := range drivers[name].ops.catalog {
			out = append(out, m.ID)
		}
	}
	slices.Sort(out)
	return out
}

// Offered lists the products a profile presents at checkout: those the
// merchant signed for (p.Methods), in the driver's display order. Empty
// p.Methods means "all of the provider's products" so profiles saved before
// per-product selection keep working.
func Offered(p kitsettings.GenericProfile) []string {
	methods := profileMethods(p)
	out := make([]string, 0, len(drivers[p.Provider].ops.catalog))
	for _, m := range drivers[p.Provider].ops.catalog {
		if len(methods) == 0 || slices.Contains(methods, m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}

func profileMethods(p kitsettings.GenericProfile) []string {
	switch raw := p.Config["methods"].(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func ProfileMethods(p kitsettings.GenericProfile) []string {
	return profileMethods(p)
}

func methodByID(provider, method string) (Method, bool) {
	for _, m := range drivers[provider].ops.catalog {
		if m.ID == method {
			return m, true
		}
	}
	return Method{}, false
}

// ValidateMethods reports whether every id in methods is a product of the
// provider's catalog. An empty list is valid — it means "all products". This is
// the save-time guard for a profile's signed products in Profile.config.methods.
func ValidateMethods(provider string, methods []string) error {
	for _, id := range methods {
		if _, ok := methodByID(provider, id); !ok {
			return fmt.Errorf("%w: %q for provider %q", ErrUnknownMethod, id, provider)
		}
	}
	return nil
}

// KindOf reports how an order's credential should be consumed by the client.
func KindOf(provider, method string) CredentialKind {
	m, _ := methodByID(provider, method)
	return m.Kind
}

// InputsOf lists the extra fields a product collects at checkout.
func InputsOf(provider, method string) []Input {
	m, _ := methodByID(provider, method)
	return m.Inputs
}

// Currency is the settlement currency of a provider's orders. The system is
// CNY-only (see docs/adr/0001): both CN drivers settle in CNY, and this exists
// only so the payment_orders.currency column carries an honest value — not as a
// multi-currency seam.
func Currency(provider string) string {
	if c := drivers[provider].ops.currency; c != "" {
		return c
	}
	return "CNY"
}
