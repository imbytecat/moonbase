package auth

import "testing"

// TestCatalog is the behavioral spec for the permission catalog: its exact
// key+description entries, in enum-declaration order. The catalog is derived
// from the proto Permission enum (descriptions carried as enum-value options,
// generated into package authcatalog), so this pins the derivation to the
// established set — the proto move stays lossless and ListPermissions keeps
// serving the same ordered list to the admin UI.
func TestCatalog(t *testing.T) {
	want := []Permission{
		{Key: "report.read", Description: "View dashboard reports and statistics"},
		{Key: "user.read", Description: "View users and their roles"},
		{Key: "user.write", Description: "Create, update and delete users; assign roles"},
		{Key: "role.read", Description: "View roles and permissions"},
		{Key: "role.write", Description: "Create, update and delete roles"},
		{Key: "settings.read", Description: "View business settings"},
		{Key: "settings.write", Description: "Change business settings"},
		{
			Key:         "system.read",
			Description: "View system settings (storage, captcha, email, SMS, AI, payments)",
		},
		{Key: "system.write", Description: "Change system settings and run channel tests"},
		{Key: "workflow.read", Description: "View workflow runs and their steps"},
		{Key: "workflow.write", Description: "Cancel, resume and trigger workflows"},
		{Key: "audit.read", Description: "View the audit trail of admin actions"},
		{Key: "payment.read", Description: "View payment orders"},
		{Key: "payment.write", Description: "Create, sync and refund payment orders"},
	}
	if len(Catalog) != len(want) {
		t.Fatalf("catalog has %d entries, want %d", len(Catalog), len(want))
	}
	for i, w := range want {
		if Catalog[i] != w {
			t.Errorf("catalog[%d] = %+v, want %+v", i, Catalog[i], w)
		}
	}
}
