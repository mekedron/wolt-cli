package domain

import "testing"

func TestCanonicalVenueURL(t *testing.T) {
	const want = "https://wolt.com/en/example/venue/example-market"
	tests := []struct {
		name string
		raw  string
		slug string
		want string
	}{
		{name: "venue root", raw: want, want: want},
		{
			name: "nested item",
			raw:  "http://user@wolt.com/en/example/venue/example-market/items/item-1?source=test#details",
			slug: "example-market",
			want: want,
		},
		{
			name: "restaurant path",
			raw:  "https://wolt.com/en/example/restaurant/example-market/menu",
			want: "https://wolt.com/en/example/restaurant/example-market",
		},
		{
			name: "www host canonicalized",
			raw:  "https://www.wolt.com/en/example/venue/example-market",
			want: want,
		},
		{name: "different venue", raw: want, slug: "other-market"},
		{name: "different host", raw: "https://example.com/en/example/venue/example-market"},
		{name: "missing venue slug", raw: "https://wolt.com/en/example/venue/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CanonicalVenueURL(test.raw, test.slug); got != test.want {
				t.Fatalf("CanonicalVenueURL(%q, %q) = %q, want %q", test.raw, test.slug, got, test.want)
			}
		})
	}
}

func TestVenueSlugFromReference(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: " example-market ", want: "example-market"},
		{raw: "https://wolt.com/en/example/venue/example-market", want: "example-market"},
		{raw: "https://www.wolt.com/en/example/venue/example-market", want: "example-market"},
		{raw: "https://wolt.com/en/example/venue/example-market/categories/fish", want: "example-market"},
		{raw: "https://wolt.com/en/example/restaurant/example-market/itemid-1", want: "example-market"},
		{raw: "https://wolt.com/en/discovery/example", want: "example"},
		{raw: "https://wolt.com", want: ""},
		{raw: "https://wolt.com/?ref=homepage#top", want: ""},
		{raw: "https://example.com/en/example/venue/example-market", want: ""},
		{raw: "ftp://wolt.com/en/example/venue/example-market", want: ""},
		{raw: "https:///en/example/venue/example-market", want: ""},
	}
	for _, test := range tests {
		if got := VenueSlugFromReference(test.raw); got != test.want {
			t.Errorf("VenueSlugFromReference(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestIsWoltURL(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want bool
	}{
		{raw: "https://wolt.com/en/example/venue/example-market", want: true},
		{raw: "http://www.wolt.com/en/example/venue/example-market", want: true},
		{raw: "https://WOLT.COM./en/example/venue/example-market", want: true},
		{raw: "https://example.com/en/example/venue/example-market", want: false},
		{raw: "https://wolt.com.example.org/en/example/venue/example-market", want: false},
		{raw: "ftp://wolt.com/en/example/venue/example-market", want: false},
		{raw: "example-market", want: false},
	} {
		if got := IsWoltURL(test.raw); got != test.want {
			t.Errorf("IsWoltURL(%q) = %t, want %t", test.raw, got, test.want)
		}
	}
}

func TestIsObjectID(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "0123456789abcdefABCDEF01", want: true},
		{value: " 0123456789abcdefABCDEF01 ", want: true},
		{value: "example-market", want: false},
		{value: "0123456789abcdefABCDEF0g", want: false},
		{value: "0123456789abcdefABCDEF012", want: false},
	} {
		if got := IsObjectID(test.value); got != test.want {
			t.Errorf("IsObjectID(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}
