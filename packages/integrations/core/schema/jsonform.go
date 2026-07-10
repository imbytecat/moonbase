package schema

import "strings"

// JSONForm converts a driver Schema into an rjsf-ready JSON Schema plus a
// companion uiSchema. Both are derived from the same Fields as Validate, so the
// rendered form and the server-side gate cannot drift (ADR-0010). Conditional
// fields (ShowWhen) become JSON Schema if/then clauses; ui:order keeps every
// field — conditional or not — in declared position.
func (s Schema) JSONForm() (jsonSchema map[string]any, uiSchema map[string]any) {
	properties := map[string]any{}
	required := []any{}
	order := make([]any, 0, len(s.Fields))
	ui := map[string]any{}

	type group struct {
		cond   *ShowWhen
		fields []Field
	}
	groups := map[string]*group{}
	groupOrder := []string{}

	for _, f := range s.Fields {
		order = append(order, f.Key)
		if u := fieldUI(f); len(u) > 0 {
			ui[f.Key] = u
		}
		if f.ShowWhen != nil {
			id := f.ShowWhen.Field + "\x00" + strings.Join(f.ShowWhen.Values, "\x00")
			g, ok := groups[id]
			if !ok {
				g = &group{cond: f.ShowWhen}
				groups[id] = g
				groupOrder = append(groupOrder, id)
			}
			g.fields = append(g.fields, f)
			continue
		}
		properties[f.Key] = fieldJSON(f)
		if f.Required {
			required = append(required, f.Key)
		}
	}

	jsonSchema = map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		jsonSchema["required"] = required
	}
	if len(groupOrder) > 0 {
		allOf := make([]any, 0, len(groupOrder))
		for _, id := range groupOrder {
			allOf = append(allOf, conditionalClause(groups[id].cond, groups[id].fields))
		}
		jsonSchema["allOf"] = allOf
	}

	ui["ui:order"] = order
	return jsonSchema, ui
}

func conditionalClause(cond *ShowWhen, fields []Field) map[string]any {
	props := map[string]any{}
	req := []any{}
	for _, f := range fields {
		props[f.Key] = fieldJSON(f)
		if f.Required {
			req = append(req, f.Key)
		}
	}
	values := make([]any, len(cond.Values))
	for i, v := range cond.Values {
		values[i] = v
	}
	then := map[string]any{"properties": props}
	if len(req) > 0 {
		then["required"] = req
	}
	return map[string]any{
		"if": map[string]any{
			"properties": map[string]any{cond.Field: map[string]any{"enum": values}},
			"required":   []any{cond.Field},
		},
		"then": then,
	}
}

func fieldJSON(f Field) map[string]any {
	switch f.Type {
	case Int:
		out := map[string]any{"type": "integer", "title": f.Label}
		if f.Min != 0 {
			out["minimum"] = f.Min
		}
		if f.Max != 0 {
			out["maximum"] = f.Max
		}
		return out
	case Bool:
		return map[string]any{"type": "boolean", "title": f.Label}
	case Enum:
		return map[string]any{"type": "string", "title": f.Label, "oneOf": optionOneOf(f.Options)}
	case Strings:
		items := map[string]any{"type": "string"}
		if len(f.Options) > 0 {
			items["oneOf"] = optionOneOf(f.Options)
		}
		out := map[string]any{"type": "array", "title": f.Label, "items": items}
		if f.Unique {
			out["uniqueItems"] = true
		}
		return out
	default:
		out := map[string]any{"type": "string", "title": f.Label}
		if f.MaxLen > 0 {
			out["maxLength"] = f.MaxLen
		}
		if f.Pattern != "" {
			out["pattern"] = f.Pattern
		}
		return out
	}
}

func optionOneOf(options []Option) []any {
	out := make([]any, len(options))
	for i, o := range options {
		label := o.Label
		if label == "" {
			label = o.Value
		}
		out[i] = map[string]any{"const": o.Value, "title": label}
	}
	return out
}

func fieldUI(f Field) map[string]any {
	ui := map[string]any{}
	opts := map[string]any{}

	switch {
	case f.Secret:
		ui["ui:widget"] = "secret"
		ui["ui:placeholder"] = "请输入" + f.Label
		opts["secret"] = true
	case f.Type == Enum || f.Type == Strings:
		ui["ui:widget"] = "optionSelect"
		ui["ui:placeholder"] = "请选择"
		if desc := optionDescriptions(f.Options); len(desc) > 0 {
			opts["descriptions"] = desc
		}
	case f.Type == Text:
		ui["ui:widget"] = "textarea"
		ui["ui:placeholder"] = "请输入" + f.Label
	case f.Type == Bool:
	default:
		ui["ui:placeholder"] = "请输入" + f.Label
	}

	if f.Immutable {
		opts["immutable"] = true
	}
	if f.Help != "" {
		ui["ui:help"] = f.Help
	}
	if len(opts) > 0 {
		ui["ui:options"] = opts
	}
	return ui
}

func optionDescriptions(options []Option) map[string]any {
	out := map[string]any{}
	for _, o := range options {
		if o.Description != "" {
			out[o.Value] = o.Description
		}
	}
	return out
}
