package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

var ErrNotConfigured = errors.New("ai model is not configured")

type Prompt struct {
	System string
	User   string
}
type Descriptor struct {
	Key          string
	Presentation integration.Presentation
	JSONSchema   map[string]any
	UISchema     map[string]any
}
type contractOps struct {
	create func(map[string]any, map[string]string) (map[string]any, error)
	update func(map[string]any, map[string]string, map[string]any) (map[string]any, error)
	view   func(map[string]any) (config.View, bool)
}
type registration struct {
	descriptor    Descriptor
	contract      contractOps
	complete      func(context.Context, map[string]any, Prompt) (string, error)
	definitionErr error
}
type Registration struct{ entry registration }

func Register[T any](key string, p integration.Presentation, c config.Contract[T], complete func(context.Context, T, Prompt) (string, error)) Registration {
	definitionErr := c.ValidateDefinition()
	if complete == nil {
		definitionErr = errors.New("provider operation is missing")
	}
	return Registration{entry: registration{descriptor: Descriptor{Key: key, Presentation: p, JSONSchema: c.JSONSchema(), UISchema: c.UISchema()}, contract: contractOps{create: c.CreateWrite, update: c.UpdateWrite, view: c.View}, complete: func(ctx context.Context, v map[string]any, p Prompt) (string, error) {
		typed, err := c.Decode(v)
		if err != nil {
			return "", fmt.Errorf("decode llm config: %w", err)
		}
		return complete(ctx, typed, p)
	}, definitionErr: definitionErr}}
}

type Registry struct {
	entries []registration
	byKey   map[string]int
}

var iconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry(items ...Registration) (Registry, error) {
	r := Registry{entries: make([]registration, 0, len(items)), byKey: make(map[string]int, len(items))}
	for _, item := range items {
		e := item.entry
		if e.descriptor.Key == "" {
			return Registry{}, errors.New("provider key 不能为空")
		}
		if e.descriptor.Presentation.Name == "" {
			return Registry{}, fmt.Errorf("provider %q 缺少 presentation", e.descriptor.Key)
		}
		if ref := e.descriptor.Presentation.IconRef; ref != "" && !iconRefPattern.MatchString(ref) {
			return Registry{}, fmt.Errorf("provider %q 的 icon_ref 无效", e.descriptor.Key)
		}
		if e.definitionErr != nil {
			return Registry{}, fmt.Errorf("provider %q 定义无效: %w", e.descriptor.Key, e.definitionErr)
		}
		if _, ok := r.byKey[e.descriptor.Key]; ok {
			return Registry{}, fmt.Errorf("provider key %q 重复", e.descriptor.Key)
		}
		r.byKey[e.descriptor.Key] = len(r.entries)
		r.entries = append(r.entries, e)
	}
	return r, nil
}
func MustRegistry(items ...Registration) Registry {
	r, err := NewRegistry(items...)
	if err != nil {
		panic(err)
	}
	return r
}
func (r Registry) entry(k string) (registration, bool) {
	i, ok := r.byKey[k]
	if !ok {
		return registration{}, false
	}
	return r.entries[i], true
}
func (r Registry) Providers() []string {
	out := make([]string, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.descriptor.Key
	}
	return out
}
func (r Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.descriptor
		out[i].JSONSchema = clone(e.descriptor.JSONSchema)
		out[i].UISchema = clone(e.descriptor.UISchema)
	}
	return out
}
func (r Registry) CreateConfig(p string, v map[string]any, s map[string]string) (map[string]any, error) {
	e, ok := r.entry(p)
	if !ok {
		return nil, fmt.Errorf("未知 provider %q", p)
	}
	return e.contract.create(v, s)
}
func (r Registry) UpdateConfig(p string, v map[string]any, s map[string]string, old map[string]any) (map[string]any, error) {
	e, ok := r.entry(p)
	if !ok {
		return nil, fmt.Errorf("未知 provider %q", p)
	}
	return e.contract.update(v, s, old)
}
func (r Registry) ViewConfig(p string, v map[string]any) (config.View, bool) {
	e, ok := r.entry(p)
	if !ok {
		return config.View{Values: map[string]any{}}, false
	}
	return e.contract.view(v)
}
func (r Registry) ConfigUsable(provider string, values map[string]any) bool {
	_, ok := r.ViewConfig(provider, values)
	return ok
}
func (r Registry) Complete(ctx context.Context, provider string, values map[string]any, system, user string) (string, error) {
	e, ok := r.entry(provider)
	if !ok {
		return "", ErrNotConfigured
	}
	return e.complete(ctx, values, Prompt{System: system, User: user})
}
func clone(v map[string]any) map[string]any {
	raw, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
