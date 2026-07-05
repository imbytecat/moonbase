package rpc

import (
	"strings"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
)

// permissionKey maps a proto Permission enum to its wire/DB string key: the
// admin wildcard for PERMISSION_ALL, empty for UNSPECIFIED, otherwise the enum
// name lower-cased with its last underscore turned into a dot
// (PERMISSION_USER_READ -> "user.read"). The DB, Identity and authz table stay
// on these string keys; the enum is only the typed surface on the wire. Kept
// in lockstep with auth.Catalog by TestPermissionEnumMatchesCatalog.
func permissionKey(p authv1.Permission) string {
	switch p {
	case authv1.Permission_PERMISSION_UNSPECIFIED:
		return ""
	case authv1.Permission_PERMISSION_ALL:
		return auth.WildcardPermission
	}
	name := strings.ToLower(strings.TrimPrefix(p.String(), "PERMISSION_"))
	if i := strings.LastIndex(name, "_"); i >= 0 {
		name = name[:i] + "." + name[i+1:]
	}
	return name
}

func permissionEnum(key string) authv1.Permission {
	switch key {
	case "":
		return authv1.Permission_PERMISSION_UNSPECIFIED
	case auth.WildcardPermission:
		return authv1.Permission_PERMISSION_ALL
	}
	name := "PERMISSION_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if v, ok := authv1.Permission_value[name]; ok {
		return authv1.Permission(v)
	}
	return authv1.Permission_PERMISSION_UNSPECIFIED
}

// permissionKeys drops UNSPECIFIED so a stray zero value never becomes an empty
// DB row.
func permissionKeys(ps []authv1.Permission) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if k := permissionKey(p); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func permissionEnums(keys []string) []authv1.Permission {
	out := make([]authv1.Permission, 0, len(keys))
	for _, k := range keys {
		out = append(out, permissionEnum(k))
	}
	return out
}
