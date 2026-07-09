package storage

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

var ErrNotConfigured = errors.New("file storage is not configured")

type Ops struct {
	PresignPut func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, purpose, key, contentType string, expires time.Duration) (string, error)
	ResolveURL func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, purpose, key string, expires time.Duration) (string, error)
	Delete     func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, purpose, key string) error
	Test       func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile) error
}

type Driver struct {
	Schema schema.Schema
	Ops    Ops
}

var drivers = map[string]Driver{
	"s3": {
		Schema: s3Schema,
		Ops: Ops{
			PresignPut: s3PresignPut,
			ResolveURL: s3ResolveURL,
			Delete:     s3Delete,
			Test:       s3Test,
		},
	},
	"local": {
		Schema: localSchema,
		Ops: Ops{
			PresignPut: localPresignPut,
			ResolveURL: localResolveURL,
			Delete:     localDelete,
			Test:       localTest,
		},
	},
}

func Schemas() map[string]schema.Schema {
	out := make(map[string]schema.Schema, len(drivers))
	for name, d := range drivers {
		out[name] = d.Schema
	}
	return out
}

func Providers() []string {
	return slices.Sorted(maps.Keys(drivers))
}

func DriverFor(provider string) (Driver, bool) {
	d, ok := drivers[provider]
	return d, ok
}

func ProfileUsable(p kitsettings.GenericProfile) bool {
	d, ok := drivers[p.Provider]
	return ok && d.Schema.Usable(p.Config)
}
