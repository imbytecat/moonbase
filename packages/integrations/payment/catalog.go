package pay

import (
	"fmt"
	"slices"
)

// ProfileProducts returns the signed products in driver declaration order.
// An empty stored products list means every product published by the driver.
func (r Registry) ConfiguredProducts(provider string, values map[string]any) []string {
	descriptor, ok := r.Describe(provider)
	if !ok {
		return nil
	}
	configured := configuredProductIDs(values)
	out := make([]string, 0, len(descriptor.Products))
	for _, product := range descriptor.Products {
		if len(configured) == 0 || slices.Contains(configured, product.ID) {
			out = append(out, product.ID)
		}
	}
	return out
}

func configuredProductIDs(values map[string]any) []string {
	switch raw := values["products"].(type) {
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

func (r Registry) ValidateProducts(provider string, products []string) error {
	descriptor, ok := r.Describe(provider)
	if !ok {
		return ErrNotConfigured
	}
	for _, id := range products {
		if !slices.ContainsFunc(
			descriptor.Products,
			func(product ProductDescriptor) bool { return product.ID == id },
		) {
			return fmt.Errorf("%w: %q for provider %q", ErrUnknownMethod, id, provider)
		}
	}
	return nil
}

func Currency(provider string) string {
	// The payment domain is deliberately CNY-only (ADR-0001).
	return "CNY"
}
