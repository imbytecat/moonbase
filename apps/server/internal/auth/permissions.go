package auth

import "github.com/imbytecat/moonbase/server/internal/authcatalog"

// Permission is a stable key checked by the authz interceptor, plus its catalog
// description. The catalog is derived from the proto auth.v1.Permission enum
// (generated into authcatalog), not hand-written, because permissions must stay
// 1:1 with what RPCs enforce; roles (DB) reference these keys. Renaming a key is
// a breaking change for stored role_permissions — add a new enum value instead.
type Permission = authcatalog.PermissionEntry

// Catalog lists every grantable permission, in enum-declaration order. The
// admin UI renders it when editing a role; ListPermissions RPC serves it. It is
// generated from the Permission enum — the single source — so a new value with
// a description joins it automatically.
var Catalog = authcatalog.PermissionCatalog

// IsKnownPermission reports whether key is in the catalog (or the wildcard).
// Role writes validate against this so a typo can't silently grant nothing.
func IsKnownPermission(key string) bool {
	if key == WildcardPermission {
		return true
	}
	for _, p := range Catalog {
		if p.Key == key {
			return true
		}
	}
	return false
}
