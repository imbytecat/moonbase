package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/integrationkit/integration"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// integrationOps is the one implementation behind every profile-based
// integration's Create/Update/Delete/Bind RPCs. Each integration supplies its
// catalog, schema-aware secret merge, and optional validation; the lifecycle
// rules are shared: create assigns the id, update keeps stored secrets when the
// wire value is empty, delete refuses while bound, bind validates the purpose
// catalog.
type integrationOps[P settings.Profile[P]] struct {
	name        string
	load        func(context.Context) (settings.Integration[P], error)
	save        func(context.Context, settings.Integration[P]) error
	purposes    integration.Catalog
	keepSecrets func(updated, stored P) P
	validate    func(P) error
}

func (o integrationOps[P]) errNotFound() error {
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s profile not found", o.name))
}

func (o integrationOps[P]) create(ctx context.Context, s *SystemService, in P) (P, error) {
	cfg, err := o.load(ctx)
	if err != nil {
		var zero P
		return zero, s.internal(ctx, "load "+o.name+" settings", err)
	}
	profile := in.WithID(uuid.NewString())
	if o.validate != nil {
		if err := o.validate(profile); err != nil {
			var zero P
			return zero, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	cfg.Profiles = append(cfg.Profiles, profile)
	if err := o.save(ctx, cfg); err != nil {
		var zero P
		return zero, s.internal(ctx, "save "+o.name+" settings", err)
	}
	return profile, nil
}

func (o integrationOps[P]) update(ctx context.Context, s *SystemService, in P) (P, error) {
	cfg, err := o.load(ctx)
	if err != nil {
		var zero P
		return zero, s.internal(ctx, "load "+o.name+" settings", err)
	}
	for i, p := range cfg.Profiles {
		if p.ProfileID() != in.ProfileID() {
			continue
		}
		updated := o.keepSecrets(in, p).WithID(p.ProfileID())
		if o.validate != nil {
			if err := o.validate(updated); err != nil {
				var zero P
				return zero, connect.NewError(connect.CodeInvalidArgument, err)
			}
		}
		cfg.Profiles[i] = updated
		if err := o.save(ctx, cfg); err != nil {
			var zero P
			return zero, s.internal(ctx, "save "+o.name+" settings", err)
		}
		return updated, nil
	}
	var zero P
	return zero, o.errNotFound()
}

func (o integrationOps[P]) delete(ctx context.Context, s *SystemService, id string) error {
	cfg, err := o.load(ctx)
	if err != nil {
		return s.internal(ctx, "load "+o.name+" settings", err)
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
		return s.internal(ctx, "save "+o.name+" settings", err)
	}
	return nil
}

// bindMany is the one binding write: every id must resolve, an empty list
// unbinds. Single-valued integrations pass ≤1 id; multi-valued ones (oauth
// login) pass the full ordered selection.
func (o integrationOps[P]) bindMany(ctx context.Context, s *SystemService, purpose string, profileIDs []string) (settings.Integration[P], error) {
	var zero settings.Integration[P]
	if !o.purposes.Known(purpose) {
		return zero, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown %s purpose %q", o.name, purpose))
	}
	cfg, err := o.load(ctx)
	if err != nil {
		return zero, s.internal(ctx, "load "+o.name+" settings", err)
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
		return zero, s.internal(ctx, "save "+o.name+" settings", err)
	}
	return cfg, nil
}

func (o integrationOps[P]) bind(ctx context.Context, s *SystemService, purpose, profileID string) (settings.Integration[P], error) {
	ids := []string{}
	if profileID != "" {
		ids = []string{profileID}
	}
	return o.bindMany(ctx, s, purpose, ids)
}

// resolveTestProfile implements the shared test-RPC convention: pass a
// profile to test unsaved form values (empty secrets fall back to the stored
// profile with the same id), or a profile id to test a stored profile as-is.
func (o integrationOps[P]) resolveTestProfile(ctx context.Context, s *SystemService, in *P, id string) (P, error) {
	var zero P
	cfg, err := o.load(ctx)
	if err != nil {
		return zero, s.internal(ctx, "load "+o.name+" settings", err)
	}
	switch {
	case in != nil:
		profile := *in
		if stored, ok := cfg.Profile(profile.ProfileID()); ok {
			profile = o.keepSecrets(profile, stored).WithID(stored.ProfileID())
		}
		return profile, nil
	case id != "":
		stored, ok := cfg.Profile(id)
		if !ok {
			return zero, o.errNotFound()
		}
		return stored, nil
	default:
		return zero, connect.NewError(connect.CodeInvalidArgument,
			errors.New("provide a profile or profile_id to test"))
	}
}

// firstID renders a single-valued binding for the wire (empty = unbound).
func firstID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}
