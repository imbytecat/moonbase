package server

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/imbytecat/moonbase/server/internal/auth"

	// Register every API proto package so the registry walk below sees them.
	// A new service only needs its generated import added here (the compiler
	// reminds you the moment you reference its procedures in authz.go).
	_ "github.com/imbytecat/moonbase/server/internal/gen/audit/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/notification/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/payment/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/report/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/role/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/settings/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/user/v1"
	_ "github.com/imbytecat/moonbase/server/internal/gen/workflow/v1"
)

// TestAuthzRulesCoverEveryProcedure fails when an RPC exists in the proto
// registry without an authz rule (or a rule points at a deleted RPC). This is
// what makes "agent adds an RPC, forgets authz" a red test instead of an
// unprotected endpoint: the interceptor denies unknown procedures by default,
// and this test explains which entry is missing.
func TestAuthzRulesCoverEveryProcedure(t *testing.T) {
	registered := map[string]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		// Only our own API packages; skip google/*, buf/* dependencies.
		if !strings.HasPrefix(fd.Path(), "audit/") &&
			!strings.HasPrefix(fd.Path(), "auth/") &&
			!strings.HasPrefix(fd.Path(), "notification/") &&
			!strings.HasPrefix(fd.Path(), "payment/") &&
			!strings.HasPrefix(fd.Path(), "report/") &&
			!strings.HasPrefix(fd.Path(), "role/") &&
			!strings.HasPrefix(fd.Path(), "settings/") &&
			!strings.HasPrefix(fd.Path(), "storage/") &&
			!strings.HasPrefix(fd.Path(), "system/") &&
			!strings.HasPrefix(fd.Path(), "user/") &&
			!strings.HasPrefix(fd.Path(), "workflow/") {
			return true
		}
		services := fd.Services()
		for i := range services.Len() {
			svc := services.Get(i)
			methods := svc.Methods()
			for j := range methods.Len() {
				proc := "/" + string(svc.FullName()) + "/" + string(methods.Get(j).Name())
				registered[proc] = true
				if _, ok := authzRules[proc]; !ok {
					t.Errorf("RPC %s has no authz rule — add it to authzRules in authz.go", proc)
				}
			}
		}
		return true
	})

	if len(registered) == 0 {
		t.Fatal("no services found in the proto registry — check the blank imports above")
	}
	for proc := range authzRules {
		if !registered[proc] {
			t.Errorf("authz rule for %s references an unregistered RPC — remove or fix it", proc)
		}
	}
}

// TestAuthzRulePermissionsExist guards against typos: every permission named
// in the table must be in the catalog, so a rule can't demand a permission
// that no role could ever grant.
func TestAuthzRulePermissionsExist(t *testing.T) {
	for proc, rule := range authzRules {
		if rule.Permission != "" && !auth.IsKnownPermission(rule.Permission) {
			t.Errorf("rule for %s requires unknown permission %q — add it to auth.Catalog", proc, rule.Permission)
		}
	}
}
