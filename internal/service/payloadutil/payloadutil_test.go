package payloadutil

import (
	"reflect"
	"testing"
)

func TestMap(t *testing.T) {
	if got := Map(nil); got != nil {
		t.Errorf("Map(nil) = %v, want nil", got)
	}
	if got := Map("not a map"); got != nil {
		t.Errorf("Map(string) = %v, want nil", got)
	}
	in := map[string]any{"a": 1}
	if got := Map(in); !reflect.DeepEqual(got, in) {
		t.Errorf("Map(map[string]any) = %v, want %v", got, in)
	}
	// map[string]string must be widened to map[string]any.
	if got := Map(map[string]string{"k": "v"}); !reflect.DeepEqual(got, map[string]any{"k": "v"}) {
		t.Errorf("Map(map[string]string) = %v, want widened map", got)
	}
}

func TestSlice(t *testing.T) {
	if got := Slice(nil); got != nil {
		t.Errorf("Slice(nil) = %v, want nil", got)
	}
	if got := Slice([]any{1, "two"}); !reflect.DeepEqual(got, []any{1, "two"}) {
		t.Errorf("Slice([]any) = %v", got)
	}
	// Typed slices are reflected into []any.
	if got := Slice([]string{"a", "b"}); !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Errorf("Slice([]string) = %v, want []any{a,b}", got)
	}
	// Byte slices are deliberately NOT treated as element slices.
	if got := Slice([]byte("hi")); got != nil {
		t.Errorf("Slice([]byte) = %v, want nil", got)
	}
	if got := Slice(42); got != nil {
		t.Errorf("Slice(int) = %v, want nil", got)
	}
}

func TestString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{42, "42"},
		{true, "true"},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("String(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBool(t *testing.T) {
	if !Bool(true) {
		t.Error("Bool(true) = false")
	}
	if Bool("true") {
		t.Error(`Bool("true") = true, want false (only real bools count)`)
	}
	if Bool(nil) {
		t.Error("Bool(nil) = true")
	}
}

func TestInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{int(7), 7},
		{int64(8), 8},
		{float64(9.9), 9}, // JSON numbers decode to float64; truncation is intentional.
		{float32(10.5), 10},
		{"11", 0}, // strings are not coerced.
		{nil, 0},
	}
	for _, c := range cases {
		if got := Int(c.in); got != c.want {
			t.Errorf("Int(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCoalesceAny(t *testing.T) {
	if got := CoalesceAny(nil, "", "first", "second"); got != "first" {
		t.Errorf("CoalesceAny skipped nil/blank wrong: got %v, want first", got)
	}
	if got := CoalesceAny(nil, "   ", 0); got != 0 {
		t.Errorf("CoalesceAny should return first non-blank (0), got %v", got)
	}
	if got := CoalesceAny(nil, ""); got != nil {
		t.Errorf("CoalesceAny(all blank) = %v, want nil", got)
	}
	// A non-empty string wins over a later value.
	if got := CoalesceAny("x", "y"); got != "x" {
		t.Errorf("CoalesceAny = %v, want x", got)
	}
}

func TestInferCurrency(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"€12.50", "EUR"},
		{"$9.99", "USD"},
		{"PLN 20", "PLN"},
		{"  €5 ", "EUR"},
		{"12.50", ""}, // no recognizable symbol
		{"", ""},
	}
	for _, c := range cases {
		if got := InferCurrency(c.in); got != c.want {
			t.Errorf("InferCurrency(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractOptionSpecs(t *testing.T) {
	payload := map[string]any{
		"option_groups": []any{
			map[string]any{
				"id":       "group-1",
				"name":     "Size",
				"required": true,
				"min":      1,
				"max":      1,
				"values": []any{
					map[string]any{"id": "val-a", "name": "Small", "price": 0},
					map[string]any{"id": "val-b", "name": "Large", "price": 150},
				},
			},
		},
	}

	specs := ExtractOptionSpecs(payload)
	group, ok := specs["group-1"]
	if !ok {
		t.Fatalf("expected group-1 in specs, got keys %v", keys(specs))
	}
	if group.Name != "Size" || !group.Required || group.MinSelect != 1 || group.MaxSelect != 1 {
		t.Errorf("group metadata mismatch: %+v", group)
	}
	if len(group.Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(group.Values))
	}
	if group.Values["val-b"].Price != 150 || group.Values["val-b"].Name != "Large" {
		t.Errorf("value val-b mismatch: %+v", group.Values["val-b"])
	}
}

func TestExtractOptionSpecsNestedPriceObject(t *testing.T) {
	// Wolt sometimes nests the price as {"amount": N} instead of a scalar.
	payload := map[string]any{
		"options": []any{
			map[string]any{
				"group_id": "g",
				"items": []any{
					map[string]any{"value_id": "v", "title": "Cheese", "price": map[string]any{"amount": 250}},
				},
			},
		},
	}
	specs := ExtractOptionSpecs(payload)
	if got := specs["g"].Values["v"].Price; got != 250 {
		t.Errorf("nested price.amount not resolved: got %d, want 250", got)
	}
	if got := specs["g"].Values["v"].Name; got != "Cheese" {
		t.Errorf("title fallback not resolved: got %q, want Cheese", got)
	}
}

func keys(m map[string]OptionGroupSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
