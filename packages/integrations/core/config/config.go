// Package config adds settings-only secret and immutable semantics to the
// neutral form config.
package config

import (
	"fmt"
	"slices"

	"github.com/imbytecat/moonbase/integrations/core/form"
)

type Field struct {
	Key       string
	Label     string
	Type      form.Type
	Secret    bool
	Immutable bool
	Required  bool
	Options   []form.Option
	Help      string
	MaxLen    int
	Pattern   string
	Min       int
	Max       int
	Unique    bool
	ShowWhen  *form.ShowWhen
}

type Type = form.Type
type Option = form.Option
type ShowWhen = form.ShowWhen

const (
	String  = form.String
	Text    = form.Text
	Int     = form.Int
	Bool    = form.Bool
	Enum    = form.Enum
	Strings = form.Strings
)

type Schema struct {
	Fields []Field
}

const setSuffix = "_set"

func (s Schema) Form() form.Schema {
	fields := make([]form.Field, len(s.Fields))
	for i, field := range s.Fields {
		fields[i] = form.Field{
			Key: field.Key, Label: field.Label, Type: field.Type, Required: field.Required,
			Options: field.Options, Help: field.Help, MaxLen: field.MaxLen, Pattern: field.Pattern,
			Min: field.Min, Max: field.Max, Unique: field.Unique, ShowWhen: field.ShowWhen,
		}
	}
	return form.Schema{Fields: fields}
}

func (s Schema) ValidateDefinition() error { return s.Form().ValidateDefinition() }

func (s Schema) Mask(values map[string]any) map[string]any {
	out := make(map[string]any, len(s.Fields)*2)
	for _, field := range s.Fields {
		out[field.Key] = values[field.Key]
		if !field.Secret {
			continue
		}
		out[field.Key+setSuffix] = !empty(out[field.Key])
		out[field.Key] = ""
	}
	return out
}

func (s Schema) Merge(incoming, stored map[string]any) map[string]any {
	out := make(map[string]any, len(s.Fields))
	for _, field := range s.Fields {
		key := field.Key
		out[key] = incoming[key]
		switch {
		case field.Immutable:
			if value, ok := stored[key]; ok {
				out[key] = value
			}
		case field.Secret:
			if empty(out[key]) {
				out[key] = stored[key]
			}
		}
	}
	return out
}

func (s Schema) Validate(values map[string]any) error {
	clean := make(map[string]any, len(values))
	for key, value := range values {
		if slices.ContainsFunc(s.Fields, func(field Field) bool { return key == field.Key+setSuffix }) {
			return fmt.Errorf("配置字段 %q 创建后不可修改", key)
		}
		clean[key] = value
	}
	return s.Form().Validate(clean)
}

func (s Schema) Usable(values map[string]any) bool { return s.Form().Usable(values) }

func (s Schema) JSONForm() (map[string]any, map[string]any) {
	jsonSchema, uiSchema := s.Form().JSONForm()
	for _, field := range s.Fields {
		if !field.Secret && !field.Immutable {
			continue
		}
		key := field.Key
		ui, _ := uiSchema[key].(map[string]any)
		if ui == nil {
			ui = map[string]any{}
			uiSchema[key] = ui
		}
		options, _ := ui["ui:options"].(map[string]any)
		if options == nil {
			options = map[string]any{}
		}
		if field.Secret {
			ui["ui:widget"] = "secret"
			ui["ui:placeholder"] = "请输入" + field.Label
			options["secret"] = true
		}
		if field.Immutable {
			options["immutable"] = true
		}
		ui["ui:options"] = options
	}
	return jsonSchema, uiSchema
}

func empty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []string:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}
