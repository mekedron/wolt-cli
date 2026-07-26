package deliveryselection

import "testing"

func TestParseDeliverySelection(t *testing.T) {
	tests := []struct {
		name          string
		payload       map[string]any
		wantSelected  string
		wantAvailable []string
		wantConfigID  string
		wantAmbiguous bool
	}{
		{
			name:          "default standard",
			payload:       map[string]any{},
			wantAvailable: []string{"standard"},
		},
		{
			name: "selected priority config",
			payload: map[string]any{
				"delivery_configs": []any{
					map[string]any{"id": "priority-config", "title": "Priority delivery", "selected": true},
				},
			},
			wantSelected:  "priority",
			wantAvailable: []string{"standard", "priority"},
			wantConfigID:  "priority-config",
		},
		{
			name: "nested explicit selection matches an unmarked config",
			payload: map[string]any{
				"checkout": map[string]any{
					"applied_delivery_mode": "priority",
					"delivery_configs": []any{
						map[string]any{"id": "standard-config", "type": "standard"},
						map[string]any{"id": "priority-config", "type": "priority"},
					},
				},
			},
			wantSelected:  "priority",
			wantAvailable: []string{"standard", "priority"},
			wantConfigID:  "priority-config",
		},
		{
			name: "conflicting nested explicit selections are ambiguous",
			payload: map[string]any{
				"first":  map[string]any{"selected_delivery_mode": "standard"},
				"second": map[string]any{"applied_delivery_mode": "priority"},
			},
			wantAvailable: []string{"standard", "priority"},
			wantAmbiguous: true,
		},
		{
			name: "explicit selection wins",
			payload: map[string]any{
				"selected_delivery_mode": "priority",
				"delivery_configs": []any{
					map[string]any{"id": "standard-config", "type": "standard", "selected": true},
					map[string]any{"id": "priority-config", "type": "priority", "selected": true},
				},
			},
			wantSelected:  "priority",
			wantAvailable: []string{"standard", "priority"},
			wantConfigID:  "priority-config",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Parse(test.payload)
			if got.SelectedMode != test.wantSelected {
				t.Fatalf("SelectedMode = %q, want %q", got.SelectedMode, test.wantSelected)
			}
			if len(got.AvailableModes) != len(test.wantAvailable) {
				t.Fatalf("AvailableModes = %#v, want %#v", got.AvailableModes, test.wantAvailable)
			}
			for index := range test.wantAvailable {
				if got.AvailableModes[index] != test.wantAvailable[index] {
					t.Fatalf("AvailableModes = %#v, want %#v", got.AvailableModes, test.wantAvailable)
				}
			}
			if test.wantConfigID != "" && got.SelectedConfig["id"] != test.wantConfigID {
				t.Fatalf("SelectedConfig = %#v, want id %q", got.SelectedConfig, test.wantConfigID)
			}
			if got.SelectionAmbiguous != test.wantAmbiguous {
				t.Fatalf("SelectionAmbiguous = %v, want %v", got.SelectionAmbiguous, test.wantAmbiguous)
			}
		})
	}
}

func TestParseDeliverySelectionIsDeterministic(t *testing.T) {
	payload := map[string]any{
		"z_priority": map[string]any{
			"delivery_configs": []any{
				map[string]any{"id": "priority-z", "type": "priority", "selected": true},
			},
		},
		"a_standard": map[string]any{
			"delivery_configs": []any{
				map[string]any{"id": "standard-a", "type": "standard", "selected": true},
			},
		},
	}
	for range 100 {
		got := Parse(payload)
		if !got.SelectionAmbiguous || got.SelectedMode != "" || got.SelectedConfig != nil {
			t.Fatalf("conflicting selections were not reported as ambiguous: %#v", got)
		}
	}
}
