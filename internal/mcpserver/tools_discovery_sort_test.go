package mcpserver

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mekedron/wolt-cli/internal/domain"
)

var discoverySortPublicValues = []string{
	"recommended",
	"distance",
	"rating",
	"delivery_price",
	"delivery_time",
	"delivery-price",
	"delivery-time",
	"delivery",
	"fee",
}

var (
	recommendedVenueOrder = []string{"Default first", "Fastest delivery", "Highest rated", "Cheapest fee"}
	fastestVenueOrder     = []string{"Fastest delivery", "Default first", "Highest rated", "Cheapest fee"}
	ratingVenueOrder      = []string{"Highest rated", "Default first", "Cheapest fee", "Fastest delivery"}
	feeVenueOrder         = []string{"Cheapest fee", "Default first", "Highest rated", "Fastest delivery"}
)

var discoverySortContract = []struct {
	input     string
	wantOrder []string
}{
	{input: "recommended", wantOrder: recommendedVenueOrder},
	{input: " RECOMMENDED ", wantOrder: recommendedVenueOrder},
	{input: "distance", wantOrder: fastestVenueOrder},
	{input: " Distance ", wantOrder: fastestVenueOrder},
	{input: "rating", wantOrder: ratingVenueOrder},
	{input: " RATING ", wantOrder: ratingVenueOrder},
	{input: "delivery_price", wantOrder: feeVenueOrder},
	{input: " DELIVERY_PRICE ", wantOrder: feeVenueOrder},
	{input: "delivery-price", wantOrder: feeVenueOrder},
	{input: " Delivery-Price ", wantOrder: feeVenueOrder},
	{input: "fee", wantOrder: feeVenueOrder},
	{input: " FEE ", wantOrder: feeVenueOrder},
	{input: "delivery_time", wantOrder: fastestVenueOrder},
	{input: " DELIVERY_TIME ", wantOrder: fastestVenueOrder},
	{input: "delivery-time", wantOrder: fastestVenueOrder},
	{input: " Delivery-Time ", wantOrder: fastestVenueOrder},
	{input: "delivery", wantOrder: fastestVenueOrder},
	{input: " DELIVERY ", wantOrder: fastestVenueOrder},
}

func TestDiscoverySortSchemasDoNotNarrowParserContract(t *testing.T) {
	_, client := connectInMemory(t, Deps{
		Wolt:     &stubWolt{},
		Profiles: &stubProfiles{},
		Location: &stubLocation{},
	})
	defer func() { _ = client.Close() }()

	found := map[string]bool{}
	for tool, err := range client.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		if tool.Name != "wolt_top" && tool.Name != "wolt_search_venues" {
			continue
		}
		found[tool.Name] = true
		properties := asMap(asMap(tool.InputSchema)["properties"])
		sortSchema := asMap(properties["sort"])
		if got := asString(sortSchema["type"]); got != "string" {
			t.Errorf("%s sort type = %q, want string", tool.Name, got)
		}
		if enum := asSlice(sortSchema["enum"]); len(enum) != 0 {
			t.Errorf("%s sort schema narrows case-insensitive inputs with enum %v", tool.Name, enum)
		}
		description := strings.ToLower(asString(sortSchema["description"]))
		for _, phrase := range []string{"case-insensitive", "whitespace"} {
			if !strings.Contains(description, phrase) {
				t.Errorf("%s sort description %q does not mention %q", tool.Name, description, phrase)
			}
		}
	}
	for _, name := range []string{"wolt_top", "wolt_search_venues"} {
		if !found[name] {
			t.Errorf("tool %s was not listed", name)
		}
	}
}

func TestDiscoveryToolsHonorPublicSortContract(t *testing.T) {
	defaultFee, expensiveFee, ratedFee, cheapFee := 300, 500, 400, 100
	items := []domain.Item{
		{
			Title: "Default first",
			Venue: &domain.Venue{
				ID:               "default-first",
				Estimate:         30,
				DeliveryPriceInt: &defaultFee,
				Currency:         "EUR",
				Rating:           &domain.Rating{Score: 8},
			},
		},
		{
			Title: "Fastest delivery",
			Venue: &domain.Venue{
				ID:               "fastest-delivery",
				Estimate:         10,
				DeliveryPriceInt: &expensiveFee,
				Currency:         "EUR",
				Rating:           &domain.Rating{Score: 6},
			},
		},
		{
			Title: "Highest rated",
			Venue: &domain.Venue{
				ID:               "highest-rated",
				Estimate:         40,
				DeliveryPriceInt: &ratedFee,
				Currency:         "EUR",
				Rating:           &domain.Rating{Score: 9},
			},
		},
		{
			Title: "Cheapest fee",
			Venue: &domain.Venue{
				ID:               "cheapest-fee",
				Estimate:         50,
				DeliveryPriceInt: &cheapFee,
				Currency:         "EUR",
				Rating:           &domain.Rating{Score: 7},
			},
		},
	}
	_, client := connectInMemory(t, Deps{
		Wolt: &stubWolt{
			itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
				return items, nil
			},
		},
		Profiles: &stubProfiles{},
		Location: &stubLocation{},
	})
	defer func() { _ = client.Close() }()

	for _, toolName := range []string{"wolt_top", "wolt_search_venues"} {
		for _, test := range discoverySortContract {
			t.Run(toolName+"/"+strings.TrimSpace(test.input), func(t *testing.T) {
				result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
					Name: toolName,
					Arguments: map[string]any{
						"lat":  10.25,
						"lon":  20.5,
						"sort": test.input,
					},
				})
				if err != nil {
					t.Fatalf("CallTool: %v", err)
				}
				if result.IsError {
					t.Fatalf("sort %q returned tool error: %s", test.input, textContent(result))
				}
				output := asMap(result.StructuredContent)
				rows := asSlice(asMap(output["data"])["items"])
				if len(rows) != len(items) {
					t.Fatalf("item count = %d, want %d", len(rows), len(items))
				}
				gotOrder := make([]string, 0, len(rows))
				for _, row := range rows {
					gotOrder = append(gotOrder, asString(asMap(row)["name"]))
				}
				if !slices.Equal(gotOrder, test.wantOrder) {
					t.Errorf("sort %q order = %v, want %v", test.input, gotOrder, test.wantOrder)
				}
			})
		}
	}
}

func TestDiscoverySortRejectsUnknownValueWithAllowedValues(t *testing.T) {
	_, client := connectInMemory(t, Deps{
		Wolt: &stubWolt{
			itemsFn: func(context.Context, domain.Location) ([]domain.Item, error) {
				t.Fatal("invalid sort must be rejected before venue discovery")
				return nil, nil
			},
		},
		Profiles: &stubProfiles{},
		Location: &stubLocation{},
	})
	defer func() { _ = client.Close() }()

	for _, toolName := range []string{"wolt_top", "wolt_search_venues"} {
		t.Run(toolName, func(t *testing.T) {
			result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
				Name: toolName,
				Arguments: map[string]any{
					"lat":  10.25,
					"lon":  20.5,
					"sort": "unknown",
				},
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if !result.IsError {
				t.Fatal("unknown sort value did not return a tool error")
			}
			message := textContent(result)
			if len(message) > 512 {
				t.Errorf("sort validation error is unexpectedly long (%d bytes): %q", len(message), message)
			}
			_, rawAllowed, found := strings.Cut(message, "; allowed: ")
			if !found {
				t.Fatalf("error %q has no exact allowed-values list", message)
			}
			gotAllowed := strings.Split(rawAllowed, ", ")
			wantAllowed := slices.Clone(discoverySortPublicValues)
			slices.Sort(gotAllowed)
			slices.Sort(wantAllowed)
			if !slices.Equal(gotAllowed, wantAllowed) {
				t.Errorf("allowed values = %v, want exact set %v (message %q)", gotAllowed, wantAllowed, message)
			}
		})
	}
}
