package payloadutil

import "testing"

func testOptionGroup(groupID, groupName, valueID, valueName string, price ...any) map[string]any {
	value := map[string]any{"id": valueID}
	if valueName != "" {
		value["name"] = valueName
	}
	if len(price) > 0 {
		value["price"] = price[0]
	}
	group := map[string]any{"id": groupID, "values": []any{value}}
	if groupName != "" {
		group["name"] = groupName
	}
	return group
}

func TestExtractOptionSpecsUsesDeterministicFreshPrecedence(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			testOptionGroup("size", "Fresh size", "large", "Fresh large", map[string]any{"amount": 125}),
		},
		"items": []any{
			map[string]any{
				"id": "item-a",
				"option_groups": []any{
					testOptionGroup("size", "Stale size", "large", "Stale large", map[string]any{"amount": 50}),
				},
			},
		},
	}

	for range 100 {
		spec := ExtractOptionSpecs(payload)["size"]
		value := spec.Values["large"]
		if spec.Name != "Fresh size" ||
			value.Name != "Fresh large" ||
			value.Price != 125 {
			t.Fatalf("option precedence changed: group %#v, value %#v", spec, value)
		}
	}
}

func TestExtractOptionSpecsDistinguishesMissingAndExplicitZeroPrice(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			testOptionGroup("missing-price", "", "value", "Fresh missing price"),
			testOptionGroup("free", "", "value", "Fresh free", 0),
		},
		"items": []any{
			map[string]any{
				"option_groups": []any{
					testOptionGroup("missing-price", "", "value", "Stale priced", 125),
					testOptionGroup("free", "", "value", "Stale priced", 125),
				},
			},
		},
	}

	specs := ExtractOptionSpecs(payload)
	missingPrice := specs["missing-price"].Values["value"]
	if missingPrice.Name != "Fresh missing price" || missingPrice.Price != 125 || !missingPrice.HasPrice {
		t.Fatalf("missing-price merge = %#v", missingPrice)
	}
	free := specs["free"].Values["value"]
	if free.Name != "Fresh free" || free.Price != 0 || !free.HasPrice {
		t.Fatalf("explicit free-price merge = %#v", free)
	}
}

func TestMergeOptionGroupsKeepsOrderAndReplacesIDOnlyDefinition(t *testing.T) {
	merged := MergeOptionGroups(
		[]any{
			map[string]any{"id": "size"},
			map[string]any{"id": "kept", "name": "First"},
		},
		[]any{
			map[string]any{
				"group_id": "SIZE",
				"name":     "Hydrated size",
				"values": []any{
					map[string]any{"id": "large", "price": 125},
				},
			},
			map[string]any{"id": "kept", "name": "Second"},
			map[string]any{"id": "added", "name": "Added"},
		},
	)

	if len(merged) != 3 ||
		optionGroupID(merged[0]) != "SIZE" ||
		optionGroupID(merged[1]) != "kept" ||
		optionGroupID(merged[2]) != "added" {
		t.Fatalf("merged option groups = %#v", merged)
	}
	first := Map(merged[0])
	if first["name"] != "Hydrated size" || len(Slice(first["values"])) != 1 {
		t.Fatalf("ID-only definition was not enriched: %#v", first)
	}
	if Map(merged[1])["name"] != "First" {
		t.Fatalf("equally complete duplicate changed precedence: %#v", merged[1])
	}
}

func TestExtractOptionSpecsMergesGroupAndValueIDsCaseInsensitively(t *testing.T) {
	payload := map[string]any{
		"options": []any{
			testOptionGroup("SIZE", "", "LARGE", ""),
		},
		"option_groups": []any{
			testOptionGroup("size", "Size", "large", "Large", 125),
		},
	}

	specs := ExtractOptionSpecs(payload)
	if len(specs) != 1 {
		t.Fatalf("specs = %#v", specs)
	}
	spec := specs["SIZE"]
	if spec.Name != "Size" || len(spec.Values) != 1 {
		t.Fatalf("merged group = %#v", spec)
	}
	value := spec.Values["LARGE"]
	if value.Name != "Large" || value.Price != 125 || !value.HasPrice {
		t.Fatalf("merged value = %#v", value)
	}
}

func TestExtractOptionSpecsSkipsRelatedItemOptionGroups(t *testing.T) {
	specs := ExtractOptionSpecs(map[string]any{
		"option_groups": []any{
			testOptionGroup("target-size", "Size", "large", "Large"),
		},
		"upsell_items": []any{
			map[string]any{
				"id":   "upsell",
				"name": "Upsell",
				"option_groups": []any{
					testOptionGroup("upsell-sauce", "Sauce", "hot", "Hot"),
				},
			},
		},
	})
	if len(specs) != 1 || specs["target-size"].Name != "Size" {
		t.Fatalf("related-item specs leaked: %#v", specs)
	}
}
