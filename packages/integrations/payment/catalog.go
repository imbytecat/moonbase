package pay

import (
	"fmt"
	"slices"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

func Providers() []string { return Registry.Names() }

func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(profile.Provider, profile.Config)
}

// ProfileProducts returns the signed products in driver declaration order.
// An empty stored products list means every product published by the driver.
func ProfileProducts(profile kitsettings.GenericProfile) []string {
	descriptor, ok := Describe(profile.Provider)
	if !ok {
		return nil
	}
	configured := profileConfiguredProducts(profile)
	out := make([]string, 0, len(descriptor.Products))
	for _, product := range descriptor.Products {
		if len(configured) == 0 || slices.Contains(configured, product.ID) {
			out = append(out, product.ID)
		}
	}
	return out
}

func profileConfiguredProducts(profile kitsettings.GenericProfile) []string {
	switch raw := profile.Config["products"].(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func ProfileConfiguredProducts(profile kitsettings.GenericProfile) []string {
	return profileConfiguredProducts(profile)
}

func ValidateProducts(provider string, products []string) error {
	descriptor, ok := Describe(provider)
	if !ok {
		return ErrNotConfigured
	}
	for _, id := range products {
		if !slices.ContainsFunc(descriptor.Products, func(product ProductDescriptor) bool { return product.ID == id }) {
			return fmt.Errorf("%w: %q for provider %q", ErrUnknownMethod, id, provider)
		}
	}
	return nil
}

func Currency(provider string) string {
	// The payment domain is deliberately CNY-only (ADR-0001).
	return "CNY"
}
