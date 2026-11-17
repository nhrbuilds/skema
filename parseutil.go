package skema

import (
	"encoding/json"
	"math"
	"strconv"
)

const floatModEpsilon = 1e-9

func intFromInt64(ctx *parseContext, i int64) (int, bool) {
	if i < int64(math.MinInt) || i > int64(math.MaxInt) {
		ctx.add(ctx.path, CodeTooBig, "integer out of range for int")
		return 0, false
	}
	return int(i), true
}

func isFloatMultipleOf(v, step float64) bool {
	if step == 0 {
		return false
	}
	q := v / step
	return math.Abs(q-math.Round(q)) < floatModEpsilon
}

func literalsEqual[T comparable](expected T, raw any) bool {
	if any(raw) == any(expected) {
		return true
	}
	switch e := any(expected).(type) {
	case int:
		return jsonNumberEqual(raw, float64(e)) || intEqual(raw, e)
	case int64:
		return jsonNumberEqual(raw, float64(e)) || int64Equal(raw, e)
	case float64:
		return jsonNumberEqual(raw, e)
	case bool:
		return boolEqual(raw, e)
	case string:
		s, ok := raw.(string)
		return ok && s == e
	default:
		return false
	}
}

func jsonNumberEqual(raw any, want float64) bool {
	switch v := raw.(type) {
	case json.Number:
		f, err := v.Float64()
		return err == nil && f == want
	case float64:
		return v == want
	case int:
		return float64(v) == want
	case int64:
		return float64(v) == want
	default:
		return false
	}
}

func intEqual(raw any, want int) bool {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		return err == nil && int(i) == want
	case float64:
		return v == float64(want) && math.Trunc(v) == v
	case int:
		return v == want
	case int64:
		return int(v) == want
	default:
		return false
	}
}

func int64Equal(raw any, want int64) bool {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		return err == nil && i == want
	case float64:
		return v == float64(want) && math.Trunc(v) == v
	case int64:
		return v == want
	case int:
		return int64(v) == want
	default:
		return false
	}
}

func boolEqual(raw any, want bool) bool {
	switch v := raw.(type) {
	case bool:
		return v == want
	case string:
		b, err := strconv.ParseBool(v)
		return err == nil && b == want
	default:
		return false
	}
}

func uniqueKeyJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "marshal_err"
	}
	return string(b)
}
