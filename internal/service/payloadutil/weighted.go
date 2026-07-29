package payloadutil

import (
	"fmt"
	"strings"
)

const (
	WeightedInputGrams         = "grams"
	WeightedInputNumberOfItems = "number_of_items"
)

// WeightConfig is the pricing information Wolt publishes for a weighted item.
type WeightConfig struct {
	InputType        string
	GramsPerStep     int
	PricePerKilogram int
}

// WeightedValues are the basket and checkout values for a selected weight.
type WeightedValues struct {
	Count int
	Grams int
	Price int
}

// WeightConfigFromItem returns a usable weighted-item configuration.
func WeightConfigFromItem(item map[string]any) (WeightConfig, bool) {
	raw := Map(item["sell_by_weight_config"])
	config := WeightConfig{
		InputType:        strings.TrimSpace(String(raw["input_type"])),
		GramsPerStep:     Int(raw["grams_per_step"]),
		PricePerKilogram: Int(raw["price_per_kg"]),
	}
	if config.InputType != WeightedInputGrams &&
		config.InputType != WeightedInputNumberOfItems {
		return WeightConfig{}, false
	}
	if config.GramsPerStep <= 0 || config.PricePerKilogram <= 0 {
		return WeightConfig{}, false
	}
	return config, true
}

// ValuesForSteps converts the quantity accepted by cart add into Wolt's
// weighted basket values. For gram input, one step is grams_per_step.
func (config WeightConfig) ValuesForSteps(steps int) (WeightedValues, error) {
	if steps <= 0 {
		return WeightedValues{}, fmt.Errorf("weighted item count must be greater than zero")
	}
	grams, ok := CheckedMultiplyInt(config.GramsPerStep, steps)
	if !ok {
		return WeightedValues{}, fmt.Errorf("weighted item amount exceeds the supported integer range")
	}
	count := steps
	if config.InputType == WeightedInputGrams {
		count = 1
	}
	return config.values(count, grams)
}

// ValuesFromBasket reads an existing weighted selection. Basket lines carry
// the purchased weight but never the input type, so the catalog config is the
// only authority on how the selection converts to a count.
func (config WeightConfig) ValuesFromBasket(line map[string]any) (WeightedValues, error) {
	info := Map(line["weighted_item_info"])
	grams := Int(info["purchased_weight_in_grams"])
	steps := Int(info["count"])
	if steps <= 0 {
		steps = Int(line["count"])
	}
	if steps <= 0 {
		steps = 1
	}
	if grams <= 0 {
		var ok bool
		grams, ok = CheckedMultiplyInt(config.GramsPerStep, steps)
		if !ok {
			return WeightedValues{}, fmt.Errorf("weighted item amount exceeds the supported integer range")
		}
	}
	count := steps
	if config.InputType == WeightedInputGrams {
		count = 1
	}
	return config.values(count, grams)
}

func (config WeightConfig) values(count, grams int) (WeightedValues, error) {
	if count <= 0 || grams <= 0 {
		return WeightedValues{}, fmt.Errorf("weighted item amount must be greater than zero")
	}
	product, ok := CheckedMultiplyInt(config.PricePerKilogram, grams)
	if !ok {
		return WeightedValues{}, fmt.Errorf("weighted item price exceeds the supported integer range")
	}
	product, ok = CheckedAddInt(product, 500)
	if !ok {
		return WeightedValues{}, fmt.Errorf("weighted item price exceeds the supported integer range")
	}
	return WeightedValues{Count: count, Grams: grams, Price: product / 1000}, nil
}

func (config WeightConfig) itemInfo(values WeightedValues) map[string]any {
	return map[string]any{
		"count":                     values.Count,
		"purchased_weight_in_grams": values.Grams,
		"weighted_item_input_type":  config.InputType,
	}
}

// BuildWeightedBasketItem builds the exact compact weighted line accepted by
// Wolt's basket endpoint.
func BuildWeightedBasketItem(line map[string]any, steps int, config WeightConfig) (map[string]any, error) {
	values, err := config.ValuesForSteps(steps)
	if err != nil {
		return nil, err
	}
	copy := make(map[string]any, len(line)+2)
	for key, value := range line {
		copy[key] = value
	}
	copy["count"] = values.Count
	copy["price"] = values.Price
	copy["weighted_item_info"] = config.itemInfo(values)
	return BuildBasketUpsertItem(copy, values.Count)
}

// MergeWeightedBasketItems preserves the complete basket replacement while
// increasing one weighted selection.
func MergeWeightedBasketItems(
	basket map[string]any,
	addedItemID string,
	addedSteps int,
	newLine map[string]any,
	config WeightConfig,
) ([]any, error) {
	replacement, err := BuildWeightedBasketItem(newLine, addedSteps, config)
	if err != nil {
		return nil, err
	}
	if basket == nil {
		return []any{replacement}, nil
	}
	existing, err := basketReplacementItems(basket)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(existing)+1)
	for index, line := range existing {
		if !strings.EqualFold(strings.TrimSpace(String(line["id"])), strings.TrimSpace(addedItemID)) ||
			!basketLineConfigurationEqual(line, replacement) {
			out = append(out, line)
			continue
		}
		current, valuesErr := config.ValuesFromBasket(line)
		if valuesErr != nil {
			return nil, valuesErr
		}
		added, valuesErr := config.ValuesForSteps(addedSteps)
		if valuesErr != nil {
			return nil, valuesErr
		}
		if config.InputType == WeightedInputGrams {
			grams, ok := CheckedAddInt(current.Grams, added.Grams)
			if !ok {
				return nil, fmt.Errorf("weighted item amount exceeds the supported integer range")
			}
			values, valuesErr := config.values(1, grams)
			if valuesErr != nil {
				return nil, valuesErr
			}
			replacement["count"] = values.Count
			replacement["price"] = values.Price
			replacement["weighted_item_info"] = config.itemInfo(values)
		} else {
			steps, ok := CheckedAddInt(current.Count, addedSteps)
			if !ok {
				return nil, fmt.Errorf("weighted item count exceeds the supported integer range")
			}
			replacement, valuesErr = BuildWeightedBasketItem(newLine, steps, config)
			if valuesErr != nil {
				return nil, valuesErr
			}
		}
		out = append(out, replacement)
		for _, trailing := range existing[index+1:] {
			out = append(out, trailing)
		}
		return out, nil
	}
	return append(out, replacement), nil
}
