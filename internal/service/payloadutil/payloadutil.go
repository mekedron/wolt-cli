package payloadutil

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var currencyCodePattern = regexp.MustCompile(`(?:^|[^A-Z])([A-Z]{3})(?:[^A-Z]|$)`)

// inferableCurrencies gates the *heuristic* code scan in InferCurrency only, so
// that ordinary three-letter words in a formatted label ("THE total") are not
// mistaken for currency codes. It must never gate an explicitly typed currency
// field — see NormalizeCurrency.
var inferableCurrencies = map[string]struct{}{
	"AED": {}, "ALL": {}, "AZN": {}, "BGN": {}, "CHF": {}, "CZK": {},
	"DKK": {}, "EUR": {}, "GBP": {}, "GEL": {}, "HUF": {}, "ILS": {},
	"ISK": {}, "JPY": {}, "KZT": {}, "MKD": {}, "NOK": {}, "PLN": {},
	"RON": {}, "RSD": {}, "SEK": {}, "TRY": {}, "UAH": {}, "USD": {},
}

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
	case strings.Contains(formatted, "₾"):
		return "GEL"
	case strings.Contains(formatted, "€"):
		return "EUR"
	case strings.Contains(formatted, "£"):
		return "GBP"
	case strings.Contains(formatted, "$"):
		return "USD"
	case strings.Contains(strings.ToLower(formatted), "zł"):
		return "PLN"
	}
	match := currencyCodePattern.FindStringSubmatch(strings.ToUpper(formatted))
	if len(match) == 2 {
		if _, ok := inferableCurrencies[match[1]]; ok {
			return match[1]
		}
	}
	return ""
}

// NormalizeCurrency validates the *shape* of an explicitly stated currency and
// returns it uppercased, or "" when the value is not an ISO 4217 alphabetic
// code.
//
// It deliberately does not consult an allowlist. Wolt states the currency
// explicitly in venue and basket payloads, and callers treat an empty result as
// "currency unverifiable" and fail closed — so gating explicit values on a
// hand-maintained list would take live markets offline whenever the list drifts
// behind Wolt's footprint. Verified against production: Albania reports "ALL"
// and North Macedonia reports "MKD", neither of which was on the original list.
// Guessing a code out of a formatted label is the only place an allowlist
// belongs; see inferableCurrencies.
func NormalizeCurrency(value string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	if len(code) != 3 {
		return ""
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return code
}

// CurrencyFromVenuePayload reads the explicit currency fields used by Wolt's
// static and dynamic venue payloads.
func CurrencyFromVenuePayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	candidates := []any{
		payload["currency"],
		payload["currency_code"],
	}
	for _, key := range []string{"venue", "venue_raw"} {
		venue := Map(payload[key])
		candidates = append(candidates,
			venue["currency"],
			venue["currency_code"],
			Map(venue["price"])["currency"],
		)
	}
	for _, candidate := range candidates {
		if currency := NormalizeCurrency(String(candidate)); currency != "" {
			return currency
		}
	}
	return ""
}

// CurrencyFromBasket prefers structured basket and venue currency fields, then
// falls back to the formatted total.
func CurrencyFromBasket(basket map[string]any) string {
	if basket == nil {
		return ""
	}
	venue := Map(basket["venue"])
	for _, candidate := range []any{
		basket["currency"],
		Map(basket["total_price"])["currency"],
		venue["currency"],
		venue["currency_code"],
	} {
		if currency := NormalizeCurrency(String(candidate)); currency != "" {
			return currency
		}
	}
	return InferCurrency(String(basket["total"]))
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
