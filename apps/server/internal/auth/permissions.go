package auth

// Permission is a stable key checked by the authz interceptor. The catalog is
// code (not DB rows) because permissions must stay 1:1 with what RPCs actually
// enforce; roles (DB) reference these keys. Renaming a key is a breaking
// change for stored role_permissions — add new keys instead.
type Permission struct {
	Key         string
	Description string
}

// Catalog lists every grantable permission, grouped by domain. The admin UI
// renders this list when editing a role; ListPermissions RPC serves it.
var Catalog = []Permission{
	{Key: "report.read", Description: "View dashboard reports and statistics"},
	{Key: "user.read", Description: "View users and their roles"},
	{Key: "user.write", Description: "Create, update and delete users; assign roles"},
	{Key: "role.read", Description: "View roles and permissions"},
	{Key: "role.write", Description: "Create, update and delete roles"},
	{Key: "settings.read", Description: "View business settings"},
	{Key: "settings.write", Description: "Change business settings"},
	{Key: "system.read", Description: "View system settings (storage, captcha, email, SMS, AI, payments)"},
	{Key: "system.write", Description: "Change system settings and run channel tests"},
	{Key: "workflow.read", Description: "View workflow runs and their steps"},
	{Key: "workflow.write", Description: "Cancel, resume and trigger workflows"},
	{Key: "audit.read", Description: "View the audit trail of admin actions"},
	{Key: "payment.read", Description: "View payment orders"},
	{Key: "payment.write", Description: "Create, sync and refund payment orders"},
}

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
