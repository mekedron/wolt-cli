package catalogitem

import (
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func TestResolveAvailability(t *testing.T) {
	tests := []struct {
		name       string
		item       map[string]any
		available  bool
		wantReason string
	}{
		{
			name:      "nil disabled info and unknown balance is available",
			item:      map[string]any{"disabled_info": nil, "purchasable_balance": nil},
			available: true,
		},
		{
			name: "disabled info is authoritative",
			item: map[string]any{
				"disabled_info":       map[string]any{"disable_text": "Sold out"},
				"purchasable_balance": 12,
			},
			available:  false,
			wantReason: "Sold out",
		},
		{
			name:       "empty disabled object fails closed",
			item:       map[string]any{"disabled_info": map[string]any{}},
			available:  false,
			wantReason: "item is disabled by Wolt",
		},
		{
			name:       "string disabled info fails closed",
			item:       map[string]any{"disabled_info": "unexpected"},
			available:  false,
			wantReason: "item is disabled by Wolt",
		},
		{
			name:       "boolean disabled info fails closed",
			item:       map[string]any{"disabled_info": false},
			available:  false,
			wantReason: "item is disabled by Wolt",
		},
		{
			name:       "array disabled info fails closed",
			item:       map[string]any{"disabled_info": []any{}},
			available:  false,
			wantReason: "item is disabled by Wolt",
		},
		{
			name: "zero balance blocks purchase",
			item: map[string]any{
				"disabled_info":       nil,
				"purchasable_balance": 0,
			},
			available:  false,
			wantReason: "purchasable balance is zero",
		},
		{
			name: "positive balance is available",
			item: map[string]any{
				"disabled_info":       nil,
				"purchasable_balance": 3,
			},
			available: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := ResolveAvailability(test.item)
			if got.IsAvailable != test.available {
				t.Fatalf("IsAvailable = %v, want %v", got.IsAvailable, test.available)
			}
			if got.Reason != test.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, test.wantReason)
			}
		})
	}
}

func TestFindAndImages(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":   "item-1",
				"name": "Chicken",
				"images": []any{
					map[string]any{"url": "https://example.test/one.jpg", "blurhash": "hash"},
					map[string]any{"url": "https://example.test/two.jpg"},
				},
			},
		},
	}

	item := Find(payload, "item-1")
	if item == nil {
		t.Fatal("Find returned nil")
	}
	urls := ImageURLs(item)
	if len(urls) != 2 || urls[0] != "https://example.test/one.jpg" || urls[1] != "https://example.test/two.jpg" {
		t.Fatalf("ImageURLs = %#v", urls)
	}
	if got := ImageBlurhash(item); got != "hash" {
		t.Fatalf("ImageBlurhash = %q, want hash", got)
	}
}

func TestFindUsesCaseInsensitiveIDAndDeterministicRichestCandidate(t *testing.T) {
	const itemID = "abcdefabcdefabcdefabcdef"
	payload := map[string]any{
		"z_stale": map[string]any{
			"id":   itemID,
			"name": "Stale item",
		},
		"a_current": map[string]any{
			"item_id":             itemID,
			"name":                "Current item",
			"price":               500,
			"purchasable_balance": 4,
		},
	}

	for range 100 {
		item := Find(payload, strings.ToUpper(itemID))
		if item == nil || item["name"] != "Current item" {
			t.Fatalf("Find selected %#v", item)
		}
	}
}

func TestMarkMissingFromCurrentAssortmentPreservesMetadata(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":          "item-1",
				"name":        "Older item name",
				"description": "Useful page metadata",
				"price":       500,
			},
		},
	}
	marked := MarkMissingFromCurrentAssortment(payload, "ITEM-1")
	item := Find(marked, "item-1")
	if item == nil || item["description"] != "Useful page metadata" {
		t.Fatalf("marked item lost metadata: %#v", item)
	}
	availability := ResolveAvailability(item)
	if availability.IsAvailable || availability.Reason != missingCurrentAssortmentReason {
		t.Fatalf("marked availability = %#v", availability)
	}
}

func TestValidateItemIDsFailsClosed(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":                  "available",
				"name":                "Available",
				"disabled_info":       nil,
				"purchasable_balance": nil,
			},
			map[string]any{
				"id":                  "sold-out",
				"name":                "Sold out item",
				"disabled_info":       map[string]any{"disable_text": "Sold out"},
				"purchasable_balance": 0,
			},
			map[string]any{
				"id":                  "malformed-disabled",
				"name":                "Malformed disabled item",
				"disabled_info":       []any{"unexpected"},
				"purchasable_balance": 5,
			},
		},
	}

	issues := ValidateItemIDs(payload, []string{"available", "sold-out", "malformed-disabled", "missing"})
	if len(issues) != 3 {
		t.Fatalf("issues = %#v, want three", issues)
	}
	if issues[0].ItemID != "sold-out" || issues[0].Reason != "Sold out" {
		t.Fatalf("unexpected sold-out issue: %#v", issues[0])
	}
	if issues[1].ItemID != "malformed-disabled" || issues[1].Reason != "item is disabled by Wolt" {
		t.Fatalf("unexpected malformed-disabled issue: %#v", issues[1])
	}
	if issues[2].ItemID != "missing" {
		t.Fatalf("unexpected missing issue: %#v", issues[2])
	}
}

func TestFindAcceptsIDOnlyItemsWithoutMatchingCategoryObjects(t *testing.T) {
	const itemID = "abcdefabcdefabcdefabcdef"
	payload := map[string]any{
		"categories": []any{
			map[string]any{"id": itemID, "name": "Not an item"},
		},
		"items": []any{
			map[string]any{"id": strings.ToUpper(itemID)},
		},
	}
	item := Find(payload, itemID)
	if item == nil || item["id"] != strings.ToUpper(itemID) {
		t.Fatalf("Find selected %#v", item)
	}
}

func TestMergeCurrentItemPreservesRichMetadataAndReplacesAvailability(t *testing.T) {
	base := map[string]any{
		"id":                  "item-a",
		"name":                "Rich name",
		"description":         "Rich description",
		"price":               map[string]any{"amount": 300, "currency": "EUR"},
		"disabled_info":       map[string]any{"disable_text": "Stale"},
		"purchasable_balance": 0,
		"images": []any{
			map[string]any{"url": "https://images.example/item-a"},
		},
		"option_groups": []any{
			map[string]any{
				"id":   "size",
				"name": "Size",
				"values": []any{
					map[string]any{"id": "large", "name": "Large", "price": 50},
				},
			},
		},
	}
	current := map[string]any{
		"id":          "item-a",
		"name":        "Fresh name",
		"description": "",
		"price":       map[string]any{"amount": 250},
		"images":      []any{},
		"options":     []any{map[string]any{"id": "SIZE"}},
	}

	merged := MergeCurrentItem(base, current)
	if merged["name"] != "Fresh name" ||
		merged["description"] != "Rich description" ||
		len(ImageURLs(merged)) != 1 ||
		payloadutil.MinorAmount(merged["price"]) != 250 {
		t.Fatalf("rich metadata merge = %#v", merged)
	}
	if payloadutil.String(payloadutil.Map(merged["price"])["currency"]) != "EUR" {
		t.Fatalf("fresh price erased the known currency: %#v", merged["price"])
	}
	if _, exists := merged["disabled_info"]; exists {
		t.Fatalf("stale disabled_info survived: %#v", merged)
	}
	if _, exists := merged["purchasable_balance"]; exists {
		t.Fatalf("stale purchasable_balance survived: %#v", merged)
	}
	spec := payloadutil.ExtractOptionSpecs(merged)["size"]
	if spec.Name != "Size" || spec.Values["large"].Name != "Large" {
		t.Fatalf("rich option definition was erased: %#v", merged)
	}

	withoutPrice := MergeCurrentItem(base, map[string]any{"id": "item-a", "name": "Fresh name"})
	if payloadutil.MinorAmount(withoutPrice["price"]) != 300 {
		t.Fatalf("missing fresh price erased the known price: %#v", withoutPrice["price"])
	}
}

func TestMergeCurrentItemPrefersEquallyRichFreshOptionDefinitions(t *testing.T) {
	optionGroup := func(name string, price int) map[string]any {
		return map[string]any{
			"id":       "size",
			"name":     name,
			"required": true,
			"values": []any{
				map[string]any{
					"id":    "large",
					"name":  "Large",
					"price": price,
				},
			},
		}
	}
	base := map[string]any{
		"id":            "item-a",
		"option_groups": []any{optionGroup("Stale size", 50)},
	}
	current := map[string]any{
		"id":      "item-a",
		"options": []any{optionGroup("Current size", 75)},
	}

	merged := MergeCurrentItem(base, current)
	spec := payloadutil.ExtractOptionSpecs(merged)["size"]
	value := spec.Values["large"]
	if spec.Name != "Current size" || !value.HasPrice || value.Price != 75 {
		t.Fatalf("fresh option definition lost: %#v", merged)
	}
}

func TestScopedItemIncludesOnlyReferencedOptionGroups(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{
				"id":               "item-a",
				"name":             "Target",
				"option_group_ids": []any{"size"},
			},
		},
		"option_groups": []any{
			map[string]any{
				"id":   "size",
				"name": "Size",
				"values": []any{
					map[string]any{"id": "large", "name": "Large"},
				},
			},
			map[string]any{
				"id":   "upsell-sauce",
				"name": "Unrelated",
				"values": []any{
					map[string]any{"id": "hot", "name": "Hot"},
				},
			},
		},
	}
	scoped := ScopedItem(payload, "ITEM-A")
	specs := payloadutil.ExtractOptionSpecs(scoped)
	if len(specs) != 1 || specs["size"].Name != "Size" {
		t.Fatalf("scoped specs = %#v, payload=%#v", specs, scoped)
	}
}
