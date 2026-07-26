package checkoutpayload

import (
	"context"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func TestCheckoutCategoryIndexUsesOnlyDeterministicCategoryContainers(t *testing.T) {
	itemID := "item-alpha"
	index := buildCheckoutCategoryIDIndex(map[string]any{
		"z_wrapper": map[string]any{
			"categories": []any{
				map[string]any{"id": "category-z", "items": []any{itemID}},
			},
		},
		"a_wrapper": map[string]any{
			"categories": []any{
				map[string]any{"id": "category-a", "items": []any{strings.ToUpper(itemID)}},
			},
		},
		"unrelated": map[string]any{
			"id":    "not-a-category",
			"items": []any{"000000000000000000000502"},
		},
	})

	if got := index[itemID]; got != "category-a" {
		t.Fatalf("category index = %#v, want deterministic category-a", index)
	}
	if _, exists := index["000000000000000000000502"]; exists {
		t.Fatalf("unrelated {id, items} object leaked into category index: %#v", index)
	}
}

func TestBuildRejectsLineTotalOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	basket := basketWithItem("EUR 1", map[string]any{
		"id":          "000000000000000000000503",
		"count":       2,
		"price":       maxInt,
		"category_id": "category-a",
	})
	_, _, err := Build(
		context.Background(),
		nil,
		nil,
		basket,
		domain.Location{},
		"standard",
		0,
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "integer range") {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestBuildRejectsIncompleteBasketReconstruction(t *testing.T) {
	validItem := map[string]any{
		"id":          "000000000000000000000504",
		"count":       1,
		"price":       500,
		"category_id": "category-a",
	}
	tests := []struct {
		name  string
		items any
		want  string
	}{
		{name: "wrong items container", items: "invalid", want: "items must be an array"},
		{name: "non-object line", items: []any{"invalid"}, want: "must be an object"},
		{
			name: "missing option value price",
			items: []any{
				map[string]any{
					"id":          validItem["id"],
					"count":       1,
					"price":       500,
					"category_id": "category-a",
					"options": []any{
						map[string]any{
							"id":     "size",
							"values": []any{map[string]any{"id": "large", "count": 1}},
						},
					},
				},
			},
			want: "has no current price",
		},
		{
			name:  "nonpositive item count",
			items: []any{map[string]any{"id": validItem["id"], "count": 0, "price": 500, "category_id": "category-a"}},
			want:  "count must be greater than zero",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			basket := basketWithItem("EUR 5", validItem)
			basket["items"] = test.items
			_, _, err := Build(
				context.Background(),
				nil,
				nil,
				basket,
				domain.Location{},
				"standard",
				0,
				"",
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build() error = %v, want %q", err, test.want)
			}
		})
	}
}
