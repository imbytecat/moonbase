// Package settings defines the shared JSONB profile/binding carrier used by
// every infrastructure integration module.
package settings

import "slices"

// identifiable lets Integration look profiles up by id without knowing the
// payload type.
type identifiable interface {
	ProfileID() string
}

// Profile is the full generic surface an integration profile exposes:
// identity for Integration lookups, the provider wire value for driver
// registries, and a value-typed id setter for generic create flows.
type Profile[P any] interface {
	identifiable
	ProviderName() string
	WithID(id string) P
}

// Integration is the one shape every profile-based infrastructure integration
// shares: operators register any number of named connection profiles and
// bind each code-defined purpose to one or more of them. Most purposes are
// single-valued; third-party login and payment are multi-valued cases.
type Integration[P identifiable] struct {
	Profiles []P                 `json:"profiles"`
	Bindings map[string][]string `json:"bindings"`
}

func (c Integration[P]) Profile(id string) (P, bool) {
	for _, p := range c.Profiles {
		if p.ProfileID() == id {
			return p, true
		}
	}
	var zero P
	return zero, false
}

// ProfileFor resolves a single-valued purpose to its bound profile.
func (c Integration[P]) ProfileFor(purpose string) (P, bool) {
	ids := c.Bindings[purpose]
	if len(ids) == 0 {
		var zero P
		return zero, false
	}
	return c.Profile(ids[0])
}

// ProfilesFor resolves a multi-valued purpose to its bound profiles, in
// binding order; ids pointing at deleted profiles are skipped.
func (c Integration[P]) ProfilesFor(purpose string) []P {
	ids := c.Bindings[purpose]
	out := make([]P, 0, len(ids))
	for _, id := range ids {
		if p, ok := c.Profile(id); ok {
			out = append(out, p)
		}
	}
	return out
}

func (c Integration[P]) Bound(id string) (string, bool) {
	for purpose, ids := range c.Bindings {
		if slices.Contains(ids, id) {
			return purpose, true
		}
	}
	return "", false
}
