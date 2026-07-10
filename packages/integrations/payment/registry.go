package pay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

// Registry is the immutable control-plane boundary for typed payment providers.
// It validates and decodes a stored profile before any provider operation runs.
type Registry struct {
	entries []registryEntry
	byKey   map[string]int
}

type registryEntry struct {
	descriptor       Descriptor
	contract         configOps
	valid            func(map[string]any) bool
	plan             func(context.Context, map[string]any, PlanRequest) (PlanResult, error)
	create           func(context.Context, map[string]any, CreateRequest) (Action, error)
	query            func(context.Context, map[string]any, string) (QueryResult, error)
	refund           func(context.Context, map[string]any, RefundRequest) (RefundResult, error)
	queryRefund      func(context.Context, map[string]any, string) (bool, error)
	parseNotify      func(context.Context, map[string]any, *http.Request) (NotifyResult, error)
	recoverAction    func(context.Context, map[string]any, string) (Action, error)
	renderHostedFlow func(string, string) ([]byte, error)
}

// Descriptor is the complete operator-facing description of one payment
// provider. The payment methods/products remain provider-owned data.
type Descriptor struct {
	Key          string
	Presentation integration.Presentation
	ConfigSchema map[string]any
	UISchema     map[string]any
	Payment      ProviderDescriptor
}

type configOps struct {
	create func(map[string]any, map[string]string) (map[string]any, error)
	update func(map[string]any, map[string]string, map[string]any) (map[string]any, error)
	view   func(map[string]any) (config.View, bool)
}

// Registration atomically joins a provider descriptor, its config contract,
// and its typed operation.
type Registration struct {
	key           string
	presentation  integration.Presentation
	definitionErr error
	entry         registryEntry
}

// Operations is one provider's typed payment seam. Optional capabilities are
// represented by nil functions and are derived by the registry.
type Operations[T any] struct {
	Plan             func(context.Context, T, PlanRequest) (PlanResult, error)
	Create           func(context.Context, T, CreateRequest) (Action, error)
	Query            func(context.Context, T, string) (QueryResult, error)
	Refund           func(context.Context, T, RefundRequest) (RefundResult, error)
	QueryRefund      func(context.Context, T, string) (bool, error)
	ParseNotify      func(context.Context, T, *http.Request) (NotifyResult, error)
	RecoverAction    func(context.Context, T, string) (Action, error)
	RenderHostedFlow func(string, string) ([]byte, error)
}

func RegisterOperations[T any](
	key string,
	presentation integration.Presentation,
	contract config.Contract[T],
	payment ProviderDescriptor,
	operations Operations[T],
) Registration {
	registration := Register(key, presentation, contract, payment, operations.Create)
	if operations.Plan == nil || operations.Query == nil {
		registration.definitionErr = errors.New("provider mandatory operation is missing")
		return registration
	}
	registration.entry.valid = func(values map[string]any) bool {
		_, err := contract.Decode(values)
		return err == nil
	}
	registration.entry.query = func(ctx context.Context, values map[string]any, outTradeNo string) (QueryResult, error) {
		typed, err := contract.Decode(values)
		if err != nil {
			return QueryResult{}, fmt.Errorf("decode payment config: %w", err)
		}
		return operations.Query(ctx, typed, outTradeNo)
	}
	registration.entry.plan = func(ctx context.Context, values map[string]any, request PlanRequest) (PlanResult, error) {
		typed, err := contract.Decode(values)
		if err != nil {
			return PlanResult{}, fmt.Errorf("decode payment config: %w", err)
		}
		return operations.Plan(ctx, typed, request)
	}
	if operations.Refund != nil {
		registration.entry.refund = func(ctx context.Context, values map[string]any, request RefundRequest) (RefundResult, error) {
			typed, err := contract.Decode(values)
			if err != nil {
				return RefundResult{}, fmt.Errorf("decode payment config: %w", err)
			}
			return operations.Refund(ctx, typed, request)
		}
	}
	if operations.QueryRefund != nil {
		registration.entry.queryRefund = func(ctx context.Context, values map[string]any, refundNo string) (bool, error) {
			typed, err := contract.Decode(values)
			if err != nil {
				return false, fmt.Errorf("decode payment config: %w", err)
			}
			return operations.QueryRefund(ctx, typed, refundNo)
		}
	}
	if operations.ParseNotify != nil {
		registration.entry.parseNotify = func(ctx context.Context, values map[string]any, request *http.Request) (NotifyResult, error) {
			typed, err := contract.Decode(values)
			if err != nil {
				return NotifyResult{}, fmt.Errorf("decode payment config: %w", err)
			}
			return operations.ParseNotify(ctx, typed, request)
		}
	}
	if operations.RecoverAction != nil {
		registration.entry.recoverAction = func(ctx context.Context, values map[string]any, outTradeNo string) (Action, error) {
			typed, err := contract.Decode(values)
			if err != nil {
				return Action{}, fmt.Errorf("decode payment config: %w", err)
			}
			return operations.RecoverAction(ctx, typed, outTradeNo)
		}
	}
	registration.entry.renderHostedFlow = operations.RenderHostedFlow
	capabilities := []string{}
	if operations.ParseNotify != nil {
		capabilities = append(capabilities, "notify")
	}
	if operations.Refund != nil {
		capabilities = append(capabilities, "refund")
	}
	if operations.QueryRefund != nil {
		capabilities = append(capabilities, "refund_query")
	}
	if operations.RenderHostedFlow != nil {
		capabilities = append(capabilities, "hosted_flow")
	}
	if operations.RecoverAction != nil {
		capabilities = append(capabilities, "action_recovery")
	}
	registration.entry.descriptor.Payment.Capabilities = capabilities
	return registration
}

func Register[T any](
	key string,
	presentation integration.Presentation,
	contract config.Contract[T],
	payment ProviderDescriptor,
	create func(context.Context, T, CreateRequest) (Action, error),
) Registration {
	definitionErr := contract.ValidateDefinition()
	if create == nil {
		definitionErr = errors.New("provider operation is missing")
	}
	return Registration{
		key: key, presentation: presentation, definitionErr: definitionErr,
		entry: registryEntry{
			descriptor: Descriptor{
				Key:          key,
				Presentation: presentation,
				ConfigSchema: contract.JSONSchema(),
				UISchema:     contract.UISchema(),
				Payment:      payment,
			},
			contract: configOps{
				create: contract.CreateWrite,
				update: contract.UpdateWrite,
				view:   contract.View,
			},
			create: func(ctx context.Context, values map[string]any, request CreateRequest) (Action, error) {
				typed, err := contract.Decode(values)
				if err != nil {
					return Action{}, fmt.Errorf("decode payment config: %w", err)
				}
				return create(ctx, typed, request)
			},
		},
	}
}

var paymentIconRefPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[A-Za-z][A-Za-z0-9]*$`)

func NewRegistry(registrations ...Registration) (Registry, error) {
	r := Registry{
		entries: make([]registryEntry, 0, len(registrations)),
		byKey:   make(map[string]int, len(registrations)),
	}
	for _, registration := range registrations {
		if registration.key == "" || registration.presentation.Name == "" ||
			registration.definitionErr != nil {
			return Registry{}, errors.New("payment provider registration is invalid")
		}
		if registration.presentation.IconRef != "" &&
			!paymentIconRefPattern.MatchString(registration.presentation.IconRef) {
			return Registry{}, fmt.Errorf("provider %q has invalid icon_ref", registration.key)
		}
		if _, exists := r.byKey[registration.key]; exists {
			return Registry{}, fmt.Errorf("provider key %q is duplicated", registration.key)
		}
		r.byKey[registration.key] = len(r.entries)
		r.entries = append(r.entries, registration.entry)
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

func (r Registry) Create(
	ctx context.Context,
	provider string,
	values map[string]any,
	request CreateRequest,
) (Action, error) {
	entry, ok := r.entry(provider)
	if !ok {
		return Action{}, ErrNotConfigured
	}
	return entry.create(ctx, values, request)
}

func (r Registry) Plan(
	ctx context.Context,
	provider string,
	values map[string]any,
	request PlanRequest,
) (PlanResult, error) {
	entry, ok := r.usableEntry(provider, values)
	if !ok {
		return PlanResult{}, ErrNotConfigured
	}
	if entry.plan != nil {
		return entry.plan(ctx, values, request)
	}
	return PlanResult{}, ErrNotConfigured
}

func (r Registry) Query(
	ctx context.Context,
	provider string,
	values map[string]any,
	outTradeNo string,
) (QueryResult, error) {
	entry, ok := r.usableEntry(provider, values)
	if !ok {
		return QueryResult{}, ErrNotConfigured
	}
	if entry.query != nil {
		return entry.query(ctx, values, outTradeNo)
	}
	return QueryResult{}, ErrNotConfigured
}

func (r Registry) Refund(
	ctx context.Context,
	provider string,
	values map[string]any,
	request RefundRequest,
) (RefundResult, error) {
	entry, ok := r.usableEntry(provider, values)
	if !ok {
		return RefundResult{}, ErrNotConfigured
	}
	if entry.refund != nil {
		return entry.refund(ctx, values, request)
	}
	return RefundResult{}, ErrNotConfigured
}

func (r Registry) QueryRefund(
	ctx context.Context,
	provider string,
	values map[string]any,
	refundNo string,
) (bool, error) {
	entry, ok := r.usableEntry(provider, values)
	if !ok {
		return false, ErrNotConfigured
	}
	if entry.queryRefund != nil {
		return entry.queryRefund(ctx, values, refundNo)
	}
	return false, ErrNotConfigured
}

func (r Registry) ParseNotify(
	ctx context.Context,
	provider string,
	values map[string]any,
	request *http.Request,
) (NotifyResult, error) {
	entry, ok := r.usableEntry(provider, values)
	if !ok {
		return NotifyResult{}, ErrNotConfigured
	}
	if entry.parseNotify != nil {
		return entry.parseNotify(ctx, values, request)
	}
	return NotifyResult{}, ErrNotConfigured
}

func (r Registry) RecoverAction(
	ctx context.Context,
	provider string,
	values map[string]any,
	outTradeNo string,
) (Action, error) {
	entry, ok := r.usableEntry(provider, values)
	if !ok || entry.recoverAction == nil {
		return Action{}, ErrNotConfigured
	}
	return entry.recoverAction(ctx, values, outTradeNo)
}

func (r Registry) RenderHostedFlow(provider, product, payload string) ([]byte, error) {
	entry, ok := r.entry(provider)
	if !ok {
		return nil, ErrNotConfigured
	}
	if entry.renderHostedFlow != nil {
		return entry.renderHostedFlow(product, payload)
	}
	return nil, ErrNotConfigured
}

func (r Registry) CreateConfig(
	provider string,
	values map[string]any,
	secrets map[string]string,
) (map[string]any, error) {
	entry, ok := r.entry(provider)
	if !ok {
		return nil, fmt.Errorf("unknown payment provider %q", provider)
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
		return nil, fmt.Errorf("unknown payment provider %q", provider)
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

func (r Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, len(r.entries))
	for i, entry := range r.entries {
		out[i] = entry.descriptor
		out[i].ConfigSchema = clonePaymentSchema(entry.descriptor.ConfigSchema)
		out[i].UISchema = clonePaymentSchema(entry.descriptor.UISchema)
		out[i].Payment = cloneProviderDescriptor(entry.descriptor.Payment)
	}
	return out
}

func (r Registry) ConfigUsable(provider string, values map[string]any) bool {
	_, ok := r.usableEntry(provider, values)
	return ok
}

func (r Registry) Describe(provider string) (ProviderDescriptor, bool) {
	entry, ok := r.entry(provider)
	if !ok {
		return ProviderDescriptor{}, false
	}
	return cloneProviderDescriptor(entry.descriptor.Payment), true
}

func (r Registry) Providers() []string {
	providers := make([]string, len(r.entries))
	for i, entry := range r.entries {
		providers[i] = entry.descriptor.Key
	}
	return providers
}

func (r Registry) entry(key string) (registryEntry, bool) {
	index, ok := r.byKey[key]
	if !ok {
		return registryEntry{}, false
	}
	return r.entries[index], true
}

func (r Registry) usableEntry(provider string, values map[string]any) (registryEntry, bool) {
	entry, ok := r.entry(provider)
	if !ok || entry.query == nil {
		return registryEntry{}, false
	}
	if entry.valid != nil {
		return entry, entry.valid(values)
	}
	_, valid := entry.contract.view(values)
	return entry, valid
}

func clonePaymentSchema(schema map[string]any) map[string]any {
	raw, _ := json.Marshal(schema)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func cloneProviderDescriptor(descriptor ProviderDescriptor) ProviderDescriptor {
	out := descriptor
	out.Methods = slices.Clone(descriptor.Methods)
	out.Products = slices.Clone(descriptor.Products)
	out.Capabilities = slices.Clone(descriptor.Capabilities)
	for i := range out.Products {
		out.Products[i].Input.Fields = slices.Clone(descriptor.Products[i].Input.Fields)
		for j := range out.Products[i].Input.Fields {
			out.Products[i].Input.Fields[j].Options = slices.Clone(
				descriptor.Products[i].Input.Fields[j].Options,
			)
			if condition := descriptor.Products[i].Input.Fields[j].ShowWhen; condition != nil {
				cloned := *condition
				cloned.Values = slices.Clone(condition.Values)
				out.Products[i].Input.Fields[j].ShowWhen = &cloned
			}
		}
	}
	return out
}
