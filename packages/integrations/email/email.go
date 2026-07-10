// Package email defines the provider execution model for sending email with
// an already selected profile. Application purposes and settings resolution
// belong to the consuming application facade.
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

var ErrNotConfigured = errors.New("email is not configured")

type Message struct {
	To       string
	Subject  string
	TextBody string
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
	send          func(context.Context, map[string]any, Message) error
	definitionErr error
}

// Registration atomically binds one provider's descriptor, config contract,
// and typed send operation. Its contents cannot be rearranged by consumers.
type Registration struct{ entry registration }

func Register[T any](
	key string,
	presentation integration.Presentation,
	contract config.Contract[T],
	send func(context.Context, T, Message) error,
) Registration {
	definitionErr := contract.ValidateDefinition()
	if send == nil {
		definitionErr = errors.New("provider operation is missing")
	}
	return Registration{entry: registration{
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
		send: func(ctx context.Context, values map[string]any, message Message) error {
			typed, err := contract.Decode(values)
			if err != nil {
				return fmt.Errorf("decode email config: %w", err)
			}
			return send(ctx, typed, message)
		},
		definitionErr: definitionErr,
	}}
}

type Registry struct {
	entries []registration
	byKey   map[string]int
}

var iconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry(registrations ...Registration) (Registry, error) {
	registry := Registry{
		entries: make([]registration, 0, len(registrations)),
		byKey:   make(map[string]int, len(registrations)),
	}
	for _, item := range registrations {
		entry := item.entry
		if entry.descriptor.Key == "" {
			return Registry{}, errors.New("provider key 不能为空")
		}
		if entry.descriptor.Presentation.Name == "" {
			return Registry{}, fmt.Errorf("provider %q 缺少 presentation", entry.descriptor.Key)
		}
		if iconRef := entry.descriptor.Presentation.IconRef; iconRef != "" &&
			!iconRefPattern.MatchString(iconRef) {
			return Registry{}, fmt.Errorf("provider %q 的 icon_ref 无效", entry.descriptor.Key)
		}
		if entry.definitionErr != nil {
			return Registry{}, fmt.Errorf(
				"provider %q 定义无效: %w",
				entry.descriptor.Key,
				entry.definitionErr,
			)
		}
		if _, exists := registry.byKey[entry.descriptor.Key]; exists {
			return Registry{}, fmt.Errorf("provider key %q 重复", entry.descriptor.Key)
		}
		registry.byKey[entry.descriptor.Key] = len(registry.entries)
		registry.entries = append(registry.entries, entry)
	}
	return registry, nil
}

func MustRegistry(registrations ...Registration) Registry {
	registry, err := NewRegistry(registrations...)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, len(r.entries))
	for i, entry := range r.entries {
		out[i] = entry.descriptor
		out[i].JSONSchema = cloneSchema(entry.descriptor.JSONSchema)
		out[i].UISchema = cloneSchema(entry.descriptor.UISchema)
	}
	return out
}

func (r Registry) Providers() []string {
	out := make([]string, len(r.entries))
	for i, entry := range r.entries {
		out[i] = entry.descriptor.Key
	}
	return out
}

func (r Registry) Send(
	ctx context.Context,
	provider string,
	values map[string]any,
	message Message,
) error {
	entry, ok := r.entry(provider)
	if !ok {
		return ErrNotConfigured
	}
	if err := entry.send(ctx, values, message); err != nil {
		return err
	}
	return nil
}

func (r Registry) CreateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
) (map[string]any, error) {
	entry, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("未知 provider %q", provider)
	}
	return entry.contract.create(values, secrets)
}

func (r Registry) UpdateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
	stored map[string]any,
) (map[string]any, error) {
	entry, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("未知 provider %q", provider)
	}
	return entry.contract.update(values, secrets, stored)
}

func (r Registry) ViewConfig(provider string, stored map[string]any) (config.View, bool) {
	entry, ok := r.entry(provider)
	if !ok {
		return config.View{Values: map[string]any{}}, false
	}
	return entry.contract.view(stored)
}

func (r Registry) ConfigUsable(provider string, values map[string]any) bool {
	_, valid := r.ViewConfig(provider, values)
	return valid
}

func (r Registry) entry(key string) (registration, bool) {
	index, ok := r.byKey[key]
	if !ok {
		return registration{}, false
	}
	return r.entries[index], true
}

func cloneSchema(schema map[string]any) map[string]any {
	raw, _ := json.Marshal(schema)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
