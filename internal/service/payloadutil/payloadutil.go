package payloadutil

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
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
	ID       string
	Name     string
	Price    int
	HasPrice bool
}

type OptionGroupSpec struct {
	ID        string
	Name      string
	Required  bool
	MinSelect int
	MaxSelect int
	Values    map[string]OptionValueSpec
}

// BasketVenueIdentity is the venue identity carried by a basket payload.
// Wolt has emitted both nested venue fields and top-level compatibility fields.
type BasketVenueIdentity struct {
	ID       string
	Slug     string
	Conflict bool
}

// BasketID returns the canonical identifier from supported basket payload
// shapes.
func BasketID(basket map[string]any) string {
	return strings.TrimSpace(String(CoalesceAny(basket["id"], basket["basket_id"])))
}

// BasketRows returns basket objects from the supported page containers.
func BasketRows(page map[string]any) []map[string]any {
	rows := []map[string]any{}
	seenIDs := map[string]struct{}{}
	for _, key := range []string{"baskets", "results"} {
		for _, raw := range Slice(page[key]) {
			basket := Map(raw)
			if basket == nil {
				continue
			}
			id := strings.ToLower(BasketID(basket))
			if id != "" {
				if _, duplicate := seenIDs[id]; duplicate {
					continue
				}
				seenIDs[id] = struct{}{}
			}
			rows = append(rows, basket)
		}
	}
	return rows
}

// BasketRowsForMutation validates every supported basket-page container before
// a caller uses the page to replace or delete basket state. Read-only callers
// can continue to use the tolerant BasketRows helper.
func BasketRowsForMutation(page map[string]any) ([]map[string]any, error) {
	rows, err := strictBasketRows(page)
	if err != nil {
		return nil, err
	}
	for index, basket := range rows {
		identity := ExtractBasketVenueIdentity(basket)
		if identity.Conflict {
			return nil, fmt.Errorf("basket at index %d has conflicting venue identities", index)
		}
	}

	out := make([]map[string]any, 0, len(rows))
	seenIDs := map[string]map[string]any{}
	for _, basket := range rows {
		id := strings.ToLower(BasketID(basket))
		if id != "" {
			if existing, duplicate := seenIDs[id]; duplicate {
				if !reflect.DeepEqual(existing, basket) {
					return nil, fmt.Errorf("basket %q has conflicting duplicate snapshots", BasketID(basket))
				}
				continue
			}
			seenIDs[id] = basket
		}
		out = append(out, basket)
	}
	return out, nil
}

func strictBasketRows(page map[string]any) ([]map[string]any, error) {
	if page == nil {
		return nil, fmt.Errorf("basket page is unavailable")
	}
	rows := []map[string]any{}
	foundContainer := false
	for _, key := range []string{"baskets", "results"} {
		raw, exists := page[key]
		if !exists {
			continue
		}
		foundContainer = true
		values, ok := strictSlice(raw)
		if !ok {
			return nil, fmt.Errorf("basket page field %q must be an array", key)
		}
		for index, value := range values {
			basket := Map(value)
			if basket == nil {
				return nil, fmt.Errorf("basket page field %q entry %d must be an object", key, index)
			}
			rows = append(rows, basket)
		}
	}
	if !foundContainer {
		return nil, fmt.Errorf("basket page has no supported basket array")
	}
	return rows, nil
}

// BasketIDs returns unique non-empty basket identifiers from a page.
func BasketIDs(page map[string]any) []string {
	rows := BasketRows(page)
	ids := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, basket := range rows {
		id := BasketID(basket)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// BasketIDsComplete reports whether every enumerated basket has a usable ID.
func BasketIDsComplete(page map[string]any) bool {
	rows, err := strictBasketRows(page)
	if err != nil {
		return false
	}
	for _, basket := range rows {
		if BasketID(basket) == "" {
			return false
		}
	}
	return true
}

// ExtractBasketVenueIdentity accepts every basket identity shape currently
// emitted by Wolt without making callers depend on one response layout.
func ExtractBasketVenueIdentity(basket map[string]any) BasketVenueIdentity {
	venue := Map(basket["venue"])
	nestedID := strings.TrimSpace(String(venue["id"]))
	topLevelID := strings.TrimSpace(String(basket["venue_id"]))
	nestedObjectID := domain.NormalizeObjectID(nestedID)
	topLevelObjectID := domain.NormalizeObjectID(topLevelID)

	identity := BasketVenueIdentity{
		Slug: strings.TrimSpace(String(CoalesceAny(
			venue["slug"],
			venue["venue_slug"],
			venue["public_slug"],
			venue["url_slug"],
			basket["venue_slug"],
		))),
	}
	switch {
	case nestedObjectID != "" && topLevelObjectID != "" &&
		!strings.EqualFold(nestedObjectID, topLevelObjectID):
		identity.Conflict = true
	case nestedObjectID != "":
		identity.ID = nestedObjectID
	case topLevelObjectID != "":
		identity.ID = topLevelObjectID
	default:
		// Preserve compatibility with older non-canonical response fixtures.
		// Mutation callers independently require a canonical ObjectID.
		identity.ID = strings.TrimSpace(String(CoalesceAny(nestedID, topLevelID)))
	}
	return identity
}

// CheckedAddInt returns false when an integer addition would overflow.
func CheckedAddInt(left, right int) (int, bool) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	if (right > 0 && left > maxInt-right) ||
		(right < 0 && left < minInt-right) {
		return 0, false
	}
	return left + right, true
}

// CheckedMultiplyInt returns false when an integer multiplication would
// overflow.
func CheckedMultiplyInt(left, right int) (int, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	minInt := -int(^uint(0)>>1) - 1
	if (left == -1 && right == minInt) ||
		(right == -1 && left == minInt) {
		return 0, false
	}
	product := left * right
	if product/right != left {
		return 0, false
	}
	return product, true
}

// BasketItemRowsForMutation returns every basket item only when the items
// container and item identities are safe to use for a state replacement.
func BasketItemRowsForMutation(basket map[string]any) ([]map[string]any, error) {
	raw, exists := basket["items"]
	if !exists {
		return nil, fmt.Errorf("basket items are unavailable")
	}
	values, ok := strictSlice(raw)
	if !ok {
		return nil, fmt.Errorf("basket items must be an array")
	}
	rows := make([]map[string]any, 0, len(values))
	for index, value := range values {
		line := Map(value)
		if line == nil {
			return nil, fmt.Errorf("basket item at index %d must be an object", index)
		}
		if strings.TrimSpace(String(line["id"])) == "" {
			return nil, fmt.Errorf("basket item at index %d has no item id", index)
		}
		rows = append(rows, line)
	}
	return rows, nil
}

// BuildBasketUpsertItem converts one basket response line back into the compact
// item shape accepted by the basket replacement endpoint. It rejects
// incomplete state instead of silently dropping fields during reconstruction.
func BuildBasketUpsertItem(line map[string]any, count int) (map[string]any, error) {
	if line == nil {
		return nil, fmt.Errorf("basket item must be an object")
	}
	itemID := strings.TrimSpace(String(line["id"]))
	if itemID == "" {
		return nil, fmt.Errorf("basket item id is unavailable")
	}
	if count <= 0 {
		return nil, fmt.Errorf("basket item %q count must be greater than zero", itemID)
	}
	if _, err := basketMutationCount(line, "basket item"); err != nil {
		return nil, fmt.Errorf("basket item %q: %w", itemID, err)
	}
	price, hasPrice := optionPrice(line)
	if !hasPrice || price <= 0 {
		return nil, fmt.Errorf("basket item %q has no positive current price", itemID)
	}

	rawOptions := []any{}
	if raw, exists := line["options"]; exists && raw != nil {
		var ok bool
		rawOptions, ok = strictSlice(raw)
		if !ok {
			return nil, fmt.Errorf("basket item %q options must be an array", itemID)
		}
	}
	options := make([]any, 0, len(rawOptions))
	for optionIndex, rawOption := range rawOptions {
		option := Map(rawOption)
		if option == nil {
			return nil, fmt.Errorf("basket item %q option %d must be an object", itemID, optionIndex)
		}
		optionID := strings.TrimSpace(String(option["id"]))
		if optionID == "" {
			return nil, fmt.Errorf("basket item %q option %d has no option id", itemID, optionIndex)
		}
		rawValues, ok := strictSlice(option["values"])
		if !ok {
			return nil, fmt.Errorf("basket item %q option %q values must be an array", itemID, optionID)
		}
		values := make([]any, 0, len(rawValues))
		for valueIndex, rawValue := range rawValues {
			value := Map(rawValue)
			if value == nil {
				return nil, fmt.Errorf(
					"basket item %q option %q value %d must be an object",
					itemID,
					optionID,
					valueIndex,
				)
			}
			valueID := strings.TrimSpace(String(value["id"]))
			if valueID == "" {
				return nil, fmt.Errorf("basket item %q option %q has a value without an id", itemID, optionID)
			}
			valueCount, err := basketMutationCount(value, "option value")
			if err != nil {
				return nil, fmt.Errorf("basket item %q option %q value %q: %w", itemID, optionID, valueID, err)
			}
			valuePrice, valueHasPrice := optionPrice(value)
			if !valueHasPrice || valuePrice < 0 {
				return nil, fmt.Errorf(
					"basket item %q option %q value %q has no current price",
					itemID,
					optionID,
					valueID,
				)
			}
			values = append(values, map[string]any{
				"id":    valueID,
				"count": valueCount,
				"price": valuePrice,
			})
		}
		options = append(options, map[string]any{
			"id":     optionID,
			"values": values,
		})
	}

	allowSubstitutions := false
	if raw, exists := line["substitution_settings"]; exists && raw != nil {
		settings := Map(raw)
		if settings == nil {
			return nil, fmt.Errorf("basket item %q substitution_settings must be an object", itemID)
		}
		if rawAllowed, exists := settings["is_allowed"]; exists {
			allowed, ok := rawAllowed.(bool)
			if !ok {
				return nil, fmt.Errorf("basket item %q substitution setting is_allowed must be boolean", itemID)
			}
			allowSubstitutions = allowed
		}
	}

	item := map[string]any{
		"id":      itemID,
		"count":   count,
		"name":    String(line["name"]),
		"price":   price,
		"options": options,
		"substitution_settings": map[string]any{
			"is_allowed": allowSubstitutions,
		},
	}
	if info := basketWeightedItemInfo(line); info != nil {
		item["weighted_item_info"] = info
	}
	return item, nil
}

// basketWeightedItemInfo returns the weighted selection carried by a basket
// line. Basket responses report count and purchased weight but omit
// weighted_item_input_type, so the input type is echoed only when the caller
// resolved it from the catalog.
func basketWeightedItemInfo(line map[string]any) map[string]any {
	info := Map(line["weighted_item_info"])
	count := Int(info["count"])
	grams := Int(info["purchased_weight_in_grams"])
	if count <= 0 || grams <= 0 {
		return nil
	}
	weighted := map[string]any{
		"count":                     count,
		"purchased_weight_in_grams": grams,
	}
	inputType := strings.TrimSpace(String(info["weighted_item_input_type"]))
	if inputType == WeightedInputGrams || inputType == WeightedInputNumberOfItems {
		weighted["weighted_item_input_type"] = inputType
	}
	return weighted
}

// MergeBasketItems preserves every existing line while adding or incrementing
// one item. The upstream mutation replaces the complete items array.
func MergeBasketItems(basket map[string]any, addedItemID string, addedCount int, newLine map[string]any) ([]any, error) {
	if addedCount <= 0 {
		return nil, fmt.Errorf("item count must be greater than zero")
	}
	target := strings.TrimSpace(addedItemID)
	if target == "" {
		return nil, fmt.Errorf("item id is required")
	}
	existing, err := basketReplacementItems(basket)
	if err != nil {
		return nil, err
	}
	replacement, err := BuildBasketUpsertItem(newLine, addedCount)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(String(replacement["id"]), target) {
		return nil, fmt.Errorf("replacement item id does not match the requested item")
	}
	out := make([]any, 0, len(existing)+1)
	merged := false
	for _, line := range existing {
		lineCount := Int(line["count"])
		if !merged &&
			strings.EqualFold(strings.TrimSpace(String(line["id"])), target) &&
			basketLineConfigurationEqual(line, replacement) {
			mergedCount, ok := CheckedAddInt(lineCount, addedCount)
			if !ok {
				return nil, fmt.Errorf("item count exceeds the supported integer range")
			}
			mergedLine, buildErr := BuildBasketUpsertItem(replacement, mergedCount)
			if buildErr != nil {
				return nil, buildErr
			}
			out = append(out, mergedLine)
			merged = true
			continue
		}
		out = append(out, line)
	}
	if !merged {
		out = append(out, replacement)
	}
	return out, nil
}

// RemoveBasketItems returns the complete replacement array after removing a
// quantity. A non-positive count removes every matching item quantity. It
// fails without returning partial state when the aggregate count overflows.
func RemoveBasketItems(basket map[string]any, itemID string, count int) ([]any, int, error) {
	target := strings.TrimSpace(itemID)
	if target == "" {
		return nil, 0, fmt.Errorf("item id is required")
	}
	existing, err := basketReplacementItems(basket)
	if err != nil {
		return nil, 0, err
	}
	remaining := make([]any, 0, len(existing))
	removed := 0
	removeAll := count <= 0
	remainingCount := count
	for _, line := range existing {
		lineCount := Int(line["count"])
		if !strings.EqualFold(strings.TrimSpace(String(line["id"])), target) {
			remaining = append(remaining, line)
			continue
		}
		if !removeAll && remainingCount <= 0 {
			remaining = append(remaining, line)
			continue
		}
		if removeAll || remainingCount >= lineCount {
			nextRemoved, ok := CheckedAddInt(removed, lineCount)
			if !ok {
				return nil, 0, fmt.Errorf("removed item count exceeds the supported integer range")
			}
			removed = nextRemoved
			if !removeAll {
				remainingCount -= lineCount
			}
			continue
		}
		if basketWeightedItemInfo(line) != nil {
			return nil, 0, fmt.Errorf("weighted item %q cannot be partially removed without current catalog pricing", target)
		}
		nextRemoved, ok := CheckedAddInt(removed, remainingCount)
		if !ok {
			return nil, 0, fmt.Errorf("removed item count exceeds the supported integer range")
		}
		removed = nextRemoved
		remainder, buildErr := BuildBasketUpsertItem(line, lineCount-remainingCount)
		if buildErr != nil {
			return nil, 0, buildErr
		}
		remaining = append(remaining, remainder)
		remainingCount = 0
	}
	return remaining, removed, nil
}

func basketReplacementItems(basket map[string]any) ([]map[string]any, error) {
	rows, err := BasketItemRowsForMutation(basket)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, line := range rows {
		count, countErr := basketMutationCount(line, "basket item")
		if countErr != nil {
			return nil, countErr
		}
		item, buildErr := BuildBasketUpsertItem(line, count)
		if buildErr != nil {
			return nil, buildErr
		}
		items = append(items, item)
	}
	return items, nil
}

func basketMutationCount(value map[string]any, label string) (int, error) {
	raw, exists := value["count"]
	if !exists || raw == nil {
		return 1, nil
	}
	count := Int(raw)
	if count <= 0 {
		return 0, fmt.Errorf("%s count must be greater than zero", label)
	}
	return count, nil
}

func basketLineConfigurationEqual(left map[string]any, right map[string]any) bool {
	leftSelections, leftOK := basketOptionSelections(left)
	rightSelections, rightOK := basketOptionSelections(right)
	return leftOK && rightOK &&
		reflect.DeepEqual(leftSelections, rightSelections) &&
		Bool(Map(left["substitution_settings"])["is_allowed"]) ==
			Bool(Map(right["substitution_settings"])["is_allowed"])
}

func basketOptionSelections(line map[string]any) (map[string]map[string]int, bool) {
	selections := map[string]map[string]int{}
	for _, rawOption := range Slice(line["options"]) {
		option := Map(rawOption)
		if option == nil {
			continue
		}
		groupID := strings.TrimSpace(String(option["id"]))
		values := selections[groupID]
		if values == nil {
			values = map[string]int{}
			selections[groupID] = values
		}
		for _, rawValue := range Slice(option["values"]) {
			value := Map(rawValue)
			if value == nil {
				continue
			}
			valueCount := Int(value["count"])
			if valueCount <= 0 {
				valueCount = 1
			}
			valueID := strings.TrimSpace(String(value["id"]))
			total, ok := CheckedAddInt(values[valueID], valueCount)
			if !ok {
				return nil, false
			}
			values[valueID] = total
		}
	}
	return selections, true
}

func ExtractOptionSpecs(payload map[string]any) map[string]OptionGroupSpec {
	specs := map[string]OptionGroupSpec{}
	visitOptionGroupCandidates(payload, func(group map[string]any) {
		groupID := strings.TrimSpace(String(CoalesceAny(
			group["id"],
			group["group_id"],
			group["option_id"],
		)))
		if groupID == "" {
			return
		}
		canonicalGroupID := equalFoldMapKey(specs, groupID)
		if canonicalGroupID == "" {
			canonicalGroupID = groupID
		}

		spec := specs[canonicalGroupID]
		if spec.ID == "" {
			spec.ID = canonicalGroupID
			spec.Values = map[string]OptionValueSpec{}
		}
		if spec.Name == "" {
			spec.Name = String(CoalesceAny(group["name"], group["title"]))
		}
		spec.Required = spec.Required || Bool(group["required"])
		if spec.MinSelect == 0 {
			spec.MinSelect = Int(CoalesceAny(group["min"], group["minimum"], group["min_select"]))
		}
		if spec.MaxSelect == 0 {
			spec.MaxSelect = Int(CoalesceAny(group["max"], group["maximum"], group["max_select"]))
		}

		for _, alias := range []string{"values", "options", "items"} {
			for _, value := range Slice(group[alias]) {
				valueMap := Map(value)
				if valueMap == nil {
					continue
				}
				valueID := strings.TrimSpace(String(CoalesceAny(valueMap["id"], valueMap["value_id"])))
				if valueID == "" {
					continue
				}
				price, hasPrice := optionPrice(valueMap)
				canonicalValueID := equalFoldMapKey(spec.Values, valueID)
				if canonicalValueID == "" {
					canonicalValueID = valueID
				}
				valueSpec, exists := spec.Values[canonicalValueID]
				if !exists {
					valueSpec = OptionValueSpec{ID: canonicalValueID}
				}
				if valueSpec.Name == "" {
					valueSpec.Name = String(CoalesceAny(valueMap["name"], valueMap["title"]))
				}
				if !valueSpec.HasPrice && hasPrice {
					valueSpec.Price = price
					valueSpec.HasPrice = true
				}
				spec.Values[canonicalValueID] = valueSpec
			}
		}
		specs[canonicalGroupID] = spec
	})
	return specs
}

func equalFoldMapKey[V any](values map[string]V, target string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.EqualFold(key, target) {
			return key
		}
	}
	return ""
}

// MergeOptionGroups preserves first-seen group order while allowing a richer
// duplicate definition to replace an ID-only or otherwise incomplete one.
func MergeOptionGroups(existing []any, incoming []any) []any {
	out := append([]any(nil), existing...)
	indexByID := make(map[string]int, len(existing)+len(incoming))
	for index, raw := range existing {
		if id := optionGroupID(raw); id != "" {
			indexByID[strings.ToLower(id)] = index
		}
	}
	for _, raw := range incoming {
		id := optionGroupID(raw)
		if id == "" {
			out = append(out, raw)
			continue
		}
		key := strings.ToLower(id)
		if index, exists := indexByID[key]; exists {
			if optionGroupScore(raw) > optionGroupScore(out[index]) {
				out[index] = raw
			}
			continue
		}
		indexByID[key] = len(out)
		out = append(out, raw)
	}
	return out
}

func optionGroupID(raw any) string {
	group := Map(raw)
	return strings.TrimSpace(String(CoalesceAny(
		group["id"],
		group["group_id"],
		group["option_id"],
	)))
}

func optionGroupScore(raw any) int {
	group := Map(raw)
	if group == nil {
		return 0
	}
	score := 0
	if strings.TrimSpace(String(CoalesceAny(group["name"], group["title"]))) != "" {
		score++
	}
	for _, key := range []string{"required", "min", "minimum", "min_select", "max", "maximum", "max_select"} {
		if _, exists := group[key]; exists {
			score++
		}
	}
	for _, alias := range []string{"values", "options", "items"} {
		for _, rawValue := range Slice(group[alias]) {
			value := Map(rawValue)
			if value == nil {
				continue
			}
			if strings.TrimSpace(String(CoalesceAny(value["id"], value["value_id"]))) != "" {
				score += 2
			}
			if strings.TrimSpace(String(CoalesceAny(value["name"], value["title"]))) != "" {
				score++
			}
			if _, hasPrice := optionPrice(value); hasPrice {
				score++
			}
		}
	}
	return score
}

func optionPrice(value map[string]any) (int, bool) {
	raw, exists := value["price"]
	if !exists || raw == nil {
		return 0, false
	}
	if price := Map(raw); price != nil {
		raw, exists = price["amount"]
		if !exists || raw == nil {
			return 0, false
		}
	}
	switch raw.(type) {
	case int, int64, float64, float32:
		return Int(raw), true
	default:
		return 0, false
	}
}

func visitOptionGroupCandidates(payload map[string]any, visit func(map[string]any)) {
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for _, alias := range []string{"options", "option_groups"} {
				groups := Slice(typed[alias])
				for _, groupValue := range groups {
					group := Map(groupValue)
					if group == nil {
						continue
					}
					if strings.TrimSpace(String(CoalesceAny(
						group["id"],
						group["group_id"],
						group["option_id"],
					))) != "" {
						visit(group)
					}
				}
			}
			keys := make([]string, 0, len(typed))
			for key := range typed {
				if key != "options" &&
					key != "option_groups" &&
					!isRelatedItemContainer(key) {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		case []any:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(payload)
}

func isRelatedItemContainer(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "upsell_items", "related_items", "recommended_items":
		return true
	default:
		return false
	}
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

func strictSlice(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil, false
	}
	kind := rv.Kind()
	if kind != reflect.Slice && kind != reflect.Array {
		return nil, false
	}
	if kind == reflect.Slice && rv.IsNil() {
		return nil, false
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil, false
	}
	return Slice(value), true
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

// FormatMinorAmount renders a minor-unit amount for human-facing output. The
// two currencies with a universally recognised glyph get it; everything else is
// prefixed with its ISO code, which is unambiguous across the markets Wolt
// serves. An empty currency yields "" — an amount without a currency is not
// presentable.
func FormatMinorAmount(amount int, currency string) string {
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

// MinorAmount accepts the scalar and {"amount": N} price shapes used by Wolt.
func MinorAmount(value any) int {
	if price := Map(value); price != nil {
		return Int(price["amount"])
	}
	return Int(value)
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
