package payloadutil

import (
	"fmt"
	"reflect"
	"strings"
)

type OptionValueSpec struct {
	ID    string
	Name  string
	Price int
}

type OptionGroupSpec struct {
	ID        string
	Name      string
	Required  bool
	MinSelect int
	MaxSelect int
	Values    map[string]OptionValueSpec
}

func ExtractOptionSpecs(payload map[string]any) map[string]OptionGroupSpec {
	specs := map[string]OptionGroupSpec{}
	visitOptionGroupCandidates(payload, func(group map[string]any) {
		groupID := strings.TrimSpace(String(CoalesceAny(group["id"], group["group_id"])))
		if groupID == "" {
			return
		}

		spec := specs[groupID]
		if spec.ID == "" {
			spec.ID = groupID
			spec.Name = String(CoalesceAny(group["name"], group["title"]))
			spec.Required = Bool(group["required"])
			spec.MinSelect = Int(CoalesceAny(group["min"], group["minimum"], group["min_select"]))
			spec.MaxSelect = Int(CoalesceAny(group["max"], group["maximum"], group["max_select"]))
			spec.Values = map[string]OptionValueSpec{}
		}

		for _, value := range Slice(CoalesceAny(group["values"], group["options"], group["items"])) {
			valueMap := Map(value)
			if valueMap == nil {
				continue
			}
			valueID := strings.TrimSpace(String(CoalesceAny(valueMap["id"], valueMap["value_id"])))
			if valueID == "" {
				continue
			}
			price := Int(valueMap["price"])
			if price == 0 {
				price = Int(Map(valueMap["price"])["amount"])
			}
			spec.Values[valueID] = OptionValueSpec{
				ID:    valueID,
				Name:  String(CoalesceAny(valueMap["name"], valueMap["title"])),
				Price: price,
			}
		}
		specs[groupID] = spec
	})
	return specs
}

func visitOptionGroupCandidates(payload map[string]any, visit func(map[string]any)) {
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if groups := Slice(CoalesceAny(typed["option_groups"], typed["options"])); len(groups) > 0 {
				for _, groupValue := range groups {
					group := Map(groupValue)
					if group == nil {
						continue
					}
					if strings.TrimSpace(String(CoalesceAny(group["id"], group["group_id"]))) != "" {
						visit(group)
					}
				}
			}
			for _, nested := range typed {
				walk(nested)
			}
		case []any:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(payload)
}

func InferCurrency(formatted string) string {
	formatted = strings.TrimSpace(formatted)
	if formatted == "" {
		return ""
	}
	switch {
	case strings.Contains(formatted, "€"):
		return "EUR"
	case strings.Contains(formatted, "$"):
		return "USD"
	case strings.HasPrefix(formatted, "PLN"):
		return "PLN"
	default:
		return ""
	}
}

func Map(value any) map[string]any {
	if value == nil {
		return nil
	}
	switch m := value.(type) {
	case map[string]any:
		return m
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	return nil
}

func Slice(value any) []any {
	if value == nil {
		return nil
	}
	if values, ok := value.([]any); ok {
		return values
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil
	}
	kind := rv.Kind()
	if kind != reflect.Slice && kind != reflect.Array {
		return nil
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil
	}
	values := make([]any, rv.Len())
	for idx := 0; idx < rv.Len(); idx++ {
		values[idx] = rv.Index(idx).Interface()
	}
	return values
}

func String(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func Bool(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

func Int(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func CoalesceAny(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}
