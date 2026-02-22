package cli

import (
	"strings"
	"testing"
)

func TestExtractWoltPlusSubscriberFromPrimaryFlag(t *testing.T) {
	payload := map[string]any{
		"user": map[string]any{
			"is_wolt_plus_subscriber": true,
		},
	}

	subscriber, ok := extractWoltPlusSubscriber(payload)
	if !ok {
		t.Fatal("expected wolt plus subscriber signal to be detected")
	}
	if !subscriber {
		t.Fatalf("expected subscriber=true, got %v", subscriber)
	}
}

func TestExtractWoltPlusSubscriberFromNestedStatus(t *testing.T) {
	payload := map[string]any{
		"user": map[string]any{
			"wolt_plus": map[string]any{
				"status": "active",
			},
		},
	}

	subscriber, ok := extractWoltPlusSubscriber(payload)
	if !ok {
		t.Fatal("expected nested wolt plus status to be detected")
	}
	if !subscriber {
		t.Fatalf("expected subscriber=true, got %v", subscriber)
	}
}

func TestBuildAuthStatusTableIncludesWoltPlusSubscriber(t *testing.T) {
	table := buildAuthStatusTable(map[string]any{
		"authenticated":        true,
		"wolt_plus_subscriber": true,
		"user_id":              "user-1",
		"country":              "FIN",
		"session_expires_at":   "2026-02-20T12:00:00Z",
	})

	if !strings.Contains(table, "Wolt+ subscriber\tyes") {
		t.Fatalf("expected table to include Wolt+ row, got:\n%s", table)
	}
}
