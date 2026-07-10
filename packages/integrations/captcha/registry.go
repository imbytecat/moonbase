package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

var ErrNotConfigured = errors.New("captcha is not configured")

type Operations[T any] struct {
	SiteKey   func(T) string
	Verify    func(context.Context, T, string, string) error
	Challenge func(context.Context, T) (any, error)
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
type operationSet struct {
	siteKey   func(map[string]any) (string, error)
	verify    func(context.Context, map[string]any, string, string) error
	challenge func(context.Context, map[string]any) (any, error)
}
type registration struct {
	descriptor    Descriptor
	contract      contractOps
	ops           operationSet
	definitionErr error
}
type Registration struct{ entry registration }

func Register[T any](key string, presentation integration.Presentation, contract config.Contract[T], ops Operations[T]) Registration {
	definitionErr := contract.ValidateDefinition()
	if ops.SiteKey == nil || ops.Verify == nil {
		definitionErr = errors.New("provider operation is missing")
	}
	decode := func(values map[string]any) (T, error) {
		typed, err := contract.Decode(values)
		if err != nil {
			var zero T
			return zero, fmt.Errorf("decode captcha config: %w", err)
		}
		return typed, nil
	}
	e := registration{descriptor: Descriptor{Key: key, Presentation: presentation, JSONSchema: contract.JSONSchema(), UISchema: contract.UISchema()}, contract: contractOps{create: contract.CreateWrite, update: contract.UpdateWrite, view: contract.View}, definitionErr: definitionErr}
	e.ops.siteKey = func(values map[string]any) (string, error) {
		typed, err := decode(values)
		if err != nil {
			return "", err
		}
		return ops.SiteKey(typed), nil
	}
	e.ops.verify = func(ctx context.Context, values map[string]any, token, ip string) error {
		typed, err := decode(values)
		if err != nil {
			return err
		}
		return ops.Verify(ctx, typed, token, ip)
	}
	if ops.Challenge != nil {
		e.ops.challenge = func(ctx context.Context, values map[string]any) (any, error) {
			typed, err := decode(values)
			if err != nil {
				return nil, err
			}
			return ops.Challenge(ctx, typed)
		}
	}
	return Registration{entry: e}
}

type Registry struct {
	entries []registration
	byKey   map[string]int
}

var iconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry(registrations ...Registration) (Registry, error) {
	r := Registry{entries: make([]registration, 0, len(registrations)), byKey: make(map[string]int, len(registrations))}
	for _, item := range registrations {
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
func (r Registry) entry(key string) (registration, bool) {
	i, ok := r.byKey[key]
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
func (r Registry) Widget(provider string, values map[string]any) (string, bool) {
	e, ok := r.entry(provider)
	if !ok {
		return "", false
	}
	key, err := e.ops.siteKey(values)
	return key, err == nil
}
func (r Registry) Verify(ctx context.Context, provider string, values map[string]any, token, ip string) error {
	e, ok := r.entry(provider)
	if !ok {
		return ErrNotConfigured
	}
	return e.ops.verify(ctx, values, token, ip)
}
func (r Registry) Challenge(ctx context.Context, provider string, values map[string]any) (any, error) {
	e, ok := r.entry(provider)
	if !ok || e.ops.challenge == nil {
		return nil, ErrNotConfigured
	}
	return e.ops.challenge(ctx, values)
}
func clone(v map[string]any) map[string]any {
	raw, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
