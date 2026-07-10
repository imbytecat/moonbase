// Package integration contains the base-owned purpose catalog and the ordered,
// self-describing provider registry shared by infrastructure integrations.
package integration

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"

	"github.com/imbytecat/moonbase/integrations/core/config"
)

type Cardinality string

const (
	Single   Cardinality = "single"
	Multiple Cardinality = "multiple"
)

type Purpose struct {
	Key         string
	Name        string
	Description string
	Cardinality Cardinality
}

type Catalog []Purpose

func (c Catalog) Known(key string) bool {
	return slices.ContainsFunc(c, func(purpose Purpose) bool { return purpose.Key == key })
}

func (c Catalog) Keys() []string {
	out := make([]string, len(c))
	for i, purpose := range c {
		out[i] = purpose.Key
	}
	return out
}

type Presentation struct {
	Name        string
	Description string
	Color       string
	IconRef     string
}

type Entry[Ops any] struct {
	Key          string
	Presentation Presentation
	Config       config.Schema
	Ops          Ops
}

type Descriptor struct {
	Key          string
	Presentation Presentation
	Config       config.Schema
}

type Registry[Ops any] struct {
	entries []Entry[Ops]
	byKey   map[string]int
}

var iconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry[Ops any](entries []Entry[Ops]) (Registry[Ops], error) {
	registry := Registry[Ops]{entries: slices.Clone(entries), byKey: make(map[string]int, len(entries))}
	for i, entry := range entries {
		if entry.Key == "" {
			return Registry[Ops]{}, fmt.Errorf("provider key 不能为空")
		}
		if _, exists := registry.byKey[entry.Key]; exists {
			return Registry[Ops]{}, fmt.Errorf("provider key %q 重复", entry.Key)
		}
		if entry.Presentation.Name == "" {
			return Registry[Ops]{}, fmt.Errorf("provider %q 缺少 presentation", entry.Key)
		}
		if entry.Presentation.IconRef != "" && !iconRefPattern.MatchString(entry.Presentation.IconRef) {
			return Registry[Ops]{}, fmt.Errorf("provider %q 的 icon_ref 无效", entry.Key)
		}
		if err := entry.Config.ValidateDefinition(); err != nil {
			return Registry[Ops]{}, fmt.Errorf("provider %q config 无效: %w", entry.Key, err)
		}
		if reflect.ValueOf(entry.Ops).IsZero() {
			return Registry[Ops]{}, fmt.Errorf("provider %q 缺少 Ops", entry.Key)
		}
		registry.byKey[entry.Key] = i
	}
	return registry, nil
}

func MustRegistry[Ops any](entries []Entry[Ops]) Registry[Ops] {
	registry, err := NewRegistry(entries)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r Registry[Ops]) Entries() []Entry[Ops] { return slices.Clone(r.entries) }

func (r Registry[Ops]) Names() []string {
	out := make([]string, len(r.entries))
	for i, entry := range r.entries {
		out[i] = entry.Key
	}
	return out
}

func (r Registry[Ops]) Descriptors() []Descriptor {
	out := make([]Descriptor, len(r.entries))
	for i, entry := range r.entries {
		out[i] = Descriptor{Key: entry.Key, Presentation: entry.Presentation, Config: entry.Config}
	}
	return out
}

func (r Registry[Ops]) entry(provider string) (Entry[Ops], bool) {
	index, ok := r.byKey[provider]
	if !ok {
		var zero Entry[Ops]
		return zero, false
	}
	return r.entries[index], true
}

func (r Registry[Ops]) EntryFor(provider string) (Entry[Ops], bool) { return r.entry(provider) }

func (r Registry[Ops]) ConfigFor(provider string) (config.Schema, bool) {
	entry, ok := r.entry(provider)
	return entry.Config, ok
}

func (r Registry[Ops]) Mask(provider string, values map[string]any) (map[string]any, bool) {
	entry, ok := r.entry(provider)
	if !ok {
		return nil, false
	}
	return entry.Config.Mask(values), true
}

func (r Registry[Ops]) Merge(provider string, incoming, stored map[string]any) (map[string]any, bool) {
	entry, ok := r.entry(provider)
	if !ok {
		return nil, false
	}
	return entry.Config.Merge(incoming, stored), true
}

func (r Registry[Ops]) Validate(provider string, values map[string]any) error {
	entry, ok := r.entry(provider)
	if !ok {
		return fmt.Errorf("未知 provider %q", provider)
	}
	return entry.Config.Validate(values)
}

func (r Registry[Ops]) ProfileUsable(provider string, values map[string]any) bool {
	entry, ok := r.entry(provider)
	return ok && entry.Config.Usable(values)
}

func (r Registry[Ops]) OpsFor(provider string, values map[string]any) (Ops, bool) {
	entry, ok := r.entry(provider)
	if !ok || !entry.Config.Usable(values) {
		var zero Ops
		return zero, false
	}
	return entry.Ops, true
}
