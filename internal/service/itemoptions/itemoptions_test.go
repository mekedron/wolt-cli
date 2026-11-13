package itemoptions

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// mealAssortment mirrors a live Wolt assortment (Taco Bell meal, 2026-11):
// the dish lists item-level groups with ranges and prerequisites, and the
// venue library carries values, prices and default_value. Every library
// group has a default, including optional ones whose default is a paid extra.
func mealAssortment() map[string]any {
	return map[string]any{
		"items": []any{
			map[string]any{
				"id":    "item-meal",
				"name":  "Crunchwrap Meal",
				"price": 1699,
				"options": []any{
					itemGroup("grp-main", "lib-main", "Crunchwrap", 1, 1, 1, nil),
					itemGroup("grp-protein", "lib-protein", "Protein", 1, 1, 1, []any{"val-crunchwrap"}),
					itemGroup("grp-extras", "lib-extras", "Add Ingredients", 0, 5, 2, []any{"val-crunchwrap"}),
					itemGroup("grp-drink", "lib-drink", "Drink", 1, 1, 1, nil),
					itemGroup("grp-sauce", "lib-sauce", "Sauce", 1, 1, 1, []any{"val-never-selected"}),
				},
			},
		},
		"options": []any{
			libraryGroup("lib-main", "val-crunchwrap", value("val-crunchwrap", "Crunchwrap", 0, 1)),
			libraryGroup("lib-protein", "val-beef",
				value("val-beef", "Beef", 0, 1),
				value("val-chicken", "Chicken", 0, 1),
			),
			libraryGroup("lib-extras", "val-extra-beef",
				value("val-extra-beef", "Extra beef", 150, 1),
				value("val-cheese", "Cheese", 100, 2),
			),
			libraryGroup("lib-drink", "val-cola",
				value("val-cola", "Cola", 0, 1),
				value("val-large-cola", "Large cola", 120, 1),
			),
			libraryGroup("lib-sauce", "val-mild", value("val-mild", "Mild", 0, 1)),
		},
	}
}

func itemGroup(id, libraryID, name string, minimum, maximum, maxSingle int, prerequisites []any) map[string]any {
	if prerequisites == nil {
		prerequisites = []any{}
	}
	return map[string]any{
		"id":                  id,
		"option_id":           libraryID,
		"name":                name,
		"prerequisite_values": prerequisites,
		"multi_choice_config": map[string]any{
			"total_range":           map[string]any{"min": minimum, "max": maximum},
			"max_single_selections": maxSingle,
			"free_selections":       0,
		},
	}
}

func libraryGroup(id, defaultValue string, values ...any) map[string]any {
	return map[string]any{
		"id":            id,
		"name":          id + " library name",
		"type":          "choice",
		"default_value": defaultValue,
		"values":        values,
	}
}

func value(id, name string, price, maximum int) map[string]any {
	return map[string]any{
		"id":    id,
		"name":  name,
		"price": price,
		"multi_choice_config": map[string]any{
			"total_range": map[string]any{"min": 0, "max": maximum},
		},
	}
}

func mealPayload(t *testing.T) map[string]any {
	t.Helper()
	assortment := mealAssortment()
	item := payloadutil.Map(payloadutil.Slice(assortment["items"])[0])
	groups := payloadutil.JoinOptionLibrary(item, payloadutil.Slice(assortment["options"]))
	if len(groups) != 5 {
		t.Fatalf("joined groups = %d, want 5: %#v", len(groups), groups)
	}
	return map[string]any{"id": "item-meal", "price": 1699, "options": groups}
}

func selectedValues(options []any) map[string]map[string]int {
	out := map[string]map[string]int{}
	for _, rawGroup := range options {
		group := payloadutil.Map(rawGroup)
		values := map[string]int{}
		for _, rawValue := range payloadutil.Slice(group["values"]) {
			entry := payloadutil.Map(rawValue)
			values[payloadutil.String(entry["id"])] = payloadutil.Int(entry["count"])
		}
		out[payloadutil.String(group["id"])] = values
	}
	return out
}

func TestBuildFillsRequiredGroupsWithTheirDefaults(t *testing.T) {
	result, err := Build(mealPayload(t), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := map[string]map[string]int{
		"grp-main":    {"val-crunchwrap": 1},
		"grp-protein": {"val-beef": 1},
		"grp-drink":   {"val-cola": 1},
	}
	if got := selectedValues(result.Options); !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %v, want %v (optional extras and the sauce group whose prerequisite is unselected stay out)", got, want)
	}
	if result.UnitTotal != 0 {
		t.Fatalf("UnitTotal = %d, want 0", result.UnitTotal)
	}
	if len(result.Defaults) != 3 {
		t.Fatalf("Defaults = %#v, want three applied defaults", result.Defaults)
	}
	for _, applied := range result.Defaults {
		if applied.GroupID == "grp-protein" && applied.GroupName != "Protein" {
			t.Fatalf("default group name = %q, want the item-level name Protein", applied.GroupName)
		}
	}
}

func TestBuildKeepsExplicitChoicesAndPricesThem(t *testing.T) {
	result, err := Build(mealPayload(t), Selections{
		"grp-drink":       {{ValueID: "Large cola", Count: 1}},
		"Add Ingredients": {{ValueID: "val-cheese", Count: 2}, {ValueID: "val-extra-beef", Count: 1}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := selectedValues(result.Options)
	if !reflect.DeepEqual(got["grp-drink"], map[string]int{"val-large-cola": 1}) {
		t.Fatalf("drink = %v, want the explicit large cola, not the default", got["grp-drink"])
	}
	if !reflect.DeepEqual(got["grp-extras"], map[string]int{"val-cheese": 2, "val-extra-beef": 1}) {
		t.Fatalf("extras = %v", got["grp-extras"])
	}
	if result.UnitTotal != 120+2*100+150 {
		t.Fatalf("UnitTotal = %d, want %d", result.UnitTotal, 120+2*100+150)
	}
	for _, applied := range result.Defaults {
		if applied.GroupID == "grp-drink" {
			t.Fatalf("explicit drink choice must not be reported as a default: %#v", result.Defaults)
		}
	}
}

func TestBuildRejectsChoicesOutsideTheLimits(t *testing.T) {
	tests := []struct {
		name       string
		selections Selections
		want       string
	}{
		{
			name:       "group whose prerequisite is not selected",
			selections: Selections{"grp-sauce": {{ValueID: "val-mild", Count: 1}}},
			want:       `option group "grp-sauce" (Sauce) applies only when one of these option values is selected: val-never-selected`,
		},
		{
			name:       "value above max_single_selections",
			selections: Selections{"grp-extras": {{ValueID: "val-cheese", Count: 2}, {ValueID: "val-cheese", Count: 1}}},
			want:       `value "val-cheese" can be chosen at most 2 times; got 3`,
		},
		{
			name:       "value above its own maximum",
			selections: Selections{"grp-extras": {{ValueID: "val-extra-beef", Count: 2}}},
			want:       `value "val-extra-beef" can be chosen at most 1 times; got 2`,
		},
		{
			name:       "unknown value",
			selections: Selections{"grp-drink": {{ValueID: "Lemonade", Count: 1}}},
			want:       `unknown option value "Lemonade"`,
		},
		{
			name:       "group above its maximum",
			selections: Selections{"grp-drink": {{ValueID: "val-cola", Count: 1}, {ValueID: "val-large-cola", Count: 1}}},
			want:       `option group "grp-drink" (Drink) selection count must be at most 1; got 2`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(mealPayload(t), test.selections)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

func TestBuildRequiresAChoiceWhenARequiredGroupHasNoDefault(t *testing.T) {
	payload := map[string]any{
		"options": []any{
			map[string]any{
				"id":                  "grp-size",
				"name":                "Size",
				"multi_choice_config": map[string]any{"total_range": map[string]any{"min": 1, "max": 1}},
				"values":              []any{map[string]any{"id": "val-small", "price": 0}},
			},
		},
	}
	if _, err := Build(payload, nil); err == nil ||
		!strings.Contains(err.Error(), `option group "grp-size" (Size) selection count must be at least 1; got 0`) {
		t.Fatalf("Build error = %v, want a missing required choice", err)
	}
	if _, ok := DefaultConfigurationTotal(payload); ok {
		t.Fatal("DefaultConfigurationTotal must report that a choice is needed")
	}
}

func TestDefaultConfigurationTotalAddsPaidRequiredDefaults(t *testing.T) {
	payload := map[string]any{
		"options": []any{
			map[string]any{
				"id":                  "grp-base",
				"default_value":       "val-regular",
				"multi_choice_config": map[string]any{"total_range": map[string]any{"min": 1, "max": 1}},
				"values": []any{
					map[string]any{"id": "val-regular", "price": 250},
					map[string]any{"id": "val-large", "price": 400},
				},
			},
			map[string]any{
				"id":                  "grp-extra",
				"default_value":       "val-bacon",
				"multi_choice_config": map[string]any{"total_range": map[string]any{"min": 0, "max": 1}},
				"values":              []any{map[string]any{"id": "val-bacon", "price": 150}},
			},
		},
	}
	total, ok := DefaultConfigurationTotal(payload)
	if !ok || total != 250 {
		t.Fatalf("DefaultConfigurationTotal = %d, %v; want 250, true (the optional bacon default is not preselected)", total, ok)
	}
}
