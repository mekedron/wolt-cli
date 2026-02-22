package cli

import (
	"context"
	"sort"
	"strings"

	woltgateway "github.com/mekedron/wolt-cli/internal/gateway/wolt"
	"github.com/mekedron/wolt-cli/internal/service/observability"
)

const defaultVenueContentPageLimit = 3

func needsVenueContentFallback(assortmentPayload map[string]any, venueID string) bool {
	if len(assortmentPayload) == 0 {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(asString(assortmentPayload["loading_strategy"])), "partial") {
		return true
	}
	return len(observability.ExtractMenuItems(assortmentPayload, venueID, "")) == 0
}

func loadVenueContentPayloads(
	ctx context.Context,
	deps Dependencies,
	venueSlug string,
	auth woltgateway.AuthContext,
	pageLimit int,
) ([]map[string]any, []string) {
	slug := strings.TrimSpace(venueSlug)
	if slug == "" {
		return nil, nil
	}
	if pageLimit <= 0 {
		pageLimit = defaultVenueContentPageLimit
	}

	payloads := make([]map[string]any, 0, pageLimit)
	warnings := []string{}
	seenTokens := map[string]struct{}{}
	nextPageToken := ""

	for page := 0; page < pageLimit; page++ {
		payload, err := deps.Wolt.VenueContentByVenueSlug(ctx, slug, nextPageToken, auth)
		if err != nil && auth.HasCredentials() {
			payload, err = deps.Wolt.VenueContentByVenueSlug(ctx, slug, nextPageToken, woltgateway.AuthContext{})
		}
		if err != nil {
			if page == 0 {
				warnings = append(warnings, "venue content endpoint unavailable")
			}
			break
		}
		if len(payload) == 0 {
			break
		}
		payloads = append(payloads, payload)

		nextToken := strings.TrimSpace(asString(coalesceAny(
			payload["next_page_token"],
			asMap(payload["pagination"])["next_page_token"],
		)))
		if nextToken == "" || nextToken == nextPageToken {
			break
		}
		if _, seen := seenTokens[nextToken]; seen {
			break
		}
		seenTokens[nextToken] = struct{}{}
		nextPageToken = nextToken
	}

	return payloads, warnings
}

func buildItemPayloadFromMenuPayload(payload map[string]any, venueID string, itemID string) map[string]any {
	targetItemID := strings.TrimSpace(itemID)
	if targetItemID == "" || payload == nil {
		return nil
	}

	var menuItem map[string]any
	for _, row := range observability.ExtractMenuItems(payload, venueID, "") {
		if !strings.EqualFold(strings.TrimSpace(asString(row["item_id"])), targetItemID) {
			continue
		}
		menuItem = row
		break
	}
	if menuItem == nil {
		return nil
	}

	basePrice := asMap(menuItem["base_price"])
	priceAmount := asInt(basePrice["amount"])
	currency := strings.TrimSpace(asString(basePrice["currency"]))
	if currency == "" {
		currency = inferCurrency(asString(basePrice["formatted_amount"]))
	}
	if currency == "" {
		currency = "EUR"
	}

	price := map[string]any{
		"amount":   priceAmount,
		"currency": currency,
	}
	optionGroupIDs := toStringSlice(asSlice(menuItem["option_group_ids"]))
	optionGroups := optionGroupsFromPayloadSpecs(payload, optionGroupIDs, currency)

	item := map[string]any{
		"id":               targetItemID,
		"item_id":          targetItemID,
		"name":             menuItem["name"],
		"description":      coalesceAny(menuItem["description"], ""),
		"price":            price,
		"base_price":       price,
		"option_group_ids": optionGroupIDs,
	}

	fallback := map[string]any{
		"id":               targetItemID,
		"item_id":          targetItemID,
		"name":             menuItem["name"],
		"description":      coalesceAny(menuItem["description"], ""),
		"price":            price,
		"base_price":       price,
		"option_group_ids": optionGroupIDs,
		"items":            []any{item},
	}
	if len(optionGroups) > 0 {
		item["option_groups"] = optionGroups
		item["options"] = optionGroups
		fallback["option_groups"] = optionGroups
		fallback["options"] = optionGroups
	}
	return fallback
}

func buildItemPayloadFromMenuPayloads(payloads []map[string]any, venueID string, itemID string) map[string]any {
	for _, payload := range payloads {
		if fallback := buildItemPayloadFromMenuPayload(payload, venueID, itemID); fallback != nil {
			return fallback
		}
	}
	return nil
}

func optionGroupsFromPayloadSpecs(payload map[string]any, groupIDs []string, currency string) []any {
	if len(groupIDs) == 0 {
		return nil
	}

	specs := extractOptionSpecs(payload)
	optionGroups := make([]any, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		spec, ok := specs[groupID]
		if !ok {
			continue
		}
		valueIDs := make([]string, 0, len(spec.Values))
		for valueID := range spec.Values {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)

		values := make([]any, 0, len(valueIDs))
		for _, valueID := range valueIDs {
			valueSpec := spec.Values[valueID]
			values = append(values, map[string]any{
				"id":   valueSpec.ID,
				"name": valueSpec.Name,
				"price": map[string]any{
					"amount":   valueSpec.Price,
					"currency": currency,
				},
			})
		}

		optionGroups = append(optionGroups, map[string]any{
			"id":       spec.ID,
			"name":     spec.Name,
			"required": spec.Required,
			"min":      spec.MinSelect,
			"max":      spec.MaxSelect,
			"values":   values,
		})
	}
	return optionGroups
}
