package catalogitem

import "testing"

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
		},
	}

	issues := ValidateItemIDs(payload, []string{"available", "sold-out", "missing"})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want two", issues)
	}
	if issues[0].ItemID != "sold-out" || issues[0].Reason != "Sold out" {
		t.Fatalf("unexpected sold-out issue: %#v", issues[0])
	}
	if issues[1].ItemID != "missing" {
		t.Fatalf("unexpected missing issue: %#v", issues[1])
	}
}
