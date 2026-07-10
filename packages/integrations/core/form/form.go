// Package form describes runtime-rendered forms without settings-specific
// secret or immutability semantics. Provider config and payment product inputs
// share this contract.
package form

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Type string

const (
	String  Type = "string"
	Text    Type = "text"
	Int     Type = "int"
	Bool    Type = "bool"
	Enum    Type = "enum"
	Strings Type = "string_array"
)

type Field struct {
	Key      string
	Label    string
	Type     Type
	Required bool
	Options  []Option
	Help     string
	MaxLen   int
	Pattern  string
	Min      int
	Max      int
	Unique   bool
	ShowWhen *ShowWhen
}

type ShowWhen struct {
	Field  string
	Values []string
}

type Option struct {
	Value       string
	Label       string
	Description string
}

type Schema struct {
	Fields []Field
}

func (s Schema) ValidateDefinition() error {
	if len(s.Fields) == 0 {
		return fmt.Errorf("表单至少需要一个字段")
	}
	known := make(map[string]struct{}, len(s.Fields))
	for _, field := range s.Fields {
		if field.Key == "" || field.Label == "" || field.Type == "" {
			return fmt.Errorf("表单字段缺少 key、label 或 type")
		}
		if _, ok := known[field.Key]; ok {
			return fmt.Errorf("表单字段 %q 重复", field.Key)
		}
		known[field.Key] = struct{}{}
	}
	for _, field := range s.Fields {
		if field.ShowWhen != nil {
			if _, ok := known[field.ShowWhen.Field]; !ok {
				return fmt.Errorf("表单字段 %q 引用了未知条件字段 %q", field.Key, field.ShowWhen.Field)
			}
		}
	}
	return nil
}

func (s Schema) Validate(values map[string]any) error {
	known := make(map[string]struct{}, len(s.Fields))
	for _, field := range s.Fields {
		known[field.Key] = struct{}{}
	}
	for key := range values {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("未知表单字段 %q", key)
		}
	}
	for _, field := range s.Fields {
		if !s.fieldActive(field, values) {
			continue
		}
		value := values[field.Key]
		if isEmpty(value) {
			if field.Required {
				return fmt.Errorf("表单字段 %q 为必填项", field.Key)
			}
			continue
		}
		if err := validateValue(field, value); err != nil {
			return err
		}
	}
	return nil
}

func (s Schema) Usable(values map[string]any) bool {
	for _, field := range s.Fields {
		if field.Required && s.fieldActive(field, values) && isEmpty(values[field.Key]) {
			return false
		}
	}
	return true
}

func (s Schema) fieldActive(field Field, values map[string]any) bool {
	if field.ShowWhen == nil {
		return true
	}
	current, _ := values[field.ShowWhen.Field].(string)
	return slices.Contains(field.ShowWhen.Values, current)
}

func validateValue(field Field, value any) error {
	switch field.Type {
	case String, Text, Enum:
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("表单字段 %q 必须是字符串", field.Key)
		}
		return validateString(field, str)
	case Int:
		n, ok := number(value)
		if !ok {
			return fmt.Errorf("表单字段 %q 必须是整数", field.Key)
		}
		if field.Min != 0 && n < field.Min || field.Max != 0 && n > field.Max {
			return fmt.Errorf("表单字段 %q 必须在 %d 到 %d 之间", field.Key, field.Min, field.Max)
		}
	case Bool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("表单字段 %q 必须是布尔值", field.Key)
		}
	case Strings:
		values, ok := stringSlice(value)
		if !ok {
			return fmt.Errorf("表单字段 %q 必须是字符串数组", field.Key)
		}
		if field.Unique && duplicates(values) {
			return fmt.Errorf("表单字段 %q 不能包含重复值", field.Key)
		}
		for _, item := range values {
			if err := validateString(field, item); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("表单字段 %q 使用了未知类型 %q", field.Key, field.Type)
	}
	return nil
}

func validateString(field Field, value string) error {
	if field.MaxLen > 0 && len(value) > field.MaxLen {
		return fmt.Errorf("表单字段 %q 不能超过 %d 个字符", field.Key, field.MaxLen)
	}
	if field.Pattern != "" {
		re, err := regexp.Compile(field.Pattern)
		if err != nil {
			return fmt.Errorf("表单字段 %q 的格式规则无效", field.Key)
		}
		if !re.MatchString(value) {
			return fmt.Errorf("表单字段 %q 格式不正确", field.Key)
		}
	}
	if len(field.Options) > 0 && !slices.ContainsFunc(field.Options, func(option Option) bool {
		return option.Value == value
	}) {
		return fmt.Errorf("表单字段 %q 必须是所列选项之一", field.Key)
	}
	return nil
}

func (s Schema) JSONForm() (map[string]any, map[string]any) {
	properties := map[string]any{}
	required := []any{}
	order := make([]any, 0, len(s.Fields))
	ui := map[string]any{}
	type group struct {
		condition *ShowWhen
		fields    []Field
	}
	groups := map[string]*group{}
	groupOrder := []string{}
	for _, field := range s.Fields {
		order = append(order, field.Key)
		if fieldUI := fieldUI(field); len(fieldUI) > 0 {
			ui[field.Key] = fieldUI
		}
		if field.ShowWhen != nil {
			id := field.ShowWhen.Field + "\x00" + strings.Join(field.ShowWhen.Values, "\x00")
			if groups[id] == nil {
				groups[id] = &group{condition: field.ShowWhen}
				groupOrder = append(groupOrder, id)
			}
			groups[id].fields = append(groups[id].fields, field)
			continue
		}
		properties[field.Key] = fieldJSON(field)
		if field.Required {
			required = append(required, field.Key)
		}
	}
	jsonSchema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		jsonSchema["required"] = required
	}
	if len(groupOrder) > 0 {
		allOf := make([]any, 0, len(groupOrder))
		for _, id := range groupOrder {
			allOf = append(allOf, conditionalClause(groups[id].condition, groups[id].fields))
		}
		jsonSchema["allOf"] = allOf
	}
	ui["ui:order"] = order
	return jsonSchema, ui
}

func conditionalClause(condition *ShowWhen, fields []Field) map[string]any {
	properties := map[string]any{}
	required := []any{}
	for _, field := range fields {
		properties[field.Key] = fieldJSON(field)
		if field.Required {
			required = append(required, field.Key)
		}
	}
	values := make([]any, len(condition.Values))
	for i, value := range condition.Values {
		values[i] = value
	}
	then := map[string]any{"properties": properties}
	if len(required) > 0 {
		then["required"] = required
	}
	return map[string]any{
		"if": map[string]any{
			"properties": map[string]any{condition.Field: map[string]any{"enum": values}},
			"required":   []any{condition.Field},
		},
		"then": then,
	}
}

func fieldJSON(field Field) map[string]any {
	switch field.Type {
	case Int:
		out := map[string]any{"type": "integer", "title": field.Label}
		if field.Min != 0 {
			out["minimum"] = field.Min
		}
		if field.Max != 0 {
			out["maximum"] = field.Max
		}
		return out
	case Bool:
		return map[string]any{"type": "boolean", "title": field.Label}
	case Enum:
		return map[string]any{
			"type":  "string",
			"title": field.Label,
			"oneOf": optionOneOf(field.Options),
		}
	case Strings:
		items := map[string]any{"type": "string"}
		if len(field.Options) > 0 {
			items["oneOf"] = optionOneOf(field.Options)
		}
		out := map[string]any{"type": "array", "title": field.Label, "items": items}
		if field.Unique {
			out["uniqueItems"] = true
		}
		return out
	default:
		out := map[string]any{"type": "string", "title": field.Label}
		if field.MaxLen > 0 {
			out["maxLength"] = field.MaxLen
		}
		if field.Pattern != "" {
			out["pattern"] = field.Pattern
		}
		return out
	}
}

func optionOneOf(options []Option) []any {
	out := make([]any, len(options))
	for i, option := range options {
		label := option.Label
		if label == "" {
			label = option.Value
		}
		out[i] = map[string]any{"const": option.Value, "title": label}
	}
	return out
}

func fieldUI(field Field) map[string]any {
	ui := map[string]any{}
	options := map[string]any{}
	switch field.Type {
	case Enum, Strings:
		ui["ui:widget"] = "optionSelect"
		ui["ui:placeholder"] = "请选择"
		if descriptions := optionDescriptions(field.Options); len(descriptions) > 0 {
			options["descriptions"] = descriptions
		}
	case Text:
		ui["ui:widget"] = "textarea"
		ui["ui:placeholder"] = "请输入" + field.Label
	case Bool:
	default:
		ui["ui:placeholder"] = "请输入" + field.Label
	}
	if field.Help != "" {
		ui["ui:help"] = field.Help
	}
	if len(options) > 0 {
		ui["ui:options"] = options
	}
	return ui
}

func optionDescriptions(options []Option) map[string]any {
	out := map[string]any{}
	for _, option := range options {
		if option.Description != "" {
			out[option.Value] = option.Description
		}
	}
	return out
}

func isEmpty(value any) bool {
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

func stringSlice(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, value)
		}
		return out, true
	default:
		return nil, false
	}
}

func number(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	}
	return 0, false
}

func duplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
