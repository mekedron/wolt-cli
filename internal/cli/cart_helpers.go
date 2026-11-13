package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/itemoptions"
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

type optionSelection = itemoptions.Choice

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
	result, err := itemoptions.Build(itemPayload, selections)
	if err != nil {
		return nil, err
	}
	return result.Options, nil
}

// basketOptionsTotal sums price * count over the selected option values.
func basketOptionsTotal(options []any) int {
	return itemoptions.Total(options)
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

	optionGroups := payloadutil.JoinOptionLibrary(item, payloadutil.MergeOptionGroups(
		asSlice(assortment["options"]),
		asSlice(assortment["option_groups"]),
	))

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

func inferCurrency(formatted string) string {
	return payloadutil.InferCurrency(formatted)
}

func formatMinorAmount(amount int, currency string) string {
	return payloadutil.FormatMinorAmount(amount, currency)
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
