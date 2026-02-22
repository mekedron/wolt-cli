package cli

import "testing"

func TestPaginateFlatRowsSetsTotalPages(t *testing.T) {
	data := map[string]any{
		"items": []any{"a", "b", "c", "d", "e"},
	}
	limit := 2

	paginateFlatRows(data, "items", &limit, 0)

	if got := asInt(data["total"]); got != 5 {
		t.Fatalf("expected total 5, got %v", data["total"])
	}
	if got := asInt(data["count"]); got != 2 {
		t.Fatalf("expected count 2, got %v", data["count"])
	}
	if got := asInt(data["total_pages"]); got != 3 {
		t.Fatalf("expected total_pages 3, got %v", data["total_pages"])
	}
	if got := asInt(data["next_offset"]); got != 2 {
		t.Fatalf("expected next_offset 2, got %v", data["next_offset"])
	}
}

func TestPaginateFlatRowsOmitsTotalPagesWithoutPositiveLimit(t *testing.T) {
	dataWithoutLimit := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	paginateFlatRows(dataWithoutLimit, "items", nil, 0)
	if _, ok := dataWithoutLimit["total_pages"]; ok {
		t.Fatalf("expected total_pages to be omitted when limit is not set")
	}

	dataWithZeroLimit := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	zeroLimit := 0
	paginateFlatRows(dataWithZeroLimit, "items", &zeroLimit, 0)
	if _, ok := dataWithZeroLimit["total_pages"]; ok {
		t.Fatalf("expected total_pages to be omitted when limit <= 0")
	}
}

func TestPaginateDiscoveryFeedRowsSetsTotalPages(t *testing.T) {
	data := map[string]any{
		"sections": []any{
			map[string]any{"name": "one", "items": []any{"a", "b", "c"}},
			map[string]any{"name": "two", "items": []any{"d", "e"}},
		},
	}
	limit := 2

	paginateDiscoveryFeedRows(data, &limit, 1)

	if got := asInt(data["total"]); got != 5 {
		t.Fatalf("expected total 5, got %v", data["total"])
	}
	if got := asInt(data["count"]); got != 2 {
		t.Fatalf("expected count 2, got %v", data["count"])
	}
	if got := asInt(data["offset"]); got != 1 {
		t.Fatalf("expected offset 1, got %v", data["offset"])
	}
	if got := asInt(data["total_pages"]); got != 3 {
		t.Fatalf("expected total_pages 3, got %v", data["total_pages"])
	}
	if got := asInt(data["next_offset"]); got != 3 {
		t.Fatalf("expected next_offset 3, got %v", data["next_offset"])
	}
}
