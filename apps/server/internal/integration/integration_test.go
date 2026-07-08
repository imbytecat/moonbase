package integration

import (
	"slices"
	"testing"
)

type fakeProfile struct {
	provider string
	ready    bool
}

func (f fakeProfile) ProviderName() string { return f.provider }

func TestCatalogKnown(t *testing.T) {
	cat := Catalog{"login", "notify"}
	if !cat.Known("login") {
		t.Error("Known(login) = false, want true")
	}
	if cat.Known("missing") {
		t.Error("Known(missing) = true, want false")
	}
}

func TestRegistry(t *testing.T) {
	reg := Registry[fakeProfile, string]{
		"beta":  {Usable: func(p fakeProfile) bool { return p.ready }, Ops: "beta-ops"},
		"alpha": {Usable: func(p fakeProfile) bool { return p.ready }, Ops: "alpha-ops"},
	}

	if got := reg.Names(); !slices.Equal(got, []string{"alpha", "beta"}) {
		t.Errorf("Names() = %v, want sorted [alpha beta]", got)
	}

	cases := []struct {
		name       string
		profile    fakeProfile
		wantUsable bool
		wantOps    string
		wantOK     bool
	}{
		{"registered and ready", fakeProfile{"alpha", true}, true, "alpha-ops", true},
		{"registered but not ready", fakeProfile{"beta", false}, false, "", false},
		{"unregistered provider", fakeProfile{"gamma", true}, false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reg.ProfileUsable(tc.profile); got != tc.wantUsable {
				t.Errorf("ProfileUsable = %v, want %v", got, tc.wantUsable)
			}
			ops, ok := reg.OpsFor(tc.profile)
			if ok != tc.wantOK || ops != tc.wantOps {
				t.Errorf("OpsFor = (%q,%v), want (%q,%v)", ops, ok, tc.wantOps, tc.wantOK)
			}
		})
	}
}
