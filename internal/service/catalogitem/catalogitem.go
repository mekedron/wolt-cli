package catalogitem

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// Availability describes whether Wolt currently allows an item to be
// purchased. Current Wolt assortment payloads use disabled_info as the
// authoritative switch; purchasable_balance is an additional inventory guard.
type Availability struct {
	IsAvailable        bool
	Reason             string
	PurchasableBalance any
}

// ValidationIssue identifies a basket item that cannot safely be purchased.
type ValidationIssue struct {
	ItemID string
	Name   string
	Reason string
}

const missingCurrentAssortmentReason = "item is missing from the current assortment"

// ResolveAvailability interprets the current consumer-assortment item schema.
// A missing or nil disabled_info means enabled. Any present non-nil value means
// disabled; object payloads may additionally carry a human-readable reason.
func ResolveAvailability(item map[string]any) Availability {
	result := Availability{
		IsAvailable:        true,
		PurchasableBalance: item["purchasable_balance"],
	}

	if rawDisabledInfo, exists := item["disabled_info"]; exists && rawDisabledInfo != nil {
		result.IsAvailable = false
		if disabledInfo := payloadutil.Map(rawDisabledInfo); disabledInfo != nil {
			result.Reason = strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
				disabledInfo["disable_text"],
				disabledInfo["disabled_reason"],
				disabledInfo["disable_reason"],
			)))
		}
		if result.Reason == "" {
			result.Reason = "item is disabled by Wolt"
		}
		return result
	}

	if balance, ok := number(item["purchasable_balance"]); ok && balance <= 0 {
		result.IsAvailable = false
		result.Reason = "purchasable balance is zero"
	}
	return result
}

// Find locates one item in an assortment or item-selection payload.
func Find(payload map[string]any, itemID string) map[string]any {
	target := strings.TrimSpace(itemID)
	if target == "" || payload == nil {
		return nil
	}

	var found map[string]any
	bestScore := -1
	var walk func(any, bool)
	walk = func(value any, itemContainer bool) {
		switch typed := value.(type) {
		case map[string]any:
			id := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
				typed["item_id"],
				typed["id"],
			)))
			if strings.EqualFold(id, target) && isItemCandidate(typed, itemContainer) {
				if score := itemCandidateScore(typed); score > bestScore {
					found = typed
					bestScore = score
				}
			}
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key], isItemContainerKey(key))
			}
		case []any:
			for _, nested := range typed {
				walk(nested, itemContainer)
			}
		}
	}
	walk(payload, false)
	return found
}

func isItemCandidate(item map[string]any, itemContainer bool) bool {
	if itemContainer {
		return true
	}
	if _, exists := item["item_id"]; exists {
		return true
	}
	for _, key := range []string{
		"base_price",
		"price",
		"description",
		"disabled_info",
		"purchasable_balance",
		"images",
		"option_group_ids",
		"sell_by_weight_config",
		"unit_info",
		"unit_price",
	} {
		if _, exists := item[key]; exists {
			return true
		}
	}
	return false
}

func isItemContainerKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "item", "items", "menu_items", "products":
		return true
	default:
		return false
	}
}

func itemCandidateScore(item map[string]any) int {
	score := 0
	for _, key := range []string{
		"item_id",
		"price",
		"base_price",
		"description",
		"disabled_info",
		"purchasable_balance",
		"images",
		"options",
		"option_groups",
	} {
		if value, exists := item[key]; exists && value != nil {
			score++
		}
	}
	return score
}

// MarkMissingFromCurrentAssortment preserves any older item metadata while
// making an authoritative exact-item omission visible to read-only callers.
func MarkMissingFromCurrentAssortment(payload map[string]any, itemID string) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}
	target := Find(payload, itemID)
	rootID := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		payload["item_id"],
		payload["id"],
	)))
	if target == nil && strings.EqualFold(rootID, strings.TrimSpace(itemID)) {
		target = payload
	}
	if target == nil {
		target = map[string]any{
			"item_id": itemID,
			"name":    itemID,
		}
		if len(payload) == 0 {
			payload = target
		} else {
			items := append([]any(nil), payloadutil.Slice(payload["items"])...)
			payload["items"] = append(items, target)
		}
	}
	target["disabled_info"] = map[string]any{
		"disable_text": missingCurrentAssortmentReason,
	}
	target["purchasable_balance"] = 0
	return payload
}

// BasketItemIDs returns unique non-empty item ids while preserving line order.
func BasketItemIDs(basket map[string]any) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, raw := range payloadutil.Slice(basket["items"]) {
		line := payloadutil.Map(raw)
		itemID := strings.TrimSpace(payloadutil.String(line["id"]))
		if itemID == "" {
			continue
		}
		if _, exists := seen[itemID]; exists {
			continue
		}
		seen[itemID] = struct{}{}
		out = append(out, itemID)
	}
	return out
}

// ValidateItemIDs checks a fresh assortment/items response. Missing ids fail
// closed because continuing would repeat the stale-cart behaviour this check
// is meant to prevent.
func ValidateItemIDs(payload map[string]any, itemIDs []string) []ValidationIssue {
	issues := []ValidationIssue{}
	for _, itemID := range itemIDs {
		item := Find(payload, itemID)
		if item == nil {
			issues = append(issues, ValidationIssue{
				ItemID: itemID,
				Name:   itemID,
				Reason: "item is missing from the current assortment",
			})
			continue
		}
		availability := ResolveAvailability(item)
		if availability.IsAvailable {
			continue
		}
		name := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(item["name"], item["title"])))
		if name == "" {
			name = itemID
		}
		issues = append(issues, ValidationIssue{
			ItemID: itemID,
			Name:   name,
			Reason: availability.Reason,
		})
	}
	return issues
}

// FormatValidationIssues produces a concise mutation/checkout error.
func FormatValidationIssues(issues []ValidationIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		label := strings.TrimSpace(issue.Name)
		if label == "" {
			label = strings.TrimSpace(issue.ItemID)
		}
		if reason := strings.TrimSpace(issue.Reason); reason != "" {
			label += ": " + reason
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "; ")
}

// ImageURLs returns all non-empty product image URLs in upstream order.
func ImageURLs(item map[string]any) []string {
	images := imageObjects(item["images"])
	urls := make([]string, 0, len(images))
	seen := map[string]struct{}{}
	for _, image := range images {
		url := strings.TrimSpace(payloadutil.String(image["url"]))
		if url == "" {
			continue
		}
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}
		urls = append(urls, url)
	}
	return urls
}

// ImageBlurhash returns the first non-empty image blurhash.
func ImageBlurhash(item map[string]any) string {
	for _, image := range imageObjects(item["images"]) {
		if blurhash := strings.TrimSpace(payloadutil.String(image["blurhash"])); blurhash != "" {
			return blurhash
		}
	}
	return ""
}

// MergeCurrentItem overlays a fresh assortment item onto an item-page payload.
// Item-page payloads still carry useful option groups, while the exact
// assortment response is authoritative for price, images and availability.
func MergeCurrentItem(base map[string]any, current map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	if current == nil {
		return merged
	}
	for key, value := range current {
		switch key {
		case "disabled_info", "purchasable_balance", "options", "option_groups":
			continue
		case "name", "title", "description":
			if strings.TrimSpace(payloadutil.String(value)) == "" &&
				strings.TrimSpace(payloadutil.String(merged[key])) != "" {
				continue
			}
		case "images":
			if len(imageObjects(value)) == 0 && len(imageObjects(merged[key])) > 0 {
				continue
			}
		case "price", "base_price":
			basePrice := payloadutil.Map(merged[key])
			currentPrice := payloadutil.Map(value)
			if basePrice != nil && currentPrice != nil {
				price := make(map[string]any, len(basePrice)+len(currentPrice))
				for priceKey, priceValue := range basePrice {
					price[priceKey] = priceValue
				}
				for priceKey, priceValue := range currentPrice {
					if text, ok := priceValue.(string); ok && strings.TrimSpace(text) == "" {
						continue
					}
					price[priceKey] = priceValue
				}
				merged[key] = price
				continue
			}
		}
		merged[key] = value
	}

	for _, field := range []string{"disabled_info", "purchasable_balance"} {
		if value, exists := current[field]; exists {
			merged[field] = value
		} else {
			delete(merged, field)
		}
	}

	currentOptionGroups := payloadutil.MergeOptionGroups(
		payloadutil.Slice(current["options"]),
		payloadutil.Slice(current["option_groups"]),
	)
	baseOptionGroups := payloadutil.MergeOptionGroups(
		payloadutil.Slice(base["options"]),
		payloadutil.Slice(base["option_groups"]),
	)
	optionGroups := payloadutil.MergeOptionGroups(currentOptionGroups, baseOptionGroups)
	if len(optionGroups) > 0 {
		merged["options"] = optionGroups
		merged["option_groups"] = optionGroups
	}
	return merged
}

// ScopedItem returns one item enriched only with option definitions that item
// references. Venue-wide option groups and unrelated items are excluded.
func ScopedItem(payload map[string]any, itemID string) map[string]any {
	target := Find(payload, itemID)
	if target == nil {
		return nil
	}
	scoped := make(map[string]any, len(target)+6)
	for key, value := range target {
		scoped[key] = value
	}
	for _, key := range []string{
		"currency",
		"venue",
		"venue_raw",
		"categories",
		"category",
		"upsell_items",
		"related_items",
		"recommended_items",
	} {
		if _, exists := scoped[key]; !exists {
			if value, ok := payload[key]; ok {
				scoped[key] = value
			}
		}
	}

	referenced := make(map[string]struct{})
	appendReference := func(raw any) {
		id := strings.TrimSpace(payloadutil.String(raw))
		if id != "" {
			referenced[strings.ToLower(id)] = struct{}{}
		}
	}
	for _, rawID := range payloadutil.Slice(target["option_group_ids"]) {
		appendReference(rawID)
	}
	targetGroups := payloadutil.MergeOptionGroups(
		payloadutil.Slice(target["options"]),
		payloadutil.Slice(target["option_groups"]),
	)
	for _, rawGroup := range targetGroups {
		appendReference(optionGroupID(rawGroup))
	}
	groups := append([]any(nil), targetGroups...)
	rootGroups := payloadutil.MergeOptionGroups(
		payloadutil.Slice(payload["options"]),
		payloadutil.Slice(payload["option_groups"]),
	)
	for _, rawGroup := range rootGroups {
		if _, ok := referenced[strings.ToLower(optionGroupID(rawGroup))]; !ok {
			continue
		}
		groups = payloadutil.MergeOptionGroups(groups, []any{rawGroup})
	}
	if len(groups) > 0 {
		scoped["options"] = groups
		scoped["option_groups"] = groups
	}
	return scoped
}

func optionGroupID(raw any) string {
	group := payloadutil.Map(raw)
	return strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
		group["id"],
		group["group_id"],
		group["option_id"],
	)))
}

func imageObjects(value any) []map[string]any {
	if single := payloadutil.Map(value); single != nil {
		return []map[string]any{single}
	}
	out := []map[string]any{}
	for _, raw := range payloadutil.Slice(value) {
		if image := payloadutil.Map(raw); image != nil {
			out = append(out, image)
		}
	}
	return out
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
