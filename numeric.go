package skema

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// signedNum and unsignedNum constrain the fixed-width numeric schemas.
type signedNum interface {
	int8 | int16 | int32
}
type unsignedNum interface {
	uint8 | uint16 | uint32 | uint64 | uint
}
type numKind interface {
	signedNum | unsignedNum | float32
}

// numSchema validates fixed-width integers and float32. Existing Int/Int64/
// Float64 builders keep their dedicated implementations; these share one
// generic core.
type numSchema[T numKind] struct {
	*baseValidator[T]
	node     *schemaNode
	coerce   bool
	min      *T
	max      *T
	multOf   *T
	positive bool
	nonNeg   bool
	negative bool
	toT      func(ctx *parseContext, raw any, coerce bool) (T, bool)
}

func newNum[T numKind](node *schemaNode, toT func(ctx *parseContext, raw any, coerce bool) (T, bool)) *numSchema[T] {
	s := &numSchema[T]{node: node, toT: toT}
	s.baseValidator = bindValidator(node, s.parse)
	return s
}

func (s *numSchema[T]) Coerce() *numSchema[T] { s.coerce = true; return s }
func (s *numSchema[T]) Min(v T) *numSchema[T] {
	s.min = &v
	f := float64(v)
	s.node.minimum = &f
	return s
}
func (s *numSchema[T]) Max(v T) *numSchema[T] {
	s.max = &v
	f := float64(v)
	s.node.maximum = &f
	return s
}
func (s *numSchema[T]) Positive() *numSchema[T]    { s.positive = true; return s }
func (s *numSchema[T]) NonNegative() *numSchema[T] { s.nonNeg = true; return s }
func (s *numSchema[T]) Negative() *numSchema[T]    { s.negative = true; return s }
func (s *numSchema[T]) MultipleOf(v T) *numSchema[T] {
	s.multOf = &v
	f := float64(v)
	s.node.multipleOf = &f
	return s
}
func (s *numSchema[T]) Optional() Validator[*T]           { return Optional(s) }
func (s *numSchema[T]) Default(v T) Validator[T]          { return Default(s, v) }
func (s *numSchema[T]) Describe(text string) Validator[T] { return Describe(s, text) }
func (s *numSchema[T]) Refine(fn func(T) bool, msg string) Validator[T] {
	return Refine(s, fn, msg)
}

func (s *numSchema[T]) parse(ctx *parseContext, raw any) (T, error) {
	var zero T
	v, ok := s.toT(ctx, raw, s.coerce)
	if !ok {
		return zero, nil
	}
	if s.min != nil && v < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum "+floatString(float64(*s.min)))
	}
	if s.max != nil && v > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum "+floatString(float64(*s.max)))
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
	if s.multOf != nil && *s.multOf != 0 {
		if _, isFloat := any(*s.multOf).(float32); isFloat {
			if !isFloatMultipleOf(float64(v), float64(*s.multOf)) {
				ctx.add(ctx.path, CodeInvalidFormat, "must be multiple of "+floatString(float64(*s.multOf)))
			}
		} else {
			q := float64(v) / float64(*s.multOf)
			if q != math.Trunc(q) {
				ctx.add(ctx.path, CodeInvalidFormat, "must be multiple of "+floatString(float64(*s.multOf)))
			}
		}
	}
	return v, nil
}

// Int8 validates 8-bit integers.
func Int8() *numSchema[int8] {
	return newIntNum[int8](math.MinInt8, math.MaxInt8, "int8")
}

// Int16 validates 16-bit integers.
func Int16() *numSchema[int16] {
	return newIntNum[int16](math.MinInt16, math.MaxInt16, "int16")
}

// Int32 validates 32-bit integers.
func Int32() *numSchema[int32] {
	return newIntNum[int32](math.MinInt32, math.MaxInt32, "int32")
}

func newIntNum[T signedNum](lo, hi int64, name string) *numSchema[T] {
	node := &schemaNode{types: []string{"integer"}}
	return newNum[T](node, func(ctx *parseContext, raw any, coerce bool) (T, bool) {
		i, ok := parseInt64Raw(ctx, raw, coerce)
		if !ok {
			return 0, false
		}
		if i < lo || i > hi {
			ctx.add(ctx.path, CodeTooBig, "integer out of range for "+name)
			return 0, false
		}
		return T(i), true
	})
}

// Uint validates platform-dependent unsigned integers.
func Uint() *numSchema[uint] {
	return newUintNum[uint](math.MaxUint, "uint")
}

// Uint8 validates 8-bit unsigned integers.
func Uint8() *numSchema[uint8] {
	return newUintNum[uint8](math.MaxUint8, "uint8")
}

// Uint16 validates 16-bit unsigned integers.
func Uint16() *numSchema[uint16] {
	return newUintNum[uint16](math.MaxUint16, "uint16")
}

// Uint32 validates 32-bit unsigned integers.
func Uint32() *numSchema[uint32] {
	return newUintNum[uint32](math.MaxUint32, "uint32")
}

// Uint64 validates 64-bit unsigned integers (full 0..2^64-1 range).
func Uint64() *numSchema[uint64] {
	return newUintNum[uint64](math.MaxUint64, "uint64")
}

func newUintNum[T unsignedNum](hi uint64, name string) *numSchema[T] {
	node := &schemaNode{types: []string{"integer"}}
	return newNum[T](node, func(ctx *parseContext, raw any, coerce bool) (T, bool) {
		u, ok := parseIntUintRaw(ctx, raw, coerce)
		if !ok {
			return 0, false
		}
		if u > hi {
			ctx.add(ctx.path, CodeTooBig, "integer out of range for "+name)
			return 0, false
		}
		return T(u), true
	})
}

// Float32 validates 32-bit floating-point numbers.
func Float32() *numSchema[float32] {
	node := &schemaNode{types: []string{"number"}}
	return newNum[float32](node, func(ctx *parseContext, raw any, coerce bool) (float32, bool) {
		f, ok := parseFloatRaw(ctx, raw, coerce)
		if !ok {
			return 0, false
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			ctx.add(ctx.path, CodeInvalidFormat, "must be finite number")
			return 0, false
		}
		if f > math.MaxFloat32 || f < -math.MaxFloat32 {
			ctx.add(ctx.path, CodeTooBig, "number out of range for float32")
			return 0, false
		}
		return float32(f), true
	})
}

// parseIntUintRaw parses unsigned integers, accepting the full uint64 range.
func parseIntUintRaw(ctx *parseContext, raw any, coerce bool) (uint64, bool) {
	switch v := raw.(type) {
	case json.Number:
		if u, err := strconv.ParseUint(v.String(), 10, 64); err == nil {
			return u, true
		}
		f, err := v.Float64()
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return 0, false
		}
		// Reject negatives, fractions, and anything at or beyond 2^64:
		// float64 conversion of out-of-range values is undefined in Go.
		if f < 0 || f >= 18446744073709551616.0 || math.Trunc(f) != f {
			ctx.add(ctx.path, CodeInvalidType, "expected unsigned integer")
			return 0, false
		}
		return uint64(f), true
	case uint64:
		return v, true
	case int64:
		if v < 0 {
			ctx.add(ctx.path, CodeInvalidType, "expected unsigned integer")
			return 0, false
		}
		return uint64(v), true
	case int:
		if v < 0 {
			ctx.add(ctx.path, CodeInvalidType, "expected unsigned integer")
			return 0, false
		}
		return uint64(v), true
	case string:
		if !coerce {
			failType(ctx, "integer")
			return 0, false
		}
		u, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		if err != nil {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return 0, false
		}
		return u, true
	default:
		failType(ctx, "integer")
		return 0, false
	}
}

// BigInt validates arbitrary-precision JSON integers into math/big.Int.
// Without Coerce, only JSON number literals (and Go int/int64 values in
// ParseAny trees) are accepted; with Coerce, digit strings too. Fractional
// and exponential forms are rejected.
type bigIntSchema struct {
	*baseValidator[big.Int]
	node   *schemaNode
	coerce bool
}

// BigInt builds an arbitrary-precision integer validator.
func BigInt() *bigIntSchema {
	s := &bigIntSchema{node: &schemaNode{types: []string{"integer"}}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *bigIntSchema) Coerce() *bigIntSchema                   { s.coerce = true; return s }
func (s *bigIntSchema) Optional() Validator[*big.Int]           { return Optional(s) }
func (s *bigIntSchema) Nullable() Validator[*big.Int]           { return Nullable(s) }
func (s *bigIntSchema) OptionalNull() Validator[*big.Int]       { return OptionalNull(s) }
func (s *bigIntSchema) Describe(text string) Validator[big.Int] { return Describe(s, text) }
func (s *bigIntSchema) Refine(fn func(big.Int) bool, msg string) Validator[big.Int] {
	return Refine(s, fn, msg)
}

func (s *bigIntSchema) parse(ctx *parseContext, raw any) (big.Int, error) {
	var zero big.Int
	switch v := raw.(type) {
	case json.Number:
		var n big.Int
		if _, ok := n.SetString(v.String(), 10); !ok {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return zero, nil
		}
		return n, nil
	case int:
		return *big.NewInt(int64(v)), nil
	case int64:
		return *big.NewInt(v), nil
	case float64:
		if math.Trunc(v) != v {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return zero, nil
		}
		return *big.NewInt(int64(v)), nil
	case string:
		if !s.coerce {
			failType(ctx, "integer")
			return zero, nil
		}
		var n big.Int
		if _, ok := n.SetString(strings.TrimSpace(v), 10); !ok {
			ctx.add(ctx.path, CodeInvalidType, "expected integer")
			return zero, nil
		}
		return n, nil
	default:
		failType(ctx, "integer")
		return zero, nil
	}
}
