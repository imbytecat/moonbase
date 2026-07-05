package rpc

import (
	"testing"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
)

// TestPermissionEnumMatchesCatalog locks the proto Permission enum to the Go
// auth.Catalog: every catalog key must round-trip through the enum, and every
// enum value (besides UNSPECIFIED and the admin wildcard) must map back to a
// catalog key. A drift on either side — a new enum value without a catalog
// entry, or a key the derive rule can't reproduce — fails here instead of
// silently corrupting authorization mapping.
func TestPermissionEnumMatchesCatalog(t *testing.T) {
	catalog := map[string]bool{}
	for _, p := range auth.Catalog {
		catalog[p.Key] = true
		e := permissionEnum(p.Key)
		if e == authv1.Permission_PERMISSION_UNSPECIFIED {
			t.Errorf("catalog key %q has no matching enum value", p.Key)
			continue
		}
		if got := permissionKey(e); got != p.Key {
			t.Errorf("round-trip mismatch: %q -> %v -> %q", p.Key, e, got)
		}
	}
	for name, num := range authv1.Permission_value {
		p := authv1.Permission(num)
		if p == authv1.Permission_PERMISSION_UNSPECIFIED || p == authv1.Permission_PERMISSION_ALL {
			continue
		}
		if key := permissionKey(p); !catalog[key] {
			t.Errorf("enum %s maps to %q which is not in auth.Catalog", name, key)
		}
	}
}
