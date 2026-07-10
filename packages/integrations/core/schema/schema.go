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
	"regexp"
	"slices"
)

// Type is a field's value kind — the closed set the config-form renderer and
// the validator both switch on.
type Type string

const (
	String  Type = "string"
	Text    Type = "text" // multi-line string
	Int     Type = "int"
	Bool    Type = "bool"
	Enum    Type = "enum"         // one of Field.Options
	Strings Type = "string_array" // zero or more values from Field.Options when set
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
	Options   []Option // the allowed choices when Type is Enum or Strings
	Help      string
	MaxLen    int // 0 = unbounded
	Pattern   string
	Min       int
	Max       int
	Unique    bool
	// When set, the field applies only if the config value of ShowWhen.Field is
	// one of ShowWhen.Values. Inactive fields are skipped by Validate/Usable and
	// hidden by the form.
	ShowWhen *ShowWhen
}

// ShowWhen makes a Field conditional on another field's value.
type ShowWhen struct {
	Field  string
	Values []string
}

// Option is one allowed choice of an Enum or Strings field: the stored Value
// plus the driver-written Label and Description the form renderer shows.
type Option struct {
	Value       string
	Label       string
	Description string
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
	out := make(map[string]any, len(s.Fields)*2)
	for _, f := range s.Fields {
		out[f.Key] = cfg[f.Key]
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
	out := make(map[string]any, len(s.Fields))
	for _, f := range s.Fields {
		out[f.Key] = incoming[f.Key]
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
	}
	return out
}

// Validate reports the first field that violates its declared constraints —
// Required presence, MaxLen, and Enum membership. It runs on the merged config
// (secrets already resolved); base calls it before every write, and the client
// form only mirrors these rules for UX.
func (s Schema) Validate(cfg map[string]any) error {
	known := make(map[string]Field, len(s.Fields))
	for _, f := range s.Fields {
		known[f.Key] = f
	}
	for key := range cfg {
		if _, ok := known[key]; ok {
			continue
		}
		if slices.ContainsFunc(s.Fields, func(f Field) bool { return key == f.Key+setSuffix }) {
			return fmt.Errorf("配置字段 %q 创建后不可修改", key)
		}
		return fmt.Errorf("未知配置字段 %q", key)
	}
	for _, f := range s.Fields {
		if !s.fieldActive(f, cfg) {
			continue
		}
		v := cfg[f.Key]
		if isEmpty(v) {
			if f.Required {
				return fmt.Errorf("配置字段 %q 为必填项", f.Key)
			}
			continue
		}
		if err := validateValue(f, v); err != nil {
			return err
		}
	}
	return nil
}

// fieldActive reports whether a field applies given the current config: fields
// with no ShowWhen always apply; conditional fields apply only when the
// referenced field's value is one of the allowed values.
func (s Schema) fieldActive(f Field, cfg map[string]any) bool {
	if f.ShowWhen == nil {
		return true
	}
	current, _ := cfg[f.ShowWhen.Field].(string)
	return slices.Contains(f.ShowWhen.Values, current)
}

func validateValue(f Field, v any) error {
	switch f.Type {
	case String, Text, Enum:
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("配置字段 %q 必须是字符串", f.Key)
		}
		return validateString(f, str)
	case Int:
		n, ok := number(v)
		if !ok {
			return fmt.Errorf("配置字段 %q 必须是整数", f.Key)
		}
		if f.Min != 0 && n < f.Min || f.Max != 0 && n > f.Max {
			return fmt.Errorf("配置字段 %q 必须在 %d 到 %d 之间", f.Key, f.Min, f.Max)
		}
	case Bool:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("配置字段 %q 必须是布尔值", f.Key)
		}
	case Strings:
		values, ok := stringSlice(v)
		if !ok {
			return fmt.Errorf("配置字段 %q 必须是字符串数组", f.Key)
		}
		if f.Unique && duplicates(values) {
			return fmt.Errorf("配置字段 %q 不能包含重复值", f.Key)
		}
		for _, str := range values {
			if err := validateString(f, str); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("配置字段 %q 使用了未知类型 %q", f.Key, f.Type)
	}
	return nil
}

func validateString(f Field, str string) error {
	if f.MaxLen > 0 && len(str) > f.MaxLen {
		return fmt.Errorf("配置字段 %q 不能超过 %d 个字符", f.Key, f.MaxLen)
	}
	if f.Pattern != "" {
		re, err := regexp.Compile(f.Pattern)
		if err != nil {
			return fmt.Errorf("配置字段 %q 的格式规则无效", f.Key)
		}
		if !re.MatchString(str) {
			return fmt.Errorf("配置字段 %q 格式不正确", f.Key)
		}
	}
	if len(f.Options) > 0 && !slices.ContainsFunc(f.Options, func(o Option) bool { return o.Value == str }) {
		return fmt.Errorf("配置字段 %q 必须是所列选项之一", f.Key)
	}
	return nil
}

func stringSlice(v any) ([]string, bool) {
	switch typed := v.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func number(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}

func duplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

// Usable reports whether every Required field is present — the cheap gate a
// seam uses to decide a profile is configured enough to attempt an operation.
// A stored config has already passed Validate, so presence is all Usable needs.
func (s Schema) Usable(cfg map[string]any) bool {
	for _, f := range s.Fields {
		if f.Required && s.fieldActive(f, cfg) && isEmpty(cfg[f.Key]) {
			return false
		}
	}
	return true
}

// isEmpty treats absent, nil, "", and empty string arrays as empty.
func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	if ok {
		return s == ""
	}
	if values, ok := stringSlice(v); ok {
		return len(values) == 0
	}
	return false
}
