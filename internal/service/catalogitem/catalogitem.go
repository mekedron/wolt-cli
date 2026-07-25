package catalogitem

import (
	"encoding/json"
	"fmt"
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

// ResolveAvailability interprets the current consumer-assortment item schema.
// A nil disabled_info means enabled. A present object means disabled even when
// it carries no human-readable reason, matching Wolt's web client.
func ResolveAvailability(item map[string]any) Availability {
	result := Availability{
		IsAvailable:        true,
		PurchasableBalance: item["purchasable_balance"],
	}

	if disabledInfo := payloadutil.Map(item["disabled_info"]); disabledInfo != nil {
		result.IsAvailable = false
		result.Reason = strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
			disabledInfo["disable_text"],
			disabledInfo["disabled_reason"],
			disabledInfo["disable_reason"],
		)))
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
	var walk func(any)
	walk = func(value any) {
		if found != nil {
			return
		}
		switch typed := value.(type) {
		case map[string]any:
			id := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
				typed["item_id"],
				typed["id"],
			)))
			name := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(
				typed["name"],
				typed["title"],
			)))
			if id == target && name != "" {
				found = typed
				return
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
	return found
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
	for key, value := range current {
		merged[key] = value
	}
	return merged
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

// UnavailableMessage creates a stable human-readable mutation error.
func UnavailableMessage(itemID string, item map[string]any, availability Availability) string {
	name := strings.TrimSpace(payloadutil.String(payloadutil.CoalesceAny(item["name"], item["title"])))
	if name == "" {
		name = strings.TrimSpace(itemID)
	}
	if name == "" {
		name = "item"
	}
	if availability.Reason == "" {
		return fmt.Sprintf("%s is unavailable", name)
	}
	return fmt.Sprintf("%s is unavailable: %s", name, availability.Reason)
}
