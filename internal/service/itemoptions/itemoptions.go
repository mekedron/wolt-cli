// Package itemoptions turns option selections for one menu item into the
// option groups of a basket line. The CLI (`wolt cart add --option`) and the
// MCP tool `wolt_cart_add` share it, so both validate and price options the
// same way.
package itemoptions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// Choice is one selected value of an option group.
type Choice struct {
	ValueID string
	Count   int
}

// Selections maps an option group token (item-level id, library id or name)
// to its chosen values (ids or names).
type Selections map[string][]Choice

// AppliedDefault records a required group that had no selection and got the
// value the Wolt app preselects.
type AppliedDefault struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name,omitempty"`
	ValueID   string `json:"value_id"`
	ValueName string `json:"value_name,omitempty"`
	Price     int    `json:"price"`
}

// Result is the basket form of the selected options.
type Result struct {
	// Options are the basket option groups: {id, values: [{id, count, price}]}.
	Options []any
	// UnitTotal is the price of the options for one unit of the item.
	UnitTotal int
	// Defaults lists the required groups filled with their default value.
	Defaults []AppliedDefault
}

// Build validates selections against the item's option groups and returns
// the basket option groups.
//
// A required group (minimum of one) left without a selection gets its default
// value, as the Wolt app preselects it; a required group with no usable
// default is an error. A group with prerequisite values applies only while one
// of them is selected: its limits are skipped otherwise, and selecting a value
// in it is an error. Groups the payload does not describe pass through by id.
func Build(itemPayload map[string]any, selections Selections) (Result, error) {
	specs := payloadutil.ExtractOptionSpecs(itemPayload)
	groupIDs := make([]string, 0, len(specs))
	for groupID := range specs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	tokens := make([]string, 0, len(selections))
	for token := range selections {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)

	resolved := map[string][]Choice{}
	for _, token := range tokens {
		groupID := strings.TrimSpace(token)
		if len(specs) > 0 {
			var err error
			groupID, err = ResolveGroupToken(token, specs)
			if err != nil {
				return Result{}, err
			}
		} else if groupID == "" {
			continue
		}
		resolved[groupID] = append(resolved[groupID], selections[token]...)
	}
	if len(specs) == 0 {
		for groupID := range resolved {
			groupIDs = append(groupIDs, groupID)
		}
		sort.Strings(groupIDs)
	}

	valueCounts := map[string]map[string]int{}
	selectionCounts := map[string]int{}
	for _, groupID := range groupIDs {
		spec := specs[groupID]
		for _, choice := range resolved[groupID] {
			if choice.Count <= 0 {
				return Result{}, fmt.Errorf("option group %q value %q count must be a positive integer", groupID, choice.ValueID)
			}
			nextCount, ok := payloadutil.CheckedAddInt(selectionCounts[groupID], choice.Count)
			if !ok {
				return Result{}, fmt.Errorf("option group %q selection count exceeds the supported integer range", groupID)
			}
			selectionCounts[groupID] = nextCount
			valueID := strings.TrimSpace(choice.ValueID)
			resolvedValueID, err := ResolveValueToken(choice.ValueID, spec)
			if err == nil {
				valueID = resolvedValueID
			} else if len(spec.Values) > 0 || valueID == "" {
				return Result{}, fmt.Errorf("option group %q: %w", groupID, err)
			}
			if valueCounts[groupID] == nil {
				valueCounts[groupID] = map[string]int{}
			}
			nextValueCount, ok := payloadutil.CheckedAddInt(valueCounts[groupID][valueID], choice.Count)
			if !ok {
				return Result{}, fmt.Errorf("option group %q value %q count exceeds the supported integer range", groupID, valueID)
			}
			valueCounts[groupID][valueID] = nextValueCount
		}
	}

	isSelected := func(valueID string) bool {
		for _, values := range valueCounts {
			for selectedID := range values {
				if strings.EqualFold(selectedID, valueID) {
					return true
				}
			}
		}
		return false
	}
	isActive := func(spec payloadutil.OptionGroupSpec) bool {
		if len(spec.PrerequisiteValueIDs) == 0 {
			return true
		}
		for _, valueID := range spec.PrerequisiteValueIDs {
			if isSelected(valueID) {
				return true
			}
		}
		return false
	}

	// A default can activate groups that depend on it, so repeat until no
	// further default applies.
	defaults := []AppliedDefault{}
	for changed := true; changed; {
		changed = false
		for _, groupID := range groupIDs {
			spec := specs[groupID]
			if selectionCounts[groupID] > 0 || minimum(spec) != 1 || !isActive(spec) {
				continue
			}
			valueID := canonicalValueID(spec, spec.DefaultValueID)
			if valueID == "" {
				continue
			}
			valueCounts[groupID] = map[string]int{valueID: 1}
			selectionCounts[groupID] = 1
			value := spec.Values[valueID]
			defaults = append(defaults, AppliedDefault{
				GroupID:   groupID,
				GroupName: spec.Name,
				ValueID:   valueID,
				ValueName: value.Name,
				Price:     value.Price,
			})
			changed = true
		}
	}

	result := Result{Options: make([]any, 0, len(groupIDs)), Defaults: defaults}
	for _, groupID := range groupIDs {
		spec := specs[groupID]
		selectionCount := selectionCounts[groupID]
		label := groupLabel(groupID, spec)
		if !isActive(spec) {
			if selectionCount > 0 {
				return Result{}, fmt.Errorf(
					"option group %s applies only when one of these option values is selected: %s",
					label,
					strings.Join(spec.PrerequisiteValueIDs, ", "),
				)
			}
			continue
		}
		minSelect := minimum(spec)
		maxSelect := spec.MaxSelect
		if minSelect < 0 || maxSelect < 0 || (maxSelect > 0 && minSelect > maxSelect) {
			return Result{}, fmt.Errorf("option group %s has invalid selection limits", label)
		}
		if selectionCount < minSelect {
			return Result{}, fmt.Errorf(
				"option group %s selection count must be at least %d; got %d",
				label,
				minSelect,
				selectionCount,
			)
		}
		if maxSelect > 0 && selectionCount > maxSelect {
			return Result{}, fmt.Errorf(
				"option group %s selection count must be at most %d; got %d",
				label,
				maxSelect,
				selectionCount,
			)
		}
		if len(valueCounts[groupID]) == 0 {
			continue
		}
		valueIDs := make([]string, 0, len(valueCounts[groupID]))
		for valueID := range valueCounts[groupID] {
			valueIDs = append(valueIDs, valueID)
		}
		sort.Strings(valueIDs)

		values := make([]any, 0, len(valueIDs))
		for _, valueID := range valueIDs {
			count := valueCounts[groupID][valueID]
			price := 0
			if valueSpec, ok := spec.Values[valueID]; ok {
				if limit := valueLimit(spec, valueSpec); limit > 0 && count > limit {
					return Result{}, fmt.Errorf(
						"option group %s value %q can be chosen at most %d times; got %d",
						label,
						valueID,
						limit,
						count,
					)
				}
				if !valueSpec.HasPrice {
					return Result{}, fmt.Errorf(
						"option group %q value %q is missing current price metadata; refresh venue item data and try again",
						groupID,
						valueID,
					)
				}
				price = valueSpec.Price
			}
			valueTotal, ok := payloadutil.CheckedMultiplyInt(price, count)
			if !ok {
				return Result{}, fmt.Errorf("option group %q value %q total exceeds the supported integer range", groupID, valueID)
			}
			result.UnitTotal, ok = payloadutil.CheckedAddInt(result.UnitTotal, valueTotal)
			if !ok {
				return Result{}, fmt.Errorf("option total exceeds the supported integer range")
			}
			values = append(values, map[string]any{
				"id":    valueID,
				"count": count,
				"price": price,
			})
		}
		result.Options = append(result.Options, map[string]any{
			"id":     groupID,
			"values": values,
		})
	}
	return result, nil
}

// DefaultConfigurationTotal is the price of the options the Wolt app
// preselects for one unit: the default values of the required groups that
// apply without any other choice. The second result is false when a required
// group has no usable default, so no complete configuration exists without an
// explicit choice.
func DefaultConfigurationTotal(itemPayload map[string]any) (int, bool) {
	result, err := Build(itemPayload, nil)
	if err != nil {
		return 0, false
	}
	return result.UnitTotal, true
}

// DefaultsData renders applied defaults as plain maps for JSON and YAML output.
func DefaultsData(defaults []AppliedDefault) []any {
	out := make([]any, 0, len(defaults))
	for _, entry := range defaults {
		row := map[string]any{
			"group_id": entry.GroupID,
			"value_id": entry.ValueID,
			"price":    entry.Price,
		}
		if entry.GroupName != "" {
			row["group_name"] = entry.GroupName
		}
		if entry.ValueName != "" {
			row["value_name"] = entry.ValueName
		}
		out = append(out, row)
	}
	return out
}

// Total sums price x count over the values of basket option groups.
func Total(options []any) int {
	total := 0
	for _, rawOption := range options {
		for _, rawValue := range payloadutil.Slice(payloadutil.Map(rawOption)["values"]) {
			value := payloadutil.Map(rawValue)
			total += payloadutil.Int(value["price"]) * payloadutil.Int(value["count"])
		}
	}
	return total
}

func minimum(spec payloadutil.OptionGroupSpec) int {
	if spec.Required && spec.MinSelect < 1 {
		return 1
	}
	return spec.MinSelect
}

func valueLimit(group payloadutil.OptionGroupSpec, value payloadutil.OptionValueSpec) int {
	limit := group.MaxSingleSelections
	if value.MaxCount > 0 && (limit == 0 || value.MaxCount < limit) {
		limit = value.MaxCount
	}
	return limit
}

func canonicalValueID(spec payloadutil.OptionGroupSpec, valueID string) string {
	valueID = strings.TrimSpace(valueID)
	if valueID == "" {
		return ""
	}
	if _, ok := spec.Values[valueID]; ok {
		return valueID
	}
	for id := range spec.Values {
		if strings.EqualFold(id, valueID) {
			return id
		}
	}
	return ""
}

func groupLabel(groupID string, spec payloadutil.OptionGroupSpec) string {
	if spec.Name == "" || strings.EqualFold(spec.Name, groupID) {
		return fmt.Sprintf("%q", groupID)
	}
	return fmt.Sprintf("%q (%s)", groupID, spec.Name)
}

// ResolveGroupToken finds the option group a token names: its item-level id,
// its shared library id (when only one group of the item uses it), or its
// name.
func ResolveGroupToken(token string, specs map[string]payloadutil.OptionGroupSpec) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("option group is required")
	}
	if _, ok := specs[token]; ok {
		return token, nil
	}
	libraryMatches := []string{}
	for groupID, spec := range specs {
		if spec.OptionID != "" && strings.EqualFold(spec.OptionID, token) {
			libraryMatches = append(libraryMatches, groupID)
		}
	}
	switch len(libraryMatches) {
	case 0:
	case 1:
		return libraryMatches[0], nil
	default:
		sort.Strings(libraryMatches)
		return "", fmt.Errorf(
			"option group %q is shared by %d groups of this item; use one of their IDs: %s",
			token,
			len(libraryMatches),
			strings.Join(libraryMatches, ", "),
		)
	}
	groupIDs := make([]string, 0, len(specs))
	for groupID := range specs {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	if match, ambiguous := uniqueMatch(token, groupIDs, func(groupID string) string {
		return groupID
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option group %q; use its ID", token)
		}
		return match, nil
	}
	if match, ambiguous := uniqueMatch(token, groupIDs, func(groupID string) string {
		return specs[groupID].Name
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option group %q; use its ID", token)
		}
		return match, nil
	}
	return "", fmt.Errorf("unknown option group %q", token)
}

// ResolveValueToken finds the value of a group a token names: its id or name.
func ResolveValueToken(token string, group payloadutil.OptionGroupSpec) (string, error) {
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
	if match, ambiguous := uniqueMatch(token, valueIDs, func(valueID string) string {
		return valueID
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option value %q; use its ID", token)
		}
		return match, nil
	}
	if match, ambiguous := uniqueMatch(token, valueIDs, func(valueID string) string {
		return group.Values[valueID].Name
	}); match != "" || ambiguous {
		if ambiguous {
			return "", fmt.Errorf("ambiguous option value %q; use its ID", token)
		}
		return match, nil
	}
	return "", fmt.Errorf("unknown option value %q", token)
}

func uniqueMatch(token string, ids []string, label func(string) string) (string, bool) {
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
