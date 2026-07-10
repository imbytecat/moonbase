package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	invopop "github.com/invopop/jsonschema"
	santhosh "github.com/santhosh-tekuri/jsonschema/v6"
)

// Policy declares lifecycle behavior that JSON Schema does not express.
type Policy struct {
	Secrets    []string
	CreateOnly []string
}

// Contract is the immutable, compiled config contract for one provider.
type Contract[T any] struct {
	schema    map[string]any
	uiSchema  map[string]any
	validator *santhosh.Schema
	policy    Policy
}

// View is the read-safe projection of a stored config.
type View struct {
	Values         map[string]any
	SetSecretPaths []string
}

// MergeWrite combines ordinary values with non-empty secret replacements.
// The result is suitable for Contract.Create/Update.
func MergeWrite(values map[string]any, secrets map[string]string) (map[string]any, error) {
	out, err := cloneMap(values)
	if err != nil {
		return nil, err
	}
	for path, secret := range secrets {
		if secret == "" {
			return nil, fmt.Errorf("secret %q must be non-empty", path)
		}
		if err := setValueAt(out, path, secret); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// MustContract constructs a Contract and panics when the provider definition
// is invalid. Provider registrations are compile-time composition, so a bad
// contract is a developer error rather than runtime data.
func MustContract[T any](policy Policy) Contract[T] {
	contract, err := NewContract[T](policy)
	if err != nil {
		panic(err)
	}
	return contract
}

// NewContract reflects T into Draft 2020-12 JSON Schema and compiles it once.
func NewContract[T any](policy Policy) (Contract[T], error) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	if t.Kind() != reflect.Struct {
		return Contract[T]{}, fmt.Errorf("config type %s must be a struct", t)
	}

	reflector := invopop.Reflector{
		Anonymous:                  true,
		DoNotReference:             true,
		RequiredFromJSONSchemaTags: true,
		AllowAdditionalProperties:  false,
	}
	reflected := reflector.ReflectFromType(t)
	raw, err := json.Marshal(reflected)
	if err != nil {
		return Contract[T]{}, fmt.Errorf("encode config schema: %w", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return Contract[T]{}, fmt.Errorf("project config schema: %w", err)
	}
	uiSchema, err := applyPolicy(schema, policy)
	if err != nil {
		return Contract[T]{}, err
	}
	raw, err = json.Marshal(schema)
	if err != nil {
		return Contract[T]{}, fmt.Errorf("encode projected config schema: %w", err)
	}

	document, err := santhosh.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Contract[T]{}, fmt.Errorf("decode config schema: %w", err)
	}
	compiler := santhosh.NewCompiler()
	const schemaURL = "urn:moonbase:provider-config"
	if err := compiler.AddResource(schemaURL, document); err != nil {
		return Contract[T]{}, fmt.Errorf("add config schema: %w", err)
	}
	validator, err := compiler.Compile(schemaURL)
	if err != nil {
		return Contract[T]{}, fmt.Errorf("compile config schema: %w", err)
	}

	return Contract[T]{
		schema:    schema,
		uiSchema:  uiSchema,
		validator: validator,
		policy:    policy,
	}, nil
}

// JSONSchema returns a detached copy of the generated standard schema.
func (c Contract[T]) JSONSchema() map[string]any {
	raw, _ := json.Marshal(c.schema)
	var schema map[string]any
	_ = json.Unmarshal(raw, &schema)
	return schema
}

// UISchema returns the minimal rjsf projection derived from lifecycle policy.
func (c Contract[T]) UISchema() map[string]any {
	raw, _ := json.Marshal(c.uiSchema)
	var schema map[string]any
	_ = json.Unmarshal(raw, &schema)
	return schema
}

// ValidateDefinition reports whether the contract was constructed and
// compiled successfully. Registries use it to reject a zero-value Contract.
func (c Contract[T]) ValidateDefinition() error {
	if c.validator == nil || c.schema == nil || c.uiSchema == nil {
		return errors.New("config contract is not initialized")
	}
	return nil
}

// Create validates input and returns its canonical typed representation.
func (c Contract[T]) Create(input map[string]any) (map[string]any, error) {
	typed, err := c.Decode(input)
	if err != nil {
		return nil, err
	}
	return encodeMap(typed)
}

// CreateWrite validates the wire split between ordinary values and secret
// replacements before materializing a complete canonical config.
func (c Contract[T]) CreateWrite(
	values map[string]any,
	secrets map[string]string,
) (map[string]any, error) {
	if err := c.rejectSecretsInValues(values); err != nil {
		return nil, err
	}
	if err := c.validateSecretReplacements(secrets); err != nil {
		return nil, err
	}
	input, err := MergeWrite(values, secrets)
	if err != nil {
		return nil, err
	}
	return c.Create(input)
}

// Update applies the small lifecycle policy: ordinary values are replaced,
// missing secrets are kept, present non-empty secrets replace the old value,
// and create-only fields may be omitted or repeated unchanged.
func (c Contract[T]) Update(input, stored map[string]any) (map[string]any, error) {
	candidate, err := cloneMap(input)
	if err != nil {
		return nil, err
	}
	for _, path := range c.policy.Secrets {
		value, present, err := valueAt(candidate, path)
		if err != nil {
			return nil, err
		}
		if present {
			secret, ok := value.(string)
			if !ok || secret == "" {
				return nil, fmt.Errorf("secret %q must be a non-empty string", path)
			}
			continue
		}
		if previous, ok, err := valueAt(stored, path); err != nil {
			return nil, err
		} else if ok {
			if err := setValueAt(candidate, path, previous); err != nil {
				return nil, err
			}
		}
	}
	for _, path := range c.policy.CreateOnly {
		previous, storedPresent, err := valueAt(stored, path)
		if err != nil {
			return nil, err
		}
		incoming, inputPresent, err := valueAt(candidate, path)
		if err != nil {
			return nil, err
		}
		switch {
		case inputPresent && storedPresent && !reflect.DeepEqual(incoming, previous):
			return nil, fmt.Errorf("config field %q cannot be changed", path)
		case !inputPresent && storedPresent:
			if err := setValueAt(candidate, path, previous); err != nil {
				return nil, err
			}
		}
	}
	return c.Create(candidate)
}

// UpdateWrite applies a split wire update to a stored config.
func (c Contract[T]) UpdateWrite(
	values map[string]any,
	secrets map[string]string,
	stored map[string]any,
) (map[string]any, error) {
	if err := c.rejectSecretsInValues(values); err != nil {
		return nil, err
	}
	if err := c.validateSecretReplacements(secrets); err != nil {
		return nil, err
	}
	input, err := MergeWrite(values, secrets)
	if err != nil {
		return nil, err
	}
	return c.Update(input, stored)
}

// View removes secret values and reports only their configured paths.
func (c Contract[T]) View(stored map[string]any) (View, bool) {
	_, validationErr := c.Create(stored)
	projected := projectKnownObject(c.schema, stored)
	setPaths := make([]string, 0, len(c.policy.Secrets))
	for _, path := range c.policy.Secrets {
		if value, ok, _ := valueAt(stored, path); ok {
			if secret, ok := value.(string); ok && secret != "" {
				setPaths = append(setPaths, path)
			}
		}
		if err := deleteValueAt(projected, path); err != nil {
			return View{Values: map[string]any{}}, false
		}
	}
	return View{Values: projected, SetSecretPaths: setPaths}, validationErr == nil
}

func (c Contract[T]) rejectSecretsInValues(values map[string]any) error {
	for _, path := range c.policy.Secrets {
		if _, present, err := valueAt(values, path); err != nil {
			return err
		} else if present {
			return fmt.Errorf("secret %q must not appear in ordinary values", path)
		}
	}
	return nil
}

func (c Contract[T]) validateSecretReplacements(secrets map[string]string) error {
	allowed := make(map[string]struct{}, len(c.policy.Secrets))
	for _, path := range c.policy.Secrets {
		allowed[path] = struct{}{}
	}
	for path := range secrets {
		if _, ok := allowed[path]; !ok {
			return fmt.Errorf("secret %q is not declared by the provider", path)
		}
	}
	return nil
}

// Decode validates input and strictly decodes it into the provider's private
// config type.
func (c Contract[T]) Decode(input map[string]any) (T, error) {
	var zero T
	raw, err := json.Marshal(input)
	if err != nil {
		return zero, fmt.Errorf("encode config: %w", err)
	}
	instance, err := santhosh.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return zero, fmt.Errorf("decode config value: %w", err)
	}
	if err := c.validator.Validate(instance); err != nil {
		return zero, fmt.Errorf("validate config: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var typed T
	if err := decoder.Decode(&typed); err != nil {
		return zero, fmt.Errorf("decode typed config: %w", err)
	}
	return typed, nil
}

func encodeMap[T any](value T) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode typed config: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode canonical config: %w", err)
	}
	return out, nil
}

func cloneMap(input map[string]any) (map[string]any, error) {
	if input == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return out, nil
}

func pointerTokens(path string) ([]string, error) {
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("invalid JSON Pointer %q", path)
	}
	raw := strings.Split(path[1:], "/")
	tokens := make([]string, len(raw))
	for i, token := range raw {
		for offset := 0; offset < len(token); offset++ {
			if token[offset] != '~' {
				continue
			}
			if offset+1 >= len(token) || (token[offset+1] != '0' && token[offset+1] != '1') {
				return nil, fmt.Errorf("invalid JSON Pointer %q", path)
			}
			offset++
		}
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		if token == "" {
			return nil, fmt.Errorf("invalid JSON Pointer %q", path)
		}
		tokens[i] = token
	}
	return tokens, nil
}

func valueAt(root map[string]any, path string) (any, bool, error) {
	tokens, err := pointerTokens(path)
	if err != nil {
		return nil, false, err
	}
	var current any = root
	for _, token := range tokens {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		current, ok = object[token]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}

func setValueAt(root map[string]any, path string, value any) error {
	tokens, err := pointerTokens(path)
	if err != nil {
		return err
	}
	current := root
	for _, token := range tokens[:len(tokens)-1] {
		next, ok := current[token]
		if !ok {
			child := map[string]any{}
			current[token] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("JSON Pointer %q crosses a non-object value", path)
		}
		current = child
	}
	current[tokens[len(tokens)-1]] = value
	return nil
}

func deleteValueAt(root map[string]any, path string) error {
	tokens, err := pointerTokens(path)
	if err != nil {
		return err
	}
	current := root
	for _, token := range tokens[:len(tokens)-1] {
		next, ok := current[token]
		if !ok {
			return nil
		}
		child, ok := next.(map[string]any)
		if !ok {
			return nil
		}
		current = child
	}
	delete(current, tokens[len(tokens)-1])
	return nil
}

func applyPolicy(schema map[string]any, policy Policy) (map[string]any, error) {
	ui := map[string]any{}
	type policyPath struct {
		kind   string
		path   string
		tokens []string
		field  map[string]any
	}
	paths := make([]policyPath, 0, len(policy.Secrets)+len(policy.CreateOnly))
	seen := map[string]string{}
	for _, item := range []struct {
		kind  string
		paths []string
	}{{kind: "secret", paths: policy.Secrets}, {kind: "create-only", paths: policy.CreateOnly}} {
		for _, path := range item.paths {
			if previous, ok := seen[path]; ok {
				return nil, fmt.Errorf(
					"policy path %q is declared as both %s and %s",
					path,
					previous,
					item.kind,
				)
			}
			field, tokens, err := schemaField(schema, path)
			if err != nil {
				return nil, err
			}
			_, hasProperties := field["properties"]
			_, hasItems := field["items"]
			if hasProperties || hasItems || field["type"] == "object" || field["type"] == "array" {
				return nil, fmt.Errorf("policy path %q must point to a leaf field", path)
			}
			seen[path] = item.kind
			paths = append(
				paths,
				policyPath{kind: item.kind, path: path, tokens: tokens, field: field},
			)
		}
	}
	for i, left := range paths {
		for _, right := range paths[i+1:] {
			if tokenPrefix(left.tokens, right.tokens) || tokenPrefix(right.tokens, left.tokens) {
				return nil, fmt.Errorf("policy paths %q and %q conflict", left.path, right.path)
			}
		}
	}
	for _, item := range paths {
		options := map[string]any{}
		if item.kind == "secret" {
			if item.field["type"] != "string" {
				return nil, fmt.Errorf("secret path %q must point to a string", item.path)
			}
			minLength, _ := item.field["minLength"].(float64)
			if minLength < 1 {
				return nil, fmt.Errorf("secret path %q must have minLength=1 or greater", item.path)
			}
			item.field["writeOnly"] = true
			options["secret"] = true
		} else {
			options["immutable"] = true
		}
		mergeUIOptions(ui, item.tokens, options)
	}
	return ui, nil
}

func tokenPrefix(left, right []string) bool {
	if len(left) >= len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func projectKnownObject(schema map[string]any, stored map[string]any) map[string]any {
	projected, _ := projectKnownValue(schema, stored).(map[string]any)
	if projected == nil {
		return map[string]any{}
	}
	return projected
}

func projectKnownValue(schema map[string]any, value any) any {
	if properties, ok := schema["properties"].(map[string]any); ok {
		object, ok := value.(map[string]any)
		if !ok {
			return value
		}
		out := make(map[string]any, len(properties))
		for key, rawField := range properties {
			storedValue, present := object[key]
			if !present {
				continue
			}
			field, _ := rawField.(map[string]any)
			out[key] = projectKnownValue(field, storedValue)
		}
		return out
	}
	if items, ok := schema["items"].(map[string]any); ok {
		array, ok := value.([]any)
		if !ok {
			return value
		}
		out := make([]any, len(array))
		for i, item := range array {
			out[i] = projectKnownValue(items, item)
		}
		return out
	}
	return value
}

func schemaField(root map[string]any, path string) (map[string]any, []string, error) {
	tokens, err := pointerTokens(path)
	if err != nil {
		return nil, nil, err
	}
	current := root
	for _, token := range tokens {
		properties, ok := current["properties"].(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("policy path %q crosses a non-object schema", path)
		}
		next, ok := properties[token].(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("policy path %q does not exist", path)
		}
		current = next
	}
	return current, tokens, nil
}

func mergeUIOptions(root map[string]any, tokens []string, options map[string]any) {
	current := root
	for _, token := range tokens {
		next, ok := current[token].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[token] = next
		}
		current = next
	}
	existing, _ := current["ui:options"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	for key, value := range options {
		existing[key] = value
	}
	current["ui:options"] = existing
	if options["secret"] == true {
		current["ui:widget"] = "secret"
	}
}
