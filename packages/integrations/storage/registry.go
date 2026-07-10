package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

var ErrNotConfigured = errors.New("file storage is not configured")

type Visibility int

const (
	VisibilityPrivate Visibility = iota
	VisibilityPublic
)

type Runtime interface {
	LocalSignedURL(
		ctx context.Context,
		method, purpose, key string,
		expires time.Duration,
	) (string, error)
	VisibilityOf(purpose string) Visibility
}

type Operations[T any] struct {
	PresignPut func(Runtime, context.Context, T, string, string, string, time.Duration) (string, error)
	ResolveURL func(Runtime, context.Context, T, string, string, time.Duration) (string, error)
	Delete     func(Runtime, context.Context, T, string, string) error
	Test       func(context.Context, Runtime, T) error
	ObjectPath func(T, string) (string, error)
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
	presignPut func(Runtime, context.Context, map[string]any, string, string, string, time.Duration) (string, error)
	resolveURL func(Runtime, context.Context, map[string]any, string, string, time.Duration) (string, error)
	delete     func(Runtime, context.Context, map[string]any, string, string) error
	test       func(context.Context, Runtime, map[string]any) error
	objectPath func(map[string]any, string) (string, error)
}

type registration struct {
	descriptor    Descriptor
	contract      contractOps
	ops           operationSet
	definitionErr error
}

type Registration struct{ entry registration }

func Register[T any](
	key string,
	presentation integration.Presentation,
	contract config.Contract[T],
	ops Operations[T],
) Registration {
	definitionErr := contract.ValidateDefinition()
	if ops.PresignPut == nil || ops.ResolveURL == nil || ops.Delete == nil || ops.Test == nil {
		definitionErr = errors.New("provider operation is missing")
	}
	decode := func(values map[string]any) (T, error) {
		typed, err := contract.Decode(values)
		if err != nil {
			var zero T
			return zero, fmt.Errorf("decode storage config: %w", err)
		}
		return typed, nil
	}
	entry := registration{
		descriptor: Descriptor{
			Key:          key,
			Presentation: presentation,
			JSONSchema:   contract.JSONSchema(),
			UISchema:     contract.UISchema(),
		},
		contract: contractOps{
			create: contract.CreateWrite,
			update: contract.UpdateWrite,
			view:   contract.View,
		},
		definitionErr: definitionErr,
	}
	entry.ops.presignPut = func(rt Runtime, ctx context.Context, values map[string]any, purpose, objectKey, contentType string, expires time.Duration) (string, error) {
		typed, err := decode(values)
		if err != nil {
			return "", err
		}
		return ops.PresignPut(rt, ctx, typed, purpose, objectKey, contentType, expires)
	}
	entry.ops.resolveURL = func(rt Runtime, ctx context.Context, values map[string]any, purpose, objectKey string, expires time.Duration) (string, error) {
		typed, err := decode(values)
		if err != nil {
			return "", err
		}
		return ops.ResolveURL(rt, ctx, typed, purpose, objectKey, expires)
	}
	entry.ops.delete = func(rt Runtime, ctx context.Context, values map[string]any, purpose, objectKey string) error {
		typed, err := decode(values)
		if err != nil {
			return err
		}
		return ops.Delete(rt, ctx, typed, purpose, objectKey)
	}
	entry.ops.test = func(ctx context.Context, rt Runtime, values map[string]any) error {
		typed, err := decode(values)
		if err != nil {
			return err
		}
		return ops.Test(ctx, rt, typed)
	}
	if ops.ObjectPath != nil {
		entry.ops.objectPath = func(values map[string]any, objectKey string) (string, error) {
			typed, err := decode(values)
			if err != nil {
				return "", err
			}
			return ops.ObjectPath(typed, objectKey)
		}
	}
	return Registration{entry: entry}
}

type Registry struct {
	entries []registration
	byKey   map[string]int
}

var iconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry(registrations ...Registration) (Registry, error) {
	r := Registry{
		entries: make([]registration, 0, len(registrations)),
		byKey:   make(map[string]int, len(registrations)),
	}
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
		if _, exists := r.byKey[e.descriptor.Key]; exists {
			return Registry{}, fmt.Errorf("provider key %q 重复", e.descriptor.Key)
		}
		r.byKey[e.descriptor.Key] = len(r.entries)
		r.entries = append(r.entries, e)
	}
	return r, nil
}

func MustRegistry(registrations ...Registration) Registry {
	r, err := NewRegistry(registrations...)
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
		out[i].JSONSchema = cloneSchema(e.descriptor.JSONSchema)
		out[i].UISchema = cloneSchema(e.descriptor.UISchema)
	}
	return out
}
func (r Registry) ConfigUsable(provider string, values map[string]any) bool {
	_, valid := r.ViewConfig(provider, values)
	return valid
}

func (r Registry) CreateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
) (map[string]any, error) {
	e, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("未知 provider %q", provider)
	}
	return e.contract.create(values, secrets)
}

func (r Registry) UpdateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
	stored map[string]any,
) (map[string]any, error) {
	e, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("未知 provider %q", provider)
	}
	return e.contract.update(values, secrets, stored)
}
func (r Registry) ViewConfig(provider string, stored map[string]any) (config.View, bool) {
	e, ok := r.entry(provider)
	if !ok {
		return config.View{Values: map[string]any{}}, false
	}
	return e.contract.view(stored)
}

func (r Registry) PresignPut(
	rt Runtime,
	ctx context.Context,
	provider string,
	values map[string]any,
	purpose, key, contentType string,
	expires time.Duration,
) (string, error) {
	e, ok := r.entry(provider)
	if !ok {
		return "", ErrNotConfigured
	}
	return e.ops.presignPut(rt, ctx, values, purpose, key, contentType, expires)
}

func (r Registry) ResolveURL(
	rt Runtime,
	ctx context.Context,
	provider string,
	values map[string]any,
	purpose, key string,
	expires time.Duration,
) (string, error) {
	e, ok := r.entry(provider)
	if !ok {
		return "", ErrNotConfigured
	}
	return e.ops.resolveURL(rt, ctx, values, purpose, key, expires)
}

func (r Registry) Delete(
	rt Runtime,
	ctx context.Context,
	provider string,
	values map[string]any,
	purpose, key string,
) error {
	e, ok := r.entry(provider)
	if !ok {
		return ErrNotConfigured
	}
	return e.ops.delete(rt, ctx, values, purpose, key)
}

func (r Registry) Test(
	ctx context.Context,
	rt Runtime,
	provider string,
	values map[string]any,
) error {
	e, ok := r.entry(provider)
	if !ok {
		return ErrNotConfigured
	}
	return e.ops.test(ctx, rt, values)
}
func (r Registry) ObjectPath(provider string, values map[string]any, key string) (string, error) {
	e, ok := r.entry(provider)
	if !ok || e.ops.objectPath == nil {
		return "", ErrNotConfigured
	}
	return e.ops.objectPath(values, key)
}
func cloneSchema(schema map[string]any) map[string]any {
	raw, _ := json.Marshal(schema)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
