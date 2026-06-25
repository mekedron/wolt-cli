package cli

import (
	"github.com/mekedron/wolt-cli/internal/service/payloadutil"
)

func asMap(value any) map[string]any {
	return payloadutil.Map(value)
}

func asSlice(value any) []any {
	return payloadutil.Slice(value)
}

func asString(value any) string {
	return payloadutil.String(value)
}

func asBool(value any) bool {
	return payloadutil.Bool(value)
}

func asInt(value any) int {
	return payloadutil.Int(value)
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
