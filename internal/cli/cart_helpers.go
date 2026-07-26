package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

type optionSelection struct {
	ValueID string
	Count   int
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

func buildBasketOptions(
	itemPayload map[string]any,
	selections map[string][]optionSelection,
) ([]any, error) {
	optionSpecs := extractOptionSpecs(itemPayload)
	groupIDs := make([]string, 0, len(optionSpecs))
	for groupID := range optionSpecs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	resolvedSelections := map[string][]optionSelection{}
	if len(optionSpecs) > 0 {
		selectionGroupTokens := make([]string, 0, len(selections))
		for rawGroupToken := range selections {
			selectionGroupTokens = append(selectionGroupTokens, rawGroupToken)
		}
		sort.Strings(selectionGroupTokens)
		for _, rawGroupToken := range selectionGroupTokens {
			choices := selections[rawGroupToken]
			resolvedGroupID, err := resolveOptionGroupToken(rawGroupToken, optionSpecs)
			if err != nil {
				return nil, err
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
		selectionCount := 0
		valueCounts := make(map[string]int, len(choices))
		for _, choice := range choices {
			nextCount, ok := payloadutil.CheckedAddInt(selectionCount, choice.Count)
			if !ok {
				return nil, fmt.Errorf("option group %q selection count exceeds the supported integer range", groupID)
			}
			selectionCount = nextCount
			valueID := choice.ValueID
			resolvedValueID, err := resolveOptionValueToken(choice.ValueID, groupSpec)
			if err == nil {
				valueID = resolvedValueID
			} else if len(groupSpec.Values) > 0 {
				return nil, fmt.Errorf("option group %q: %w", groupID, err)
			}
			nextValueCount, ok := payloadutil.CheckedAddInt(valueCounts[valueID], choice.Count)
			if !ok {
				return nil, fmt.Errorf("option group %q value %q count exceeds the supported integer range", groupID, valueID)
			}
			valueCounts[valueID] = nextValueCount
		}
		minSelect := groupSpec.MinSelect
		if groupSpec.Required && minSelect < 1 {
			minSelect = 1
		}
		maxSelect := groupSpec.MaxSelect
		if minSelect < 0 || maxSelect < 0 || (maxSelect > 0 && minSelect > maxSelect) {
			return nil, fmt.Errorf("option group %q has invalid selection limits", groupID)
		}
		if selectionCount < minSelect {
			return nil, fmt.Errorf(
				"option group %q selection count must be at least %d; got %d",
				groupID,
				minSelect,
				selectionCount,
			)
		}
		if maxSelect > 0 && selectionCount > maxSelect {
			return nil, fmt.Errorf(
				"option group %q selection count must be at most %d; got %d",
				groupID,
				maxSelect,
				selectionCount,
			)
		}
		if len(valueCounts) == 0 {
			continue
		}
		valueIDs := make([]string, 0, len(valueCounts))
		for valueID := range valueCounts {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)

		values := make([]any, 0, len(valueIDs))
		for _, valueID := range valueIDs {
			price := 0
			if valueSpec, ok := groupSpec.Values[valueID]; ok {
				if !valueSpec.HasPrice {
					return nil, fmt.Errorf(
						"option group %q value %q is missing current price metadata; refresh venue item data and try again",
						groupID,
						valueID,
					)
				}
				price = valueSpec.Price
			}
			values = append(values, map[string]any{
				"id":    valueID,
				"count": valueCounts[valueID],
				"price": price,
			})
		}
		options = append(options, map[string]any{
			"id":     groupID,
			"values": values,
		})
	}
	return options, nil
}

func extractOptionSpecs(payload map[string]any) map[string]payloadutil.OptionGroupSpec {
	return payloadutil.ExtractOptionSpecs(payload)
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
		if !strings.EqualFold(candidateID, targetItemID) {
			continue
		}
		item = candidate
		break
	}
	if item == nil {
		return nil
	}

	priceAmount := payloadutil.MinorAmount(item["price"])
	if priceAmount <= 0 {
		priceAmount = payloadutil.MinorAmount(item["base_price"])
	}
	currency := strings.TrimSpace(asString(coalesceAny(
		item["currency"],
		asMap(item["price"])["currency"],
		asMap(asMap(assortment["venue"])["price"])["currency"],
		asMap(assortment["venue"])["currency"],
	)))
	if currency == "" {
		currency = payloadutil.CurrencyFromVenuePayload(assortment)
	}

	optionGroupIDs := extractAssortmentOptionGroupIDs(item)
	optionGroupIndex := map[string]map[string]any{}
	rootOptionGroups := payloadutil.MergeOptionGroups(
		asSlice(assortment["options"]),
		asSlice(assortment["option_groups"]),
	)
	for _, rawGroup := range rootOptionGroups {
		group := asMap(rawGroup)
		if group == nil {
			continue
		}
		groupID := strings.TrimSpace(asString(coalesceAny(group["id"], group["option_id"], group["group_id"])))
		if groupID != "" {
			optionGroupIndex[strings.ToLower(groupID)] = group
		}
	}
	optionGroups := make([]any, 0, len(optionGroupIDs))
	for _, groupID := range optionGroupIDs {
		if group, ok := optionGroupIndex[strings.ToLower(groupID)]; ok {
			optionGroups = append(optionGroups, group)
		}
	}
	if len(optionGroups) == 0 {
		optionGroups = asSlice(coalesceAny(item["option_groups"], item["options"]))
	}

	result := map[string]any{
		"item_id": targetItemID,
		"id":      targetItemID,
	}
	if priceAmount > 0 {
		price := map[string]any{"amount": priceAmount}
		if currency != "" {
			price["currency"] = currency
		}
		result["price"] = price
		result["base_price"] = price
	}
	if name := strings.TrimSpace(asString(coalesceAny(item["name"], item["title"]))); name != "" {
		result["name"] = name
	}
	if description := strings.TrimSpace(asString(item["description"])); description != "" {
		result["description"] = description
	}
	if len(optionGroups) > 0 {
		result["option_groups"] = optionGroups
		result["options"] = optionGroups
	}
	for _, key := range []string{
		"images",
		"disabled_info",
		"purchasable_balance",
		"unit_info",
		"unit_price",
		"sell_by_weight_config",
		"available_times",
	} {
		if value, exists := item[key]; exists {
			result[key] = value
		}
	}
	return result
}

func extractAssortmentOptionGroupIDs(item map[string]any) []string {
	groupIDs := []string{}
	seen := map[string]struct{}{}
	appendGroupID := func(raw any) {
		groupID := strings.TrimSpace(asString(raw))
		key := strings.ToLower(groupID)
		if groupID == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		groupIDs = append(groupIDs, groupID)
	}
	for _, value := range asSlice(item["option_group_ids"]) {
		appendGroupID(value)
	}
	for _, alias := range []string{"options", "option_groups"} {
		for _, optionValue := range asSlice(item[alias]) {
			option := asMap(optionValue)
			if option == nil {
				continue
			}
			appendGroupID(coalesceAny(option["option_id"], option["id"], option["group_id"]))
		}
	}
	return groupIDs
}

func inferCurrency(formatted string) string {
	return payloadutil.InferCurrency(formatted)
}

func formatMinorAmount(amount int, currency string) string {
	return payloadutil.FormatMinorAmount(amount, currency)
}

func resolveOptionGroupToken(
	token string,
	specs map[string]payloadutil.OptionGroupSpec,
) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("option group is required")
	}
	if _, ok := specs[token]; ok {
		return token, nil
	}
	groupIDs := make([]string, 0, len(specs))
	for groupID := range specs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	if match, ambiguous := uniqueOptionMatch(token, groupIDs, func(groupID string) string {
		return groupID
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option group %q; use its ID", token)
		}
		return match, nil
	}
	if match, ambiguous := uniqueOptionMatch(token, groupIDs, func(groupID string) string {
		return specs[groupID].Name
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option group %q; use its ID", token)
		}
		return match, nil
	}
	return "", fmt.Errorf("unknown option group %q", token)
}

func resolveOptionValueToken(
	token string,
	group payloadutil.OptionGroupSpec,
) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("option value is required")
	}
	if _, ok := group.Values[token]; ok {
		return token, nil
	}
	valueIDs := make([]string, 0, len(group.Values))
	for valueID := range group.Values {
		valueIDs = append(valueIDs, valueID)
	}
	sort.Strings(valueIDs)
	if match, ambiguous := uniqueOptionMatch(token, valueIDs, func(valueID string) string {
		return valueID
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option value %q; use its ID", token)
		}
		return match, nil
	}
	if match, ambiguous := uniqueOptionMatch(token, valueIDs, func(valueID string) string {
		return group.Values[valueID].Name
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option value %q; use its ID", token)
		}
		return match, nil
	}
	return "", fmt.Errorf("unknown option value %q", token)
}

func uniqueOptionMatch(
	token string,
	ids []string,
	label func(string) string,
) (string, bool) {
	match := ""
	for _, id := range ids {
		if !strings.EqualFold(strings.TrimSpace(label(id)), token) {
			continue
		}
		if match != "" && match != id {
			return "", true
		}
		match = id
	}
	return match, false
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
