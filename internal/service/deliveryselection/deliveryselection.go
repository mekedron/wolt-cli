package deliveryselection

import (
	"reflect"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// State describes the delivery modes advertised and selected by a checkout
// preview response.
type State struct {
	AvailableModes     []string
	SelectedMode       string
	SelectedConfig     map[string]any
	SelectionAmbiguous bool
}

// Parse accepts the nested checkout shapes currently emitted by Wolt.
func Parse(payload map[string]any) State {
	collector := selectionCollector{
		available:       map[string]bool{"standard": true},
		explicitModes:   map[string]struct{}{},
		configs:         map[string][]map[string]any{},
		selectedConfigs: map[string][]map[string]any{},
	}
	collector.collect(payload)

	selectedMode, selectedConfig, ambiguous := collector.selection()
	modes := []string{"standard"}
	if collector.available["priority"] {
		modes = append(modes, "priority")
	}
	return State{
		AvailableModes:     modes,
		SelectedMode:       selectedMode,
		SelectedConfig:     selectedConfig,
		SelectionAmbiguous: ambiguous,
	}
}

type selectionCollector struct {
	available       map[string]bool
	explicitModes   map[string]struct{}
	configs         map[string][]map[string]any
	selectedConfigs map[string][]map[string]any
}

func (c *selectionCollector) collect(raw any) {
	switch value := raw.(type) {
	case map[string]any:
		c.collectExplicitSelection(value)
		c.collectAvailableModes(value)
		c.collectConfigs(value)
		keys := make([]string, 0, len(value))
		for key := range value {
			if key != "delivery_configs" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			c.collect(value[key])
		}
	case []any:
		for _, nested := range value {
			c.collect(nested)
		}
	}
}

func (c *selectionCollector) collectExplicitSelection(value map[string]any) {
	for _, key := range []string{"selected_delivery_mode", "applied_delivery_mode"} {
		if mode := modeFromLabel(payloadutil.String(value[key])); mode != "" {
			c.explicitModes[mode] = struct{}{}
			c.available[mode] = true
		}
	}
}

func (c *selectionCollector) collectAvailableModes(value map[string]any) {
	for _, rawMode := range payloadutil.Slice(value["available_delivery_modes"]) {
		if mode := modeFromLabel(payloadutil.String(rawMode)); mode != "" {
			c.available[mode] = true
		}
	}
}

func (c *selectionCollector) collectConfigs(value map[string]any) {
	for _, rawConfig := range payloadutil.Slice(value["delivery_configs"]) {
		config := payloadutil.Map(rawConfig)
		mode := modeFromConfig(config)
		if mode == "" {
			continue
		}
		c.available[mode] = true
		c.configs[mode] = appendUniqueConfig(c.configs[mode], config)
		if payloadutil.Bool(config["selected"]) ||
			payloadutil.Bool(config["is_selected"]) ||
			payloadutil.Bool(config["active"]) {
			c.selectedConfigs[mode] = appendUniqueConfig(c.selectedConfigs[mode], config)
		}
	}
}

func (c *selectionCollector) selection() (string, map[string]any, bool) {
	if len(c.explicitModes) > 0 {
		mode, unique := onlyMode(c.explicitModes)
		if !unique {
			return "", nil, true
		}
		return mode, c.configForExplicitMode(mode), false
	}

	selectedModes := map[string]struct{}{}
	for mode, configs := range c.selectedConfigs {
		if len(configs) > 0 {
			selectedModes[mode] = struct{}{}
		}
	}
	mode, unique := onlyMode(selectedModes)
	if !unique {
		return "", nil, len(selectedModes) > 1
	}
	configs := c.selectedConfigs[mode]
	if len(configs) != 1 {
		return mode, nil, false
	}
	return mode, configs[0], false
}

func (c *selectionCollector) configForExplicitMode(mode string) map[string]any {
	if selected := c.selectedConfigs[mode]; len(selected) == 1 {
		return selected[0]
	}
	if matching := c.configs[mode]; len(matching) == 1 {
		return matching[0]
	}
	return nil
}

func onlyMode(modes map[string]struct{}) (string, bool) {
	if len(modes) != 1 {
		return "", false
	}
	for mode := range modes {
		return mode, true
	}
	return "", false
}

func appendUniqueConfig(configs []map[string]any, candidate map[string]any) []map[string]any {
	for _, existing := range configs {
		if reflect.DeepEqual(existing, candidate) {
			return configs
		}
	}
	return append(configs, candidate)
}

func modeFromConfig(config map[string]any) string {
	for _, value := range []any{
		config["delivery_mode"],
		config["type"],
		config["mode"],
		config["id"],
		config["name"],
		config["title"],
		config["label"],
	} {
		if mode := modeFromLabel(payloadutil.String(value)); mode != "" {
			return mode
		}
	}
	return ""
}

func modeFromLabel(raw string) string {
	label := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(label, "priority"):
		return "priority"
	case strings.Contains(label, "standard"):
		return "standard"
	default:
		return ""
	}
}
