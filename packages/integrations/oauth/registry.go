package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

type Descriptor struct {
	Key          string
	Presentation integration.Presentation
	JSONSchema   map[string]any
	UISchema     map[string]any
}

type Operations[T any] struct {
	AuthorizeURL func(context.Context, T, string, string) (string, FlowSecrets, error)
	Exchange     func(context.Context, T, string, string, FlowSecrets) (ExternalIdentity, error)
}

type registration struct {
	descriptor Descriptor
	create     func(map[string]any, map[string]string) (map[string]any, error)
	update     func(map[string]any, map[string]string, map[string]any) (map[string]any, error)
	view       func(map[string]any) (config.View, bool)
	authorize  func(context.Context, map[string]any, string, string) (string, FlowSecrets, error)
	exchange   func(context.Context, map[string]any, string, string, FlowSecrets) (ExternalIdentity, error)
	err        error
}

type Registration struct{ entry registration }

func Register[T any](
	key string,
	presentation integration.Presentation,
	contract config.Contract[T],
	operations Operations[T],
) Registration {
	definitionErr := contract.ValidateDefinition()
	if operations.AuthorizeURL == nil || operations.Exchange == nil {
		definitionErr = errors.New("provider operation is missing")
	}
	return Registration{entry: registration{
		descriptor: Descriptor{
			Key:          key,
			Presentation: presentation,
			JSONSchema:   contract.JSONSchema(),
			UISchema:     contract.UISchema(),
		},
		create: contract.CreateWrite,
		update: contract.UpdateWrite,
		view:   contract.View,
		authorize: func(ctx context.Context, values map[string]any, redirectURI, state string) (string, FlowSecrets, error) {
			typed, err := contract.Decode(values)
			if err != nil {
				return "", FlowSecrets{}, fmt.Errorf("decode oauth config: %w", err)
			}
			return operations.AuthorizeURL(ctx, typed, redirectURI, state)
		},
		exchange: func(ctx context.Context, values map[string]any, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error) {
			typed, err := contract.Decode(values)
			if err != nil {
				return ExternalIdentity{}, fmt.Errorf("decode oauth config: %w", err)
			}
			return operations.Exchange(ctx, typed, code, redirectURI, secrets)
		},
		err: definitionErr,
	}}
}

type Registry struct {
	entries []registration
	byKey   map[string]int
}

var oauthIconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry(registrations ...Registration) (Registry, error) {
	r := Registry{
		entries: make([]registration, 0, len(registrations)),
		byKey:   make(map[string]int, len(registrations)),
	}
	for _, item := range registrations {
		e := item.entry
		if e.descriptor.Key == "" || e.descriptor.Presentation.Name == "" || e.err != nil {
			return Registry{}, errors.New("oauth provider registration is invalid")
		}
		if icon := e.descriptor.Presentation.IconRef; icon != "" &&
			!oauthIconRefPattern.MatchString(icon) {
			return Registry{}, fmt.Errorf("provider %q has invalid icon_ref", e.descriptor.Key)
		}
		if _, exists := r.byKey[e.descriptor.Key]; exists {
			return Registry{}, fmt.Errorf("provider key %q is duplicated", e.descriptor.Key)
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

func (r Registry) entry(provider string) (registration, bool) {
	i, ok := r.byKey[provider]
	if !ok {
		return registration{}, false
	}
	return r.entries[i], true
}

func (r Registry) AuthorizeURL(
	ctx context.Context,
	provider string,
	values map[string]any,
	redirectURI, state string,
) (string, FlowSecrets, error) {
	e, ok := r.entry(provider)
	if !ok {
		return "", FlowSecrets{}, ErrNotConfigured
	}
	return e.authorize(ctx, values, redirectURI, state)
}

func (r Registry) Exchange(
	ctx context.Context,
	provider string,
	values map[string]any,
	code, redirectURI string,
	secrets FlowSecrets,
) (ExternalIdentity, error) {
	e, ok := r.entry(provider)
	if !ok {
		return ExternalIdentity{}, ErrNotConfigured
	}
	return e.exchange(ctx, values, code, redirectURI, secrets)
}

func (r Registry) CreateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
) (map[string]any, error) {
	e, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("unknown oauth provider %q", provider)
	}
	return e.create(values, secrets)
}

func (r Registry) UpdateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
	stored map[string]any,
) (map[string]any, error) {
	e, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("unknown oauth provider %q", provider)
	}
	return e.update(values, secrets, stored)
}
func (r Registry) ViewConfig(provider string, stored map[string]any) (config.View, bool) {
	e, ok := r.entry(provider)
	if !ok {
		return config.View{Values: map[string]any{}}, false
	}
	return e.view(stored)
}
func (r Registry) ConfigUsable(provider string, values map[string]any) bool {
	_, valid := r.ViewConfig(provider, values)
	return valid
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
		out[i].JSONSchema = cloneOAuthMap(e.descriptor.JSONSchema)
		out[i].UISchema = cloneOAuthMap(e.descriptor.UISchema)
	}
	return out
}
func cloneOAuthMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
