// Package schema is the runtime contract between base and an integration
// driver's connection config. A driver publishes a Schema — an ordered list of
// field descriptors (key, type, and the structural flags secret / immutable /
// required) — and base runs one generic engine (Mask, Merge, Validate, Usable)
// over it. Config *values* travel as an opaque map (the wire carries a
// google.protobuf.Struct); only the Schema is structured, and only the driver
// understands what a value means.
//
// The point is the write-only-secret rule: masking lives in exactly one
// audited place (Mask, keyed on Field.Secret), so no driver reimplements it and
// a single driver bug cannot leak a credential. The descriptor is modelled on
// Terraform's provider schema (Sensitive / ForceNew / Required) so the
// base<->driver seam already has the shape a go-plugin protocol would need.
// See ADR-0006.
package schema

import (
	"fmt"
	"maps"
	"slices"
)

// Type is a field's value kind — the closed set the config-form renderer and
// the validator both switch on.
type Type string

const (
	String Type = "string"
	Text   Type = "text" // multi-line string
	Int    Type = "int"
	Bool   Type = "bool"
	Enum   Type = "enum" // one of Field.Options
)

// Field describes one config value. The structural flags (Secret / Immutable /
// Required) drive base's generic engine; the presentation fields (Label / Type
// / Options / Help) drive the form renderer. Key is the value's key in the
// opaque config map — the one name both ends agree on.
type Field struct {
	Key       string
	Label     string
	Type      Type
	Secret    bool     // masked on read; kept from stored on an empty update
	Immutable bool     // always kept from stored on update (e.g. a stable key)
	Required  bool     // must be present for the profile to be Usable
	Options   []string // the allowed values when Type is Enum
	Help      string
	MaxLen    int // 0 = unbounded
}

// Schema is a driver's whole config contract, in display order.
type Schema struct {
	Fields []Field
}

// setSuffix names the companion "<key>_set" bool Mask emits so a masked
// response can still tell the UI whether a secret is stored.
const setSuffix = "_set"

// Mask returns a copy of cfg safe to send outward: every Secret field is
// blanked and a companion "<key>_set" bool announces whether a value is
// stored. This is the write-only-secret rule, in one place.
func (s Schema) Mask(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg)+len(s.Fields))
	maps.Copy(out, cfg)
	for _, f := range s.Fields {
		if !f.Secret {
			continue
		}
		out[f.Key+setSuffix] = !isEmpty(out[f.Key])
		out[f.Key] = ""
	}
	return out
}

// Merge folds an incoming update onto the stored config: a Secret field left
// empty falls back to the stored secret (this is how "leave the password blank
// to keep it" works), and an Immutable field is always taken from stored. The
// read-only "<key>_set" companions the client echoed back are dropped so they
// never persist.
func (s Schema) Merge(incoming, stored map[string]any) map[string]any {
	out := make(map[string]any, len(incoming))
	maps.Copy(out, incoming)
	for _, f := range s.Fields {
		switch {
		case f.Immutable:
			if v, ok := stored[f.Key]; ok {
				out[f.Key] = v
			}
		case f.Secret:
			if isEmpty(out[f.Key]) {
				out[f.Key] = stored[f.Key]
			}
		}
		delete(out, f.Key+setSuffix)
	}
	return out
}

// Validate reports the first field that violates its declared constraints —
// Required presence, MaxLen, and Enum membership. It runs on the merged config
// (secrets already resolved); base calls it before every write, and the client
// form only mirrors these rules for UX.
func (s Schema) Validate(cfg map[string]any) error {
	for _, f := range s.Fields {
		v := cfg[f.Key]
		if isEmpty(v) {
			if f.Required {
				return fmt.Errorf("schema: %q is required", f.Key)
			}
			continue
		}
		str, _ := v.(string)
		if f.MaxLen > 0 && len(str) > f.MaxLen {
			return fmt.Errorf("schema: %q exceeds %d characters", f.Key, f.MaxLen)
		}
		if f.Type == Enum && !slices.Contains(f.Options, str) {
			return fmt.Errorf("schema: %q must be one of %v", f.Key, f.Options)
		}
	}
	return nil
}

// Usable reports whether every Required field is present — the cheap gate a
// seam uses to decide a profile is configured enough to attempt an operation.
// A stored config has already passed Validate, so presence is all Usable needs.
func (s Schema) Usable(cfg map[string]any) bool {
	for _, f := range s.Fields {
		if f.Required && isEmpty(cfg[f.Key]) {
			return false
		}
	}
	return true
}

// isEmpty treats absent, nil, and "" as empty. Secret and required checks only
// ever guard string-typed values (credentials, ids), so string emptiness is
// the only case that matters.
func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}
