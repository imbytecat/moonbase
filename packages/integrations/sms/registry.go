package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

var ErrNotConfigured = errors.New("sms is not configured")

type Message struct {
	TemplateCode string
	E164         string
	Content      string
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
				return fmt.Errorf("decode sms config: %w", err)
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

func (r Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.descriptor
		out[i].JSONSchema = cloneSchema(e.descriptor.JSONSchema)
		out[i].UISchema = cloneSchema(e.descriptor.UISchema)
	}
	return out
}

func (r Registry) Providers() []string {
	out := make([]string, len(r.entries))
	for i, e := range r.entries {
		out[i] = e.descriptor.Key
	}
	return out
}

func (r Registry) SendTemplate(
	ctx context.Context,
	provider string,
	values map[string]any,
	templateCode, e164, content string,
) error {
	e, ok := r.entry(provider)
	if !ok {
		return ErrNotConfigured
	}
	return e.send(ctx, values, Message{TemplateCode: templateCode, E164: e164, Content: content})
}

func (r Registry) SendCode(
	ctx context.Context,
	provider string,
	values map[string]any,
	e164, code string,
) error {
	return r.SendTemplate(ctx, provider, values, "", e164, code)
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

func (r Registry) ConfigUsable(provider string, values map[string]any) bool {
	_, valid := r.ViewConfig(provider, values)
	return valid
}

func (r Registry) entry(key string) (registration, bool) {
	i, ok := r.byKey[key]
	if !ok {
		return registration{}, false
	}
	return r.entries[i], true
}

func cloneSchema(schema map[string]any) map[string]any {
	raw, _ := json.Marshal(schema)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
