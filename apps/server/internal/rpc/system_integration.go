package rpc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// integrationOps keeps only the lifecycle shared across every profile-based
// integration: delete refuses while bound, and bind validates the application
// purpose catalog. Typed registries own Create/Update/View/Describe/Test config
// behavior because their Contract and operations are integration-specific. It
// takes a *systemBase (not the whole facade) so it depends on nothing beyond
// the shared settings store and error helper.
type integrationOps[P settings.Profile[P]] struct {
	name     string
	load     func(context.Context) (settings.Integration[P], error)
	save     func(context.Context, settings.Integration[P]) error
	purposes integration.Catalog
}

func (o integrationOps[P]) errNotFound() error {
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s profile not found", o.name))
}

func (o integrationOps[P]) delete(ctx context.Context, base *systemBase, id string) error {
	cfg, err := o.load(ctx)
	if err != nil {
		return base.internal(ctx, "load "+o.name+" settings", err)
	}
	if purpose, bound := cfg.Bound(id); bound {
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("profile is still bound to %q — unbind it first", purpose))
	}
	kept := cfg.Profiles[:0]
	found := false
	for _, p := range cfg.Profiles {
		if p.ProfileID() == id {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return o.errNotFound()
	}
	cfg.Profiles = kept
	if err := o.save(ctx, cfg); err != nil {
		return base.internal(ctx, "save "+o.name+" settings", err)
	}
	return nil
}

// bindMany is the one binding write: every id must resolve, an empty list
// unbinds. Single-valued integrations pass ≤1 id; multi-valued ones (oauth
// login) pass the full ordered selection.
func (o integrationOps[P]) bindMany(
	ctx context.Context,
	base *systemBase,
	purpose string,
	profileIDs []string,
) (settings.Integration[P], error) {
	var zero settings.Integration[P]
	if !o.purposes.Known(purpose) {
		return zero, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown %s purpose %q", o.name, purpose))
	}
	cfg, err := o.load(ctx)
	if err != nil {
		return zero, base.internal(ctx, "load "+o.name+" settings", err)
	}
	for _, id := range profileIDs {
		if _, ok := cfg.Profile(id); !ok {
			return zero, o.errNotFound()
		}
	}
	if len(profileIDs) > 0 {
		cfg.Bindings[purpose] = profileIDs
	} else {
		delete(cfg.Bindings, purpose)
	}
	if err := o.save(ctx, cfg); err != nil {
		return zero, base.internal(ctx, "save "+o.name+" settings", err)
	}
	return cfg, nil
}

func (o integrationOps[P]) bind(
	ctx context.Context,
	base *systemBase,
	purpose, profileID string,
) (settings.Integration[P], error) {
	ids := []string{}
	if profileID != "" {
		ids = []string{profileID}
	}
	return o.bindMany(ctx, base, purpose, ids)
}

// firstID renders a single-valued binding for the wire (empty = unbound).
func firstID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}
