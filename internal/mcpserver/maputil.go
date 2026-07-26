package mcpserver

import (
	"strconv"

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
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return 0
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
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func coalesceAny(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if s, ok := value.(string); ok && s == "" {
			continue
		}
		return value
	}
	return nil
}
