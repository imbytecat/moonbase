package rpc

import (
	"testing"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
)

// TestPermissionEnumMatchesCatalog is the behavioral backstop for the generated
// permission catalog. auth.Catalog is now derived from the proto Permission
// enum (protoc-gen-permissions), so parity is structural rather than hand-kept;
// this catches the two things generation can still get wrong: a grantable enum
// value that shipped without a (moonbase.v1.description) — so it never became a
// catalog entry — and drift between the generator's key derivation and the
// runtime permissionKey/permissionEnum mapping the wire relies on.
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
