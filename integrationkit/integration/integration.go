// Package integration holds the two primitives every infrastructure
// integration (storage, captcha, email, sms, llm, oauth, payment) is built
// from: a code-defined purpose Catalog and a provider-keyed driver Registry.
// Each integration package declares its purposes and drivers with these types
// so the shared vocabulary (known purpose, usable profile, provider names) is
// implemented exactly once — only the per-integration Ops shape differs.
package integration

import (
	"maps"
	"slices"
)

// Catalog is an integration's purpose list, served to the admin UI in display
// order. Purposes are code, not data: each is a fixed slot the application
// addresses the integration through, and operators bind each one to connection
// profiles at runtime. Adding a feature that uses an integration = adding a
// purpose to that integration's catalog.
type Catalog []string

// Known reports whether key is in the catalog — binding writes validate
// against it so a typo can't create a dangling binding.
func (c Catalog) Known(key string) bool {
	return slices.Contains(c, key)
}

// Provider names the driver a profile selects; every settings profile
// implements it.
type Provider interface {
	ProviderName() string
}

// Driver pairs a provider's usable-check with its integration-specific ops
// (the send/verify/complete/... function or bundle). Ops signatures are
// deliberately DIFFERENT per integration — only this wrapper shape is shared.
type Driver[P Provider, Ops any] struct {
	// Usable reports whether the profile carries everything the provider
	// needs; every action gates on it.
	Usable func(P) bool
	Ops    Ops
}

// Registry maps the wire value of a profile's provider field to its driver.
// TestProviderRegistriesMatchContract (internal/rpc) keeps every registry
// aligned with the proto `in:` constraint.
type Registry[P Provider, Ops any] map[string]Driver[P, Ops]

// Names lists registered driver names, sorted.
func (r Registry[P, Ops]) Names() []string {
	return slices.Sorted(maps.Keys(r))
}

// ProfileUsable reports whether the profile's driver is registered and fully
// configured.
func (r Registry[P, Ops]) ProfileUsable(p P) bool {
	d, ok := r[p.ProviderName()]
	return ok && d.Usable(p)
}

// Ops returns the driver ops for a usable profile; ok=false means the
// provider is unregistered or not fully configured.
func (r Registry[P, Ops]) OpsFor(p P) (Ops, bool) {
	d, ok := r[p.ProviderName()]
	if !ok || !d.Usable(p) {
		var zero Ops
		return zero, false
	}
	return d.Ops, true
}
