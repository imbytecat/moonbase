package settings

import (
	"slices"
	"testing"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
)

// TestIntegrationLookups pins the pure resolution logic behind every
// integration: profile lookup, single- and multi-valued purpose binding, order
// preservation, deleted-target skipping, and reverse binding lookup.
func TestIntegrationLookups(t *testing.T) {
	ch := Storage{
		Profiles: []systemcodec.StorageProfile{
			{Id: "a", Provider: "local"},
			{Id: "b", Provider: "s3"},
		},
		Bindings: map[string][]string{
			"avatars": {"a"},
			"uploads": {"b", "a"},
			"stale":   {"gone"}, // binding points at a deleted profile
		},
	}

	if p, ok := ch.Profile("b"); !ok || p.Provider != "s3" {
		t.Errorf("Profile(b) = %+v,%v, want the s3 profile", p, ok)
	}
	if _, ok := ch.Profile("missing"); ok {
		t.Error("Profile(missing) = true, want false")
	}

	if p, ok := ch.ProfileFor("avatars"); !ok || p.Id != "a" {
		t.Errorf("ProfileFor(avatars) = %+v,%v, want profile a", p, ok)
	}
	if _, ok := ch.ProfileFor("unbound"); ok {
		t.Error("ProfileFor(unbound) = true, want false")
	}
	if _, ok := ch.ProfileFor("stale"); ok {
		t.Error("ProfileFor(stale) = true, want false (bound profile deleted)")
	}

	ids := func(ps []systemcodec.StorageProfile) []string {
		out := make([]string, 0, len(ps))
		for _, p := range ps {
			out = append(out, p.Id)
		}
		return out
	}
	if got := ids(ch.ProfilesFor("uploads")); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("ProfilesFor(uploads) = %v, want binding order [b a]", got)
	}
	if got := ch.ProfilesFor("stale"); len(got) != 0 {
		t.Errorf("ProfilesFor(stale) = %v, want empty (target deleted)", got)
	}

	if purpose, ok := ch.Bound("a"); !ok || (purpose != "avatars" && purpose != "uploads") {
		t.Errorf("Bound(a) = %q,%v, want a purpose a is bound to", purpose, ok)
	}
	if _, ok := ch.Bound("b-unbound-elsewhere"); ok {
		t.Error("Bound(unknown) = true, want false")
	}
}
