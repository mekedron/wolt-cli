package payloadutil

import (
	"reflect"
	"testing"
)

func TestBuildWeightedBasketItemUsesSelectedWeightPrice(t *testing.T) {
	config, ok := WeightConfigFromItem(map[string]any{
		"sell_by_weight_config": map[string]any{
			"input_type":     "grams",
			"grams_per_step": 500,
			"price_per_kg":   1645,
		},
	})
	if !ok {
		t.Fatal("WeightConfigFromItem() did not recognize a valid config")
	}

	item, err := BuildWeightedBasketItem(map[string]any{
		"id":      "000000000000000000000401",
		"name":    "Synthetic weighted item",
		"price":   1645,
		"options": []any{},
	}, 1, config)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo := map[string]any{
		"count":                     1,
		"purchased_weight_in_grams": 500,
		"weighted_item_input_type":  "grams",
	}
	if item["count"] != 1 || item["price"] != 823 ||
		!reflect.DeepEqual(item["weighted_item_info"], wantInfo) {
		t.Fatalf("weighted basket item = %#v", item)
	}
	if _, exists := item["is_weighted_item"]; exists {
		t.Fatal("basket item contains checkout-only is_weighted_item")
	}
}

func TestWeightedValuesSupportNumberOfItems(t *testing.T) {
	config := WeightConfig{
		InputType:        WeightedInputNumberOfItems,
		GramsPerStep:     200,
		PricePerKilogram: 3999,
	}
	values, err := config.ValuesForSteps(2)
	if err != nil {
		t.Fatal(err)
	}
	if values != (WeightedValues{Count: 2, Grams: 400, Price: 1600}) {
		t.Fatalf("ValuesForSteps(2) = %#v", values)
	}
}

func TestMergeWeightedBasketItemsRepairsLegacyLine(t *testing.T) {
	config := WeightConfig{
		InputType:        WeightedInputGrams,
		GramsPerStep:     500,
		PricePerKilogram: 1645,
	}
	basket := map[string]any{"items": []any{
		map[string]any{
			"id":    "000000000000000000000401",
			"count": 1,
			"name":  "Old snapshot",
			"price": 1645,
			"weighted_item_info": map[string]any{
				"count":                     1,
				"purchased_weight_in_grams": 500,
			},
		},
		map[string]any{"id": "000000000000000000000402", "count": 1, "price": 250},
	}}
	items, err := MergeWeightedBasketItems(
		basket,
		"000000000000000000000401",
		1,
		map[string]any{"id": "000000000000000000000401", "name": "Current item", "price": 1645},
		config,
	)
	if err != nil {
		t.Fatal(err)
	}
	weighted := Map(items[0])
	info := Map(weighted["weighted_item_info"])
	if len(items) != 2 || weighted["count"] != 1 || weighted["price"] != 1645 ||
		info["purchased_weight_in_grams"] != 1000 ||
		String(Map(items[1])["id"]) != "000000000000000000000402" {
		t.Fatalf("merged basket items = %#v", items)
	}
}

func TestRemoveBasketItemsRejectsPartialWeightedRemoval(t *testing.T) {
	basket := map[string]any{"items": []any{
		map[string]any{
			"id":    "000000000000000000000403",
			"count": 2,
			"price": 1600,
			"weighted_item_info": map[string]any{
				"count":                     2,
				"purchased_weight_in_grams": 400,
				"weighted_item_input_type":  "number_of_items",
			},
		},
	}}
	if _, _, err := RemoveBasketItems(basket, "000000000000000000000403", 1); err == nil {
		t.Fatal("partial weighted removal succeeded without current catalog pricing")
	}
}
