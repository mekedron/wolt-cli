package deliveryselection

import (
	"reflect"
	"sort"
	"strings"

	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

// State describes the delivery modes advertised and selected by a checkout
// preview response.
//
// SelectedMode is empty whenever upstream advertises modes without naming an
// active one. Wolt's checkout preview normally does exactly that: it returns one
// delivery_configs entry per offered mode and carries no selection flag, because
// the mode is chosen by the purchase plan that was posted. Treat an empty
// SelectedMode as "upstream did not contradict the request", not as a failure —
// Resolve encodes that rule.
type State struct {
	AvailableModes     []string
	SelectedMode       string
	SelectedConfig     map[string]any
	SelectionAmbiguous bool
	// configsByMode holds the advertised config per mode when exactly one was
	// offered for it, so a requested mode can be reported with its pricing.
	configsByMode map[string]map[string]any
}

// Parse accepts the nested checkout shapes currently emitted by Wolt.
func Parse(payload map[string]any) State {
	collector := selectionCollector{
		available:       map[string]bool{},
		explicitModes:   map[string]struct{}{},
		configs:         map[string][]map[string]any{},
		selectedConfigs: map[string][]map[string]any{},
	}
	collector.collect(payload)

	selectedMode, selectedConfig, ambiguous := collector.selection()
	modes := []string{}
	for _, mode := range []string{"standard", "priority"} {
		if collector.available[mode] {
			modes = append(modes, mode)
		}
	}
	// A payload that advertises no delivery metadata still supports the default
	// mode; anything else would report a venue as undeliverable on a field Wolt
	// simply omitted.
	if len(modes) == 0 {
		modes = []string{"standard"}
	}
	configsByMode := map[string]map[string]any{}
	for mode, configs := range collector.configs {
		if len(configs) == 1 {
			configsByMode[mode] = configs[0]
		}
	}
	return State{
		AvailableModes:     modes,
		SelectedMode:       selectedMode,
		SelectedConfig:     selectedConfig,
		SelectionAmbiguous: ambiguous,
		configsByMode:      configsByMode,
	}
}

// Resolve reports the mode that applies to a requested mode, along with the
// advertised config for it when upstream priced exactly one.
//
// It fails only on evidence that the request was not honored: conflicting
// explicit selections, an explicit selection naming a different mode, or a mode
// the response never advertised. Absent any such signal the request stands, so a
// preview is not discarded over a selection flag Wolt does not send.
func (s State) Resolve(requested string) (string, map[string]any, bool) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = "standard"
	}
	if s.SelectionAmbiguous {
		return "", nil, false
	}
	if s.SelectedMode != "" {
		if s.SelectedMode != requested {
			return "", nil, false
		}
		return requested, s.configForMode(requested), true
	}
	for _, mode := range s.AvailableModes {
		if mode == requested {
			return requested, s.configForMode(requested), true
		}
	}
	return "", nil, false
}

func (s State) configForMode(mode string) map[string]any {
	if s.SelectedMode == mode && s.SelectedConfig != nil {
		return s.SelectedConfig
	}
	return s.configsByMode[mode]
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

// modeFromConfig reads the machine-readable discriminators before any display
// copy. Wolt localizes label/name/title/description — a Finnish response labels
// standard delivery "Normaali" — so matching those first makes mode detection
// depend on the response locale. `schedule` carries the stable mode slug.
func modeFromConfig(config map[string]any) string {
	stable := []any{
		config["schedule"],
		config["delivery_mode"],
		config["type"],
		config["mode"],
		config["id"],
	}
	localized := []any{
		config["name"],
		config["title"],
		config["label"],
	}
	for _, value := range append(stable, localized...) {
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
