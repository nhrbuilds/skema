package skema

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

type intSchema struct {
	*baseValidator[int]
	node     *schemaNode
	coerce   bool
	min      *int
	max      *int
	positive bool
	nonNeg   bool
	negative bool
	multOf   *int
}

// Int validates JSON integers (rejects fractional numbers).
func Int() *intSchema {
	s := &intSchema{node: &schemaNode{types: []string{"integer"}}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *intSchema) Coerce() *intSchema { s.coerce = true; return s }
func (s *intSchema) Min(v int) *intSchema {
	s.min = &v
	f := float64(v)
	s.node.minimum = &f
	return s
}
func (s *intSchema) Max(v int) *intSchema {
	s.max = &v
	f := float64(v)
	s.node.maximum = &f
	return s
}
func (s *intSchema) Positive() *intSchema    { s.positive = true; return s }
func (s *intSchema) NonNegative() *intSchema { s.nonNeg = true; return s }
func (s *intSchema) Negative() *intSchema    { s.negative = true; return s }
func (s *intSchema) MultipleOf(v int) *intSchema {
	s.multOf = &v
	f := float64(v)
	s.node.multipleOf = &f
	return s
}
func (s *intSchema) Optional() Validator[*int]    { return Optional(s) }
func (s *intSchema) Default(v int) Validator[int] { return Default(s, v) }

func (s *intSchema) schemaNode() *schemaNode { return s.node }

func (s *intSchema) parse(ctx *parseContext, raw any) (int, error) {
	v, ok := parseIntRaw(ctx, raw, s.coerce)
	if !ok {
		return 0, nil
	}
	return s.check(ctx, v)
}

func (s *intSchema) check(ctx *parseContext, v int) (int, error) {
	if s.min != nil && v < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum "+itoa(*s.min))
	}
	if s.max != nil && v > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum "+itoa(*s.max))
	}
	if s.positive && v <= 0 {
		ctx.add(ctx.path, CodeTooSmall, "must be positive")
	}
	if s.nonNeg && v < 0 {
		ctx.add(ctx.path, CodeTooSmall, "must be non-negative")
	}
	if s.negative && v >= 0 {
		ctx.add(ctx.path, CodeTooBig, "must be negative")
	}
	if s.multOf != nil && *s.multOf != 0 && v%*s.multOf != 0 {
		ctx.add(ctx.path, CodeInvalidFormat, "must be multiple of "+itoa(*s.multOf))
	}
	return v, nil
}

type int64Schema struct {
	*baseValidator[int64]
	node   *schemaNode
	coerce bool
	min    *int64
	max    *int64
	multOf *int64
}

// Int64 validates 64-bit integers.
func Int64() *int64Schema {
	s := &int64Schema{node: &schemaNode{types: []string{"integer"}}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *int64Schema) Coerce() *int64Schema { s.coerce = true; return s }
func (s *int64Schema) Min(v int64) *int64Schema {
	s.min = &v
	f := float64(v)
	s.node.minimum = &f
	return s
}
func (s *int64Schema) Max(v int64) *int64Schema {
	s.max = &v
	f := float64(v)
	s.node.maximum = &f
	return s
}
func (s *int64Schema) MultipleOf(v int64) *int64Schema {
	s.multOf = &v
	f := float64(v)
	s.node.multipleOf = &f
	return s
}
func (s *int64Schema) Optional() Validator[*int64]      { return Optional(s) }
func (s *int64Schema) Default(v int64) Validator[int64] { return Default(s, v) }

func (s *int64Schema) parse(ctx *parseContext, raw any) (int64, error) {
	v, ok := parseInt64Raw(ctx, raw, s.coerce)
	if !ok {
		return 0, nil
	}
	if s.min != nil && v < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum value")
	}
	if s.max != nil && v > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum value")
	}
	if s.multOf != nil && *s.multOf != 0 && v%*s.multOf != 0 {
		ctx.add(ctx.path, CodeInvalidFormat, "must be multiple")
	}
	return v, nil
}

type float64Schema struct {
	*baseValidator[float64]
	node     *schemaNode
	coerce   bool
	min      *float64
	max      *float64
	positive bool
	nonNeg   bool
	negative bool
	multOf   *float64
}

// Float64 validates floating-point numbers.
func Float64() *float64Schema {
	s := &float64Schema{node: &schemaNode{types: []string{"number"}}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *float64Schema) Coerce() *float64Schema { s.coerce = true; return s }
func (s *float64Schema) Min(v float64) *float64Schema {
	s.min = &v
	s.node.minimum = &v
	return s
}
func (s *float64Schema) Max(v float64) *float64Schema {
	s.max = &v
	s.node.maximum = &v
	return s
}
func (s *float64Schema) Positive() *float64Schema    { s.positive = true; return s }
func (s *float64Schema) NonNegative() *float64Schema { s.nonNeg = true; return s }
func (s *float64Schema) Negative() *float64Schema    { s.negative = true; return s }
func (s *float64Schema) MultipleOf(v float64) *float64Schema {
	s.multOf = &v
	s.node.multipleOf = &v
	return s
}
func (s *float64Schema) Optional() Validator[*float64]        { return Optional(s) }
func (s *float64Schema) Default(v float64) Validator[float64] { return Default(s, v) }

func (s *float64Schema) parse(ctx *parseContext, raw any) (float64, error) {
	v, ok := parseFloatRaw(ctx, raw, s.coerce)
	if !ok {
		return 0, nil
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		ctx.add(ctx.path, CodeInvalidFormat, "must be finite number")
		return 0, nil
	}
	if s.min != nil && v < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum value")
	}
	if s.max != nil && v > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum value")
	}
	if s.positive && v <= 0 {
		ctx.add(ctx.path, CodeTooSmall, "must be positive")
	}
	if s.nonNeg && v < 0 {
		ctx.add(ctx.path, CodeTooSmall, "must be non-negative")
	}
	if s.negative && v >= 0 {
		ctx.add(ctx.path, CodeTooBig, "must be negative")
	}
	if s.multOf != nil && *s.multOf != 0 && !isFloatMultipleOf(v, *s.multOf) {
		ctx.add(ctx.path, CodeInvalidFormat, "must be multiple")
	}
	return v, nil
}

func parseIntRaw(ctx *parseContext, raw any, coerce bool) (int, bool) {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			if coerce {
				f, err2 := v.Float64()
				if err2 != nil || math.Trunc(f) != f {
					ctx.add(ctx.path, CodeInvalidType, "expected integer, got float")
					return 0, false
				}
				return intFromInt64(ctx, int64(f))
			}
			ctx.add(ctx.path, CodeInvalidType, "expected integer, got float")
			return 0, false
		}
		return intFromInt64(ctx, i)
	case float64:
		if math.Trunc(v) != v {
			ctx.add(ctx.path, CodeInvalidType, "expected integer, got float")
			return 0, false
		}
		return intFromInt64(ctx, int64(v))
	case int:
		return v, true
	case int64:
		return intFromInt64(ctx, v)
	case string:
		if !coerce {
			failType(ctx, "integer")
			return 0, false
		}
		s := strings.TrimSpace(v)
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return 0, false
		}
		return intFromInt64(ctx, i)
	default:
		failType(ctx, "integer")
		return 0, false
	}
}

func parseInt64Raw(ctx *parseContext, raw any, coerce bool) (int64, bool) {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected integer, got float")
			return 0, false
		}
		return i, true
	case float64:
		if math.Trunc(v) != v {
			ctx.add(ctx.path, CodeInvalidType, "expected integer, got float")
			return 0, false
		}
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		if !coerce {
			failType(ctx, "integer")
			return 0, false
		}
		s := strings.TrimSpace(v)
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return 0, false
		}
		return i, true
	default:
		failType(ctx, "integer")
		return 0, false
	}
}

func parseFloatRaw(ctx *parseContext, raw any, coerce bool) (float64, bool) {
	switch v := raw.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected number")
			return 0, false
		}
		return f, true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if !coerce {
			failType(ctx, "number")
			return 0, false
		}
		s := strings.TrimSpace(v)
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected number")
			return 0, false
		}
		return f, true
	default:
		failType(ctx, "number")
		return 0, false
	}
}
