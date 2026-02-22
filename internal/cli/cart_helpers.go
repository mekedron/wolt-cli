package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type optionSelection struct {
	ValueID string
	Count   int
}

type optionValueSpec struct {
	ID    string
	Name  string
	Price int
}

type optionGroupSpec struct {
	ID        string
	Name      string
	Required  bool
	MinSelect int
	MaxSelect int
	Values    map[string]optionValueSpec
}

func parseOptionSelections(raw []string) (map[string][]optionSelection, error) {
	result := map[string][]optionSelection{}
	for _, item := range raw {
		token := strings.TrimSpace(item)
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid --option value %q, expected group-id=value-id or group-id=value-id:count", item)
		}
		groupID := strings.TrimSpace(parts[0])
		valueToken := strings.TrimSpace(parts[1])
		valueID := valueToken
		count := 1
		if strings.Contains(valueToken, ":") {
			valueParts := strings.SplitN(valueToken, ":", 2)
			valueID = strings.TrimSpace(valueParts[0])
			countToken := strings.TrimSpace(valueParts[1])
			if valueID == "" || countToken == "" {
				return nil, fmt.Errorf("invalid --option value %q, expected group-id=value-id or group-id=value-id:count", item)
			}
			parsedCount, err := strconv.Atoi(countToken)
			if err != nil || parsedCount <= 0 {
				return nil, fmt.Errorf("invalid --option value %q, count must be a positive integer", item)
			}
			count = parsedCount
		}
		if valueID == "" {
			return nil, fmt.Errorf("invalid --option value %q, value-id is required", item)
		}
		result[groupID] = append(result[groupID], optionSelection{ValueID: valueID, Count: count})
	}
	return result, nil
}

func buildBasketOptions(itemPayload map[string]any, selections map[string][]optionSelection) []any {
	optionSpecs := extractOptionSpecs(itemPayload)
	groupIDs := make([]string, 0, len(optionSpecs))
	for groupID := range optionSpecs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	resolvedSelections := map[string][]optionSelection{}
	if len(optionSpecs) > 0 {
		for rawGroupToken, choices := range selections {
			resolvedGroupID := resolveOptionGroupToken(rawGroupToken, optionSpecs)
			if resolvedGroupID == "" {
				resolvedGroupID = strings.TrimSpace(rawGroupToken)
			}
			resolvedSelections[resolvedGroupID] = append(resolvedSelections[resolvedGroupID], choices...)
		}
	} else {
		for rawGroupToken, choices := range selections {
			groupID := strings.TrimSpace(rawGroupToken)
			if groupID == "" {
				continue
			}
			resolvedSelections[groupID] = append(resolvedSelections[groupID], choices...)
			groupIDs = append(groupIDs, groupID)
		}
		sort.Strings(groupIDs)
		groupIDs = dedupeStrings(groupIDs)
	}

	options := make([]any, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		groupSpec := optionSpecs[groupID]
		choices := resolvedSelections[groupID]
		values := make([]any, 0, len(choices))
		for _, choice := range choices {
			valueID := choice.ValueID
			if resolvedValueID := resolveOptionValueToken(choice.ValueID, groupSpec); resolvedValueID != "" {
				valueID = resolvedValueID
			}
			price := 0
			if valueSpec, ok := groupSpec.Values[valueID]; ok {
				price = valueSpec.Price
			}
			values = append(values, map[string]any{
				"id":    valueID,
				"count": choice.Count,
				"price": price,
			})
		}
		options = append(options, map[string]any{
			"id":     groupID,
			"values": values,
		})
	}
	return options
}

func extractOptionSpecs(payload map[string]any) map[string]optionGroupSpec {
	specs := map[string]optionGroupSpec{}
	visitOptionGroupCandidates(payload, func(group map[string]any) {
		groupID := strings.TrimSpace(asString(coalesceAny(group["id"], group["group_id"])))
		if groupID == "" {
			return
		}

		spec := specs[groupID]
		if spec.ID == "" {
			spec.ID = groupID
			spec.Name = asString(coalesceAny(group["name"], group["title"]))
			spec.Required = asBool(group["required"])
			spec.MinSelect = asInt(coalesceAny(group["min"], group["minimum"], group["min_select"]))
			spec.MaxSelect = asInt(coalesceAny(group["max"], group["maximum"], group["max_select"]))
			spec.Values = map[string]optionValueSpec{}
		}

		for _, value := range asSlice(coalesceAny(group["values"], group["options"], group["items"])) {
			valueMap := asMap(value)
			if valueMap == nil {
				continue
			}
			valueID := strings.TrimSpace(asString(coalesceAny(valueMap["id"], valueMap["value_id"])))
			if valueID == "" {
				continue
			}
			price := asInt(valueMap["price"])
			if price == 0 {
				price = asInt(asMap(valueMap["price"])["amount"])
			}
			spec.Values[valueID] = optionValueSpec{
				ID:    valueID,
				Name:  asString(coalesceAny(valueMap["name"], valueMap["title"])),
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
			if groups := asSlice(coalesceAny(typed["option_groups"], typed["options"])); len(groups) > 0 {
				for _, groupValue := range groups {
					group := asMap(groupValue)
					if group == nil {
						continue
					}
					if strings.TrimSpace(asString(coalesceAny(group["id"], group["group_id"]))) != "" {
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

func buildItemPayloadFromAssortment(assortment map[string]any, itemID string) map[string]any {
	targetItemID := strings.TrimSpace(itemID)
	if targetItemID == "" || assortment == nil {
		return nil
	}

	var item map[string]any
	for _, rawItem := range asSlice(assortment["items"]) {
		candidate := asMap(rawItem)
		if candidate == nil {
			continue
		}
		candidateID := strings.TrimSpace(asString(coalesceAny(candidate["item_id"], candidate["id"])))
		if candidateID != targetItemID {
			continue
		}
		item = candidate
		break
	}
	if item == nil {
		return nil
	}

	priceAmount := asInt(item["price"])
	if priceAmount <= 0 {
		priceAmount = asInt(item["base_price"])
	}
	if priceAmount <= 0 {
		priceAmount = asInt(asMap(item["price"])["amount"])
	}
	currency := strings.TrimSpace(asString(coalesceAny(
		item["currency"],
		asMap(item["price"])["currency"],
		asMap(asMap(assortment["venue"])["price"])["currency"],
		asMap(assortment["venue"])["currency"],
	)))
	if currency == "" {
		currency = "EUR"
	}

	optionGroupIDs := extractAssortmentOptionGroupIDs(item)
	optionGroupIndex := map[string]map[string]any{}
	for _, rawGroup := range asSlice(coalesceAny(assortment["options"], assortment["option_groups"])) {
		group := asMap(rawGroup)
		if group == nil {
			continue
		}
		groupID := strings.TrimSpace(asString(coalesceAny(group["id"], group["option_id"], group["group_id"])))
		if groupID == "" {
			continue
		}
		optionGroupIndex[groupID] = group
	}
	optionGroups := make([]any, 0, len(optionGroupIDs))
	for _, groupID := range optionGroupIDs {
		if group, ok := optionGroupIndex[groupID]; ok {
			optionGroups = append(optionGroups, group)
		}
	}
	if len(optionGroups) == 0 {
		optionGroups = asSlice(coalesceAny(item["option_groups"], item["options"]))
	}

	price := map[string]any{
		"amount":   priceAmount,
		"currency": currency,
	}
	return map[string]any{
		"item_id":       targetItemID,
		"id":            targetItemID,
		"name":          item["name"],
		"description":   coalesceAny(item["description"], ""),
		"price":         price,
		"base_price":    price,
		"option_groups": optionGroups,
		"options":       optionGroups,
		"items": []any{
			map[string]any{
				"id":            targetItemID,
				"item_id":       targetItemID,
				"name":          item["name"],
				"description":   coalesceAny(item["description"], ""),
				"price":         price,
				"base_price":    price,
				"option_groups": optionGroups,
				"options":       optionGroups,
			},
		},
	}
}

func extractAssortmentOptionGroupIDs(item map[string]any) []string {
	groupIDs := []string{}
	for _, value := range asSlice(item["option_group_ids"]) {
		groupID := strings.TrimSpace(asString(value))
		if groupID == "" {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	for _, optionValue := range asSlice(item["options"]) {
		option := asMap(optionValue)
		if option == nil {
			continue
		}
		groupID := strings.TrimSpace(asString(coalesceAny(option["option_id"], option["id"], option["group_id"])))
		if groupID == "" {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	return dedupeStrings(groupIDs)
}

func inferCurrency(formatted string) string {
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

func formatMinorAmount(amount int, currency string) string {
	currency = strings.TrimSpace(currency)
	if currency == "" {
		return ""
	}
	switch currency {
	case "EUR":
		return fmt.Sprintf("€%.2f", float64(amount)/100)
	case "USD":
		return fmt.Sprintf("$%.2f", float64(amount)/100)
	default:
		return fmt.Sprintf("%s %.2f", currency, float64(amount)/100)
	}
}

func resolveOptionGroupToken(token string, specs map[string]optionGroupSpec) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if _, ok := specs[token]; ok {
		return token
	}
	for groupID, spec := range specs {
		if strings.EqualFold(groupID, token) || strings.EqualFold(spec.Name, token) {
			return groupID
		}
	}
	return ""
}

func resolveOptionValueToken(token string, group optionGroupSpec) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if _, ok := group.Values[token]; ok {
		return token
	}
	for valueID, valueSpec := range group.Values {
		if strings.EqualFold(valueID, token) || strings.EqualFold(valueSpec.Name, token) {
			return valueID
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
