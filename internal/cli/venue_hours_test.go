package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mekedron/wolt-cli/internal/domain"
)

func TestVenueHoursAcceptsSlugIDAndURLWithoutLocationOrRetiredEndpoint(t *testing.T) {
	for _, input := range []string{
		cliVenueSlug,
		cliVenueID,
		cliVenueURL + "/itemid-000000000000000000000072",
	} {
		t.Run(input, func(t *testing.T) {
			withIsolatedSlugCache(t)
			api := &testWoltAPI{
				venuePageStaticFn: func(context.Context, string) (map[string]any, error) {
					return cliVenueStaticPayload(), nil
				},
			}
			cmd := newVenueHoursCommand(Dependencies{
				Wolt: api,
				Profiles: &testProfiles{profile: domain.Profile{
					Name:      "default",
					IsDefault: true,
				}},
			})
			output := &bytes.Buffer{}
			cmd.SetOut(output)
			cmd.SetErr(output)
			cmd.SetArgs([]string{
				input,
				"--timezone", "America/New_York",
				"--format", "json",
			})

			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("venue hours: %v\n%s", err, output.String())
			}
			data := decodeCLIData(t, output)
			if asString(data["venue_id"]) != cliVenueID ||
				asString(data["slug"]) != cliVenueSlug ||
				asString(data["timezone"]) != "Europe/Paris" ||
				asString(data["canonical_url"]) != cliVenueURL {
				t.Fatalf("hours identity/timezone = %#v", data)
			}
			if len(asSlice(data["opening_windows"])) != 2 {
				t.Fatalf("opening_windows = %#v", data["opening_windows"])
			}
			var envelope map[string]any
			if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			foundOverrideWarning := false
			for _, raw := range asSlice(envelope["warnings"]) {
				if strings.Contains(asString(raw), "was not applied") {
					foundOverrideWarning = true
				}
			}
			if !foundOverrideWarning {
				t.Fatalf("warnings = %#v", envelope["warnings"])
			}
		})
	}
}
