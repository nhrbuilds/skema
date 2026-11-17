package skema

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var (
	timeType   = reflect.TypeOf(time.Time{})
	bigIntType = reflect.TypeOf(big.Int{})
	anyType    = reflect.TypeOf((*any)(nil)).Elem()
)

// FromStruct builds a struct validator from `sk:"..."` struct tags instead of
// explicit Field calls. JSON names follow the json tag (untagged fields use
// snake_case; `json:"-"` and anonymous fields are skipped; `json:...,omitempty`
// implies optional).
//
// Tag tokens (comma-separated): optional, nullable, coerce, trim, lower,
// upper, nonempty, positive, nonnegative, negative, unique; formats email,
// url, uuid, uuid4, uuid6, uuid7, datetime, date, ipv4, ipv6, base64, cidr,
// jwt; min=N, max=N (length for strings/slices, key count for maps, bounds
// for numbers), len=N, multiple=N, pattern=RE, prefix=X, suffix=X;
// enum=v1|v2 (string fields; "|" separated — "," splits tokens); literal=x;
// default=x. Unknown tokens panic at build time.
//
// Supported field types: string, bool, all int/uint widths, float32/64,
// time.Time, big.Int, slices/maps of those plus any, any/interface{}, and
// pointers to them. Nested struct fields require their schema to be
// registered via RegisterTagSchema (Go generics cannot be instantiated from
// reflection). For shapes tags cannot express, build with Struct/Field.
func FromStruct[T any]() *structValidator[T] {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt.Kind() != reflect.Struct {
		panic(fmt.Sprintf("skema.FromStruct: %T must be a struct", zero))
	}
	s := &structSchema[T]{
		node: &schemaNode{
			types:      []string{"object"},
			properties: map[string]*schemaNode{},
		},
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" || f.Anonymous {
			continue
		}
		jsonName := jsonTagName(f)
		if jsonName == "" {
			continue // json:"-"
		}
		rules, err := parseSKTag(f.Tag.Get("sk"))
		if err != nil {
			panic(fmt.Sprintf("skema.FromStruct: field %s: %v", f.Name, err))
		}
		if tagHasOmitempty(f) {
			rules.optional = true
		}
		fb, err := bindTagField[T](jsonName, f, rules)
		if err != nil {
			panic(fmt.Sprintf("skema.FromStruct: field %s: %v", f.Name, err))
		}
		s.fields = append(s.fields, *fb)
	}
	s.rebuildNode()
	return &structValidator[T]{
		baseValidator: bindValidator[T](s.node, s.parse),
		schema:        s,
	}
}

var tagSchemaRegistry sync.Map // reflect.Type -> Validator[any]

// RegisterTagSchema registers a schema for a struct type so FromStruct can
// bind nested struct fields of that type. Nested types may themselves be
// built with FromStruct or with explicit Struct/Field calls.
func RegisterTagSchema[T any](v Validator[T]) {
	tagSchemaRegistry.Store(reflect.TypeOf(*new(T)), Lift(v))
}

// --- tag parsing ---

type tagRules struct {
	optional   bool
	nullable   bool
	coerce     bool
	trim       bool
	lower      bool
	upper      bool
	nonEmpty   bool
	positive   bool
	nonNeg     bool
	negative   bool
	unique     bool
	format     string
	minStr     string
	maxStr     string
	lenStr     string
	multiple   string
	pattern    string
	prefix     string
	suffix     string
	enum       []string
	literal    string
	defaultVal string
}

var tagFormats = map[string]bool{
	"email": true, "url": true, "uuid": true, "uuid4": true, "uuid6": true,
	"uuid7": true, "datetime": true, "date": true, "ipv4": true, "ipv6": true,
	"base64": true, "cidr": true, "jwt": true,
}

func parseSKTag(tag string) (tagRules, error) {
	var r tagRules
	if strings.TrimSpace(tag) == "" {
		return r, nil
	}
	for _, tok := range strings.Split(tag, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		key, val, hasVal := strings.Cut(tok, "=")
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "optional":
			r.optional = true
		case "nullable":
			r.nullable = true
		case "required": // the default; explicit for readability
		case "coerce":
			r.coerce = true
		case "trim":
			r.trim = true
		case "lower":
			r.lower = true
		case "upper":
			r.upper = true
		case "nonempty":
			r.nonEmpty = true
		case "positive":
			r.positive = true
		case "nonnegative":
			r.nonNeg = true
		case "negative":
			r.negative = true
		case "unique":
			r.unique = true
		case "min", "max", "len", "multiple":
			if !hasVal || val == "" {
				return r, fmt.Errorf("%q requires a value", key)
			}
			if _, err := strconv.ParseFloat(val, 64); err != nil {
				return r, fmt.Errorf("%q: invalid number %q", key, val)
			}
			switch key {
			case "min":
				r.minStr = val
			case "max":
				r.maxStr = val
			case "len":
				r.lenStr = val
			case "multiple":
				r.multiple = val
			}
		case "pattern":
			if _, err := regexp.Compile(val); err != nil {
				return r, fmt.Errorf("invalid pattern: %v", err)
			}
			r.pattern = val
		case "prefix":
			r.prefix = val
		case "suffix":
			r.suffix = val
		case "enum":
			if !hasVal || val == "" {
				return r, fmt.Errorf("enum requires values (enum=v1|v2)")
			}
			for _, e := range strings.Split(val, "|") {
				e = strings.TrimSpace(e)
				if e == "" {
					return r, fmt.Errorf("enum contains an empty value")
				}
				r.enum = append(r.enum, e)
			}
		case "literal":
			if !hasVal {
				return r, fmt.Errorf("literal requires a value")
			}
			r.literal = val
		case "default":
			if !hasVal {
				return r, fmt.Errorf("default requires a value")
			}
			r.defaultVal = val
		default:
			if tagFormats[key] {
				if hasVal {
					return r, fmt.Errorf("format %q takes no value", key)
				}
				r.format = key
				continue
			}
			return r, fmt.Errorf("unknown tag token %q", key)
		}
	}
	return r, nil
}

func tagHasOmitempty(f reflect.StructField) bool {
	for _, part := range strings.Split(f.Tag.Get("json"), ",")[1:] {
		if strings.TrimSpace(part) == "omitempty" {
			return true
		}
	}
	return false
}

// --- field binding ---

func bindTagField[T any](jsonName string, f reflect.StructField, r tagRules) (*fieldBinding, error) {
	ft := f.Type
	if ft.Kind() == reflect.Pointer {
		return bindPointerField[T](jsonName, f.Offset, ft.Elem(), r)
	}
	switch ft.Kind() {
	case reflect.String:
		v, err := buildStringValidator(r)
		if err != nil {
			return nil, err
		}
		return bindValueField[T, string](jsonName, f.Offset, v, r)
	case reflect.Bool:
		v, err := buildBoolValidator(r)
		if err != nil {
			return nil, err
		}
		return bindValueField[T, bool](jsonName, f.Offset, v, r)
	case reflect.Int:
		v, err := buildIntValidator(r)
		if err != nil {
			return nil, err
		}
		return bindValueField[T, int](jsonName, f.Offset, v, r)
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		v, err := buildWidthValidator(ft, r)
		if err != nil {
			return nil, err
		}
		return bindWidthField[T](jsonName, f.Offset, ft, v, r)
	case reflect.Int64:
		if ft == timeType {
			return bindValueField[T, time.Time](jsonName, f.Offset, Time(), r)
		}
		v, err := buildInt64Validator(r)
		if err != nil {
			return nil, err
		}
		return bindValueField[T, int64](jsonName, f.Offset, v, r)
	case reflect.Uint64:
		v, err := buildUint64Validator(r)
		if err != nil {
			return nil, err
		}
		return bindValueField[T, uint64](jsonName, f.Offset, v, r)
	case reflect.Float32, reflect.Float64:
		v, err := buildFloatValidator(ft, r)
		if err != nil {
			return nil, err
		}
		if ft.Kind() == reflect.Float32 {
			return bindValueField[T, float32](jsonName, f.Offset, v.(Validator[float32]), r)
		}
		return bindValueField[T, float64](jsonName, f.Offset, v.(Validator[float64]), r)
	case reflect.Slice:
		return bindSliceField[T](jsonName, f.Offset, ft.Elem(), r)
	case reflect.Map:
		if ft.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type %s", ft.Key())
		}
		return bindMapField[T](jsonName, f.Offset, ft.Elem(), r)
	case reflect.Struct:
		switch ft {
		case timeType:
			return bindValueField[T, time.Time](jsonName, f.Offset, Time(), r)
		case bigIntType:
			v, err := buildBigIntValidator(r)
			if err != nil {
				return nil, err
			}
			return bindValueField[T, big.Int](jsonName, f.Offset, v, r)
		default:
			return bindRegistryField[T](jsonName, f, r, false)
		}
	case reflect.Interface:
		if ft != anyType {
			return nil, fmt.Errorf("unsupported interface type %s", ft)
		}
		return bindValueField[T, any](jsonName, f.Offset, Any(), r)
	default:
		return nil, fmt.Errorf("unsupported field type %s", ft)
	}
}

func bindPointerField[T any](jsonName string, offset uintptr, elem reflect.Type, r tagRules) (*fieldBinding, error) {
	if r.defaultVal != "" {
		return nil, fmt.Errorf("default is not supported on pointer fields")
	}
	switch elem.Kind() {
	case reflect.String:
		iv, err := buildStringValidator(r)
		if err != nil {
			return nil, err
		}
		return bindPointer[T, string](jsonName, offset, iv, r)
	case reflect.Bool:
		return bindPointer[T, bool](jsonName, offset, Bool(), r)
	case reflect.Int:
		iv, err := buildIntValidator(r)
		if err != nil {
			return nil, err
		}
		return bindPointer[T, int](jsonName, offset, iv, r)
	case reflect.Int64:
		if elem == timeType {
			return bindPointerTime[T](jsonName, offset, r)
		}
		iv, err := buildInt64Validator(r)
		if err != nil {
			return nil, err
		}
		return bindPointer[T, int64](jsonName, offset, iv, r)
	case reflect.Uint64:
		iv, err := buildUint64Validator(r)
		if err != nil {
			return nil, err
		}
		return bindPointer[T, uint64](jsonName, offset, iv, r)
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		iv, err := buildWidthValidator(elem, r)
		if err != nil {
			return nil, err
		}
		return bindWidthPointer[T](jsonName, offset, elem, iv, r)
	case reflect.Float64:
		return bindPointer[T, float64](jsonName, offset, Float64(), r)
	case reflect.Float32:
		return bindPointer[T, float32](jsonName, offset, Float32(), r)
	case reflect.Struct:
		if elem == timeType {
			return bindPointerTime[T](jsonName, offset, r)
		}
		if elem == bigIntType {
			iv, err := buildBigIntValidator(r)
			if err != nil {
				return nil, err
			}
			return bindPointer[T, big.Int](jsonName, offset, iv, r)
		}
		return bindRegistryField[T](jsonName, reflect.StructField{Type: elem, Offset: offset, Name: jsonName}, r, true)
	case reflect.Interface:
		if elem != anyType {
			return nil, fmt.Errorf("unsupported pointer element type %s", elem)
		}
		return bindPointer[T, any](jsonName, offset, Any(), r)
	default:
		return nil, fmt.Errorf("unsupported pointer element type %s", elem)
	}
}

// bindValueField binds a value-typed field, applying the optional tag via
// OptionalValue (which keeps the field type).
func bindValueField[T any, F any](jsonName string, offset uintptr, v Validator[F], r tagRules) (*fieldBinding, error) {
	if r.optional && r.defaultVal == "" {
		v = OptionalValue(v)
	}
	return bindTyped[T, F](jsonName, offset, v), nil
}

func bindTyped[T any, F any](jsonName string, offset uintptr, v Validator[F]) *fieldBinding {
	node := v.schemaNode()
	fb := &fieldBinding{
		jsonName: jsonName,
		offset:   offset,
		optional: node.optional,
		nullable: node.nullable,
		node:     node,
		parse: func(ctx *parseContext, raw any, base unsafe.Pointer) {
			dest := (*F)(unsafe.Pointer(uintptr(base) + offset))
			child := ctx.child(jsonName)
			val, err := v.parse(child, raw)
			if err != nil {
				return
			}
			if len(child.issues.Issues) > 0 {
				return
			}
			*dest = val
		},
	}
	fieldParseRaw(jsonName, offset, v, fb)
	return fb
}

func bindPointer[T any, E any](jsonName string, offset uintptr, inner Validator[E], r tagRules) (*fieldBinding, error) {
	var v Validator[*E]
	switch {
	case r.optional && r.nullable:
		v = OptionalNull(inner)
	case r.optional:
		v = Optional(inner)
	case r.nullable:
		v = Nullable(inner)
	default:
		v = requiredPointer(inner)
	}
	return bindTyped[T, *E](jsonName, offset, v), nil
}

func bindPointerTime[T any](jsonName string, offset uintptr, r tagRules) (*fieldBinding, error) {
	return bindPointer[T, time.Time](jsonName, offset, Time(), r)
}

func bindWidthPointer[T any](jsonName string, offset uintptr, elem reflect.Type, v any, r tagRules) (*fieldBinding, error) {
	switch elem.Kind() {
	case reflect.Int8:
		return bindPointer[T, int8](jsonName, offset, v.(Validator[int8]), r)
	case reflect.Int16:
		return bindPointer[T, int16](jsonName, offset, v.(Validator[int16]), r)
	case reflect.Int32:
		return bindPointer[T, int32](jsonName, offset, v.(Validator[int32]), r)
	case reflect.Uint:
		return bindPointer[T, uint](jsonName, offset, v.(Validator[uint]), r)
	case reflect.Uint8:
		return bindPointer[T, uint8](jsonName, offset, v.(Validator[uint8]), r)
	case reflect.Uint16:
		return bindPointer[T, uint16](jsonName, offset, v.(Validator[uint16]), r)
	case reflect.Uint32:
		return bindPointer[T, uint32](jsonName, offset, v.(Validator[uint32]), r)
	}
	return nil, fmt.Errorf("unsupported width type %s", elem)
}

func bindWidthField[T any](jsonName string, offset uintptr, elem reflect.Type, v any, r tagRules) (*fieldBinding, error) {
	switch elem.Kind() {
	case reflect.Int8:
		return bindValueField[T, int8](jsonName, offset, v.(Validator[int8]), r)
	case reflect.Int16:
		return bindValueField[T, int16](jsonName, offset, v.(Validator[int16]), r)
	case reflect.Int32:
		return bindValueField[T, int32](jsonName, offset, v.(Validator[int32]), r)
	case reflect.Uint:
		return bindValueField[T, uint](jsonName, offset, v.(Validator[uint]), r)
	case reflect.Uint8:
		return bindValueField[T, uint8](jsonName, offset, v.(Validator[uint8]), r)
	case reflect.Uint16:
		return bindValueField[T, uint16](jsonName, offset, v.(Validator[uint16]), r)
	case reflect.Uint32:
		return bindValueField[T, uint32](jsonName, offset, v.(Validator[uint32]), r)
	}
	return nil, fmt.Errorf("unsupported width type %s", elem)
}

func bindSliceField[T any](jsonName string, offset uintptr, elem reflect.Type, r tagRules) (*fieldBinding, error) {
	var fb *fieldBinding
	var err error
	switch elem.Kind() {
	case reflect.String:
		fb, err = bindTypedE[T, []string](jsonName, offset, r, func() Validator[[]string] { return sliceOf(r, String()) })
	case reflect.Bool:
		fb, err = bindTypedE[T, []bool](jsonName, offset, r, func() Validator[[]bool] { return sliceOf(r, Bool()) })
	case reflect.Int:
		fb, err = bindTypedE[T, []int](jsonName, offset, r, func() Validator[[]int] { return sliceOf(r, Int()) })
	case reflect.Int64:
		fb, err = bindTypedE[T, []int64](jsonName, offset, r, func() Validator[[]int64] { return sliceOf(r, Int64()) })
	case reflect.Float64:
		fb, err = bindTypedE[T, []float64](jsonName, offset, r, func() Validator[[]float64] { return sliceOf(r, Float64()) })
	case reflect.Interface:
		if elem != anyType {
			err = fmt.Errorf("unsupported slice element type %s", elem)
			break
		}
		fb, err = bindTypedE[T, []any](jsonName, offset, r, func() Validator[[]any] { return sliceOf(r, Any()) })
	default:
		err = fmt.Errorf("unsupported slice element type %s", elem)
	}
	return fb, err
}

func bindTypedE[T any, F any](jsonName string, offset uintptr, r tagRules, build func() Validator[F]) (*fieldBinding, error) {
	v := build()
	if r.optional && r.defaultVal == "" {
		v = OptionalValue(v)
	}
	return bindTyped[T, F](jsonName, offset, v), nil
}

func sliceOf[F any](r tagRules, elem Validator[F]) Validator[[]F] {
	s := Slice(elem)
	if r.minStr != "" {
		f, _ := strconv.ParseFloat(r.minStr, 64)
		s.Min(int(f))
	}
	if r.maxStr != "" {
		f, _ := strconv.ParseFloat(r.maxStr, 64)
		s.Max(int(f))
	}
	if r.unique {
		s.Unique()
	}
	return s
}

func bindMapField[T any](jsonName string, offset uintptr, elem reflect.Type, r tagRules) (*fieldBinding, error) {
	switch elem.Kind() {
	case reflect.String:
		return bindTypedE[T, map[string]string](jsonName, offset, r, func() Validator[map[string]string] { return mapOf(r, String()) })
	case reflect.Int:
		return bindTypedE[T, map[string]int](jsonName, offset, r, func() Validator[map[string]int] { return mapOf(r, Int()) })
	case reflect.Int64:
		return bindTypedE[T, map[string]int64](jsonName, offset, r, func() Validator[map[string]int64] { return mapOf(r, Int64()) })
	case reflect.Float64:
		return bindTypedE[T, map[string]float64](jsonName, offset, r, func() Validator[map[string]float64] { return mapOf(r, Float64()) })
	case reflect.Interface:
		if elem != anyType {
			return nil, fmt.Errorf("unsupported map value type %s", elem)
		}
		return bindTypedE[T, map[string]any](jsonName, offset, r, func() Validator[map[string]any] { return mapOf(r, Any()) })
	default:
		return nil, fmt.Errorf("unsupported map value type %s", elem)
	}
}

func mapOf[F any](r tagRules, values Validator[F]) Validator[map[string]F] {
	m := Map(values)
	if r.minStr != "" {
		f, _ := strconv.ParseFloat(r.minStr, 64)
		m.MinKeys(int(f))
	}
	if r.maxStr != "" {
		f, _ := strconv.ParseFloat(r.maxStr, 64)
		m.MaxKeys(int(f))
	}
	return m
}

// bindRegistryField binds a nested struct field whose schema comes from the
// tag registry. Values are written via reflect because the concrete field
// type is only known at runtime.
func bindRegistryField[T any](jsonName string, f reflect.StructField, r tagRules, isPointer bool) (*fieldBinding, error) {
	raw, ok := tagSchemaRegistry.Load(f.Type)
	if !ok {
		return nil, fmt.Errorf("no registered schema for struct type %s; call skema.RegisterTagSchema for it", f.Type)
	}
	v := raw.(Validator[any])
	node := v.schemaNode().cloneShallow()
	node.optional = r.optional
	node.nullable = r.nullable
	fb := &fieldBinding{
		jsonName: jsonName,
		offset:   f.Offset,
		optional: r.optional,
		nullable: r.nullable,
		node:     node,
		parse: func(ctx *parseContext, raw any, base unsafe.Pointer) {
			if raw == nil {
				return // optional/nullable nested struct: treated as absent
			}
			child := ctx.child(jsonName)
			val, err := v.parse(child, raw)
			if err != nil {
				return
			}
			if len(child.issues.Issues) > 0 {
				return
			}
			if isPointer {
				ptr := reflect.New(f.Type.Elem())
				ptr.Elem().Set(reflect.ValueOf(val))
				val = ptr.Interface()
			}
			dest := reflect.NewAt(f.Type, unsafe.Pointer(uintptr(base)+f.Offset))
			dest.Elem().Set(reflect.ValueOf(val))
		},
	}
	fb.parseRaw = func(ctx *parseContext, rawMsg json.RawMessage, base unsafe.Pointer) {
		decoded, err := anyFromJSON(rawMsg)
		if err != nil {
			child := ctx.child(jsonName)
			child.add(child.path, CodeInvalidFormat, "invalid object")
			return
		}
		fb.parse(ctx, decoded, base)
	}
	return fb, nil
}

// requiredPointer wraps an inner validator into a required (non-optional,
// non-nullable) pointer field validator.
func requiredPointer[T any](inner Validator[T]) Validator[*T] {
	node := inner.schemaNode().cloneShallow()
	node.optional = false
	node.nullable = false
	return bindValidator(node, func(ctx *parseContext, raw any) (*T, error) {
		if raw == nil {
			ctx.add(ctx.path, CodeInvalidType, "must not be null")
			return nil, nil
		}
		out, err := inner.parse(ctx, raw)
		if err != nil {
			return nil, err
		}
		if len(ctx.issues.Issues) > 0 {
			return nil, nil
		}
		return &out, nil
	})
}

// --- per-type validator builders ---

func buildStringValidator(r tagRules) (Validator[string], error) {
	if len(r.enum) > 0 || r.literal != "" {
		if hasAnyTag(r, "format", "pattern", "minStr", "maxStr", "lenStr", "prefix", "suffix", "nonEmpty", "trim", "lower", "upper") {
			return nil, fmt.Errorf("enum/literal cannot be combined with other string rules")
		}
		switch {
		case len(r.enum) > 0:
			return withDefault(Enum(r.enum...), r), nil
		default:
			return withDefault(Literal(r.literal), r), nil
		}
	}
	s := String()
	if r.trim {
		s = s.Trim()
	}
	if r.lower {
		s = s.Lower()
	}
	if r.upper {
		s = s.Upper()
	}
	if r.nonEmpty {
		s.NonEmpty()
	}
	if r.lenStr != "" {
		s.Len(atoiTag(r.lenStr))
	} else {
		if r.minStr != "" {
			s.Min(atoiTag(r.minStr))
		}
		if r.maxStr != "" {
			s.Max(atoiTag(r.maxStr))
		}
	}
	if r.pattern != "" {
		s.Regex(r.pattern)
	}
	if r.prefix != "" {
		s.Prefix(r.prefix)
	}
	if r.suffix != "" {
		s.Suffix(r.suffix)
	}
	switch r.format {
	case "":
	case "email":
		s.Email()
	case "url":
		s.URL()
	case "uuid":
		s.UUID()
	case "uuid4":
		s.UUIDv4()
	case "uuid6":
		s.UUIDv6()
	case "uuid7":
		s.UUIDv7()
	case "datetime":
		s.DateTime()
	case "date":
		s.Date()
	case "ipv4":
		s.IPv4()
	case "ipv6":
		s.IPv6()
	case "base64":
		s.Base64()
	case "cidr":
		s.CIDR()
	case "jwt":
		s.JWT()
	default:
		return nil, fmt.Errorf("format %q does not apply to strings", r.format)
	}
	return withDefault(s, r), nil
}

func buildBoolValidator(r tagRules) (Validator[bool], error) {
	b := Bool()
	if r.coerce {
		b.Coerce()
	}
	if r.defaultVal == "" {
		return b, nil
	}
	d, err := strconv.ParseBool(r.defaultVal)
	if err != nil {
		return nil, fmt.Errorf("default: %v", err)
	}
	return Default(b, d), nil
}

func buildIntValidator(r tagRules) (Validator[int], error) {
	n := Int()
	if r.coerce {
		n.Coerce()
	}
	if err := applyIntRules(r, func(v int64) { n.Min(int(v)) }, func(v int64) { n.Max(int(v)) }, func(v int64) { n.MultipleOf(int(v)) }); err != nil {
		return nil, err
	}
	applySign(r, n.Positive, n.NonNegative, n.Negative)
	return withDefaultFunc(r, func() (int, error) {
		return strconv.Atoi(r.defaultVal)
	}, n)
}

func buildInt64Validator(r tagRules) (Validator[int64], error) {
	n := Int64()
	if r.coerce {
		n.Coerce()
	}
	if err := applyIntRules(r, func(v int64) { n.Min(v) }, func(v int64) { n.Max(v) }, func(v int64) { n.MultipleOf(v) }); err != nil {
		return nil, err
	}
	return withDefaultFunc(r, func() (int64, error) {
		return strconv.ParseInt(r.defaultVal, 10, 64)
	}, n)
}

func buildUint64Validator(r tagRules) (Validator[uint64], error) {
	n := Uint64()
	if r.coerce {
		n.Coerce()
	}
	if r.minStr != "" {
		v, err := strconv.ParseUint(r.minStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("min: %v", err)
		}
		n.Min(v)
	}
	if r.maxStr != "" {
		v, err := strconv.ParseUint(r.maxStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("max: %v", err)
		}
		n.Max(v)
	}
	if r.multiple != "" {
		v, err := strconv.ParseUint(r.multiple, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("multiple: %v", err)
		}
		n.MultipleOf(v)
	}
	return withDefaultFunc(r, func() (uint64, error) {
		return strconv.ParseUint(r.defaultVal, 10, 64)
	}, n)
}

func buildWidthValidator(ft reflect.Type, r tagRules) (any, error) {
	bound := widthBounds(ft)
	switch ft.Kind() {
	case reflect.Int8:
		return buildWidthNum(r, Int8, bound)
	case reflect.Int16:
		return buildWidthNum(r, Int16, bound)
	case reflect.Int32:
		return buildWidthNum(r, Int32, bound)
	case reflect.Uint:
		return buildWidthNum(r, Uint, bound)
	case reflect.Uint8:
		return buildWidthNum(r, Uint8, bound)
	case reflect.Uint16:
		return buildWidthNum(r, Uint16, bound)
	case reflect.Uint32:
		return buildWidthNum(r, Uint32, bound)
	}
	return nil, fmt.Errorf("unsupported width type %s", ft)
}

func buildWidthNum[T numKind](r tagRules, base func() *numSchema[T], bound [2]int64) (Validator[T], error) {
	n := base()
	if r.coerce {
		n.Coerce()
	}
	parseInt := func(s string) (int64, error) {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, err
		}
		if v < bound[0] || v > bound[1] {
			return 0, fmt.Errorf("value %d out of range", v)
		}
		return v, nil
	}
	if r.minStr != "" {
		v, err := parseInt(r.minStr)
		if err != nil {
			return nil, fmt.Errorf("min: %v", err)
		}
		n.Min(T(v))
	}
	if r.maxStr != "" {
		v, err := parseInt(r.maxStr)
		if err != nil {
			return nil, fmt.Errorf("max: %v", err)
		}
		n.Max(T(v))
	}
	if r.multiple != "" {
		v, err := parseInt(r.multiple)
		if err != nil {
			return nil, fmt.Errorf("multiple: %v", err)
		}
		if v == 0 {
			return nil, fmt.Errorf("multiple must be non-zero")
		}
		n.MultipleOf(T(v))
	}
	applySign(r, n.Positive, n.NonNegative, n.Negative)
	if r.defaultVal == "" {
		return n, nil
	}
	v, err := parseInt(r.defaultVal)
	if err != nil {
		return nil, fmt.Errorf("default: %v", err)
	}
	return Default(n, T(v)), nil
}

func buildFloatValidator(ft reflect.Type, r tagRules) (any, error) {
	if ft.Kind() == reflect.Float32 {
		n := Float32()
		if r.coerce {
			n.Coerce()
		}
		if err := applyFloatRules(r, func(v float64) { n.Min(float32(v)) }, func(v float64) { n.Max(float32(v)) }, func(v float64) { n.MultipleOf(float32(v)) }); err != nil {
			return nil, err
		}
		applySign(r, n.Positive, n.NonNegative, n.Negative)
		if r.defaultVal == "" {
			return n, nil
		}
		v, err := strconv.ParseFloat(r.defaultVal, 64)
		if err != nil {
			return nil, fmt.Errorf("default: %v", err)
		}
		return Default(n, float32(v)), nil
	}
	n := Float64()
	if r.coerce {
		n.Coerce()
	}
	if err := applyFloatRules(r, func(v float64) { n.Min(v) }, func(v float64) { n.Max(v) }, func(v float64) { n.MultipleOf(v) }); err != nil {
		return nil, err
	}
	applySign(r, n.Positive, n.NonNegative, n.Negative)
	if r.defaultVal == "" {
		return n, nil
	}
	v, err := strconv.ParseFloat(r.defaultVal, 64)
	if err != nil {
		return nil, fmt.Errorf("default: %v", err)
	}
	return Default(n, v), nil
}

func buildBigIntValidator(r tagRules) (Validator[big.Int], error) {
	b := BigInt()
	if r.coerce {
		b.Coerce()
	}
	if r.defaultVal == "" {
		return b, nil
	}
	var d big.Int
	if _, ok := d.SetString(r.defaultVal, 10); !ok {
		return nil, fmt.Errorf("default: invalid integer %q", r.defaultVal)
	}
	return Default(b, d), nil
}

func applyIntRules(r tagRules, setMin, setMax, setMult func(int64)) error {
	if r.minStr != "" {
		v, err := strconv.ParseInt(r.minStr, 10, 64)
		if err != nil {
			return fmt.Errorf("min: %v", err)
		}
		setMin(v)
	}
	if r.maxStr != "" {
		v, err := strconv.ParseInt(r.maxStr, 10, 64)
		if err != nil {
			return fmt.Errorf("max: %v", err)
		}
		setMax(v)
	}
	if r.multiple != "" {
		v, err := strconv.ParseInt(r.multiple, 10, 64)
		if err != nil {
			return fmt.Errorf("multiple: %v", err)
		}
		if v == 0 {
			return fmt.Errorf("multiple must be non-zero")
		}
		setMult(v)
	}
	return nil
}

func applyFloatRules(r tagRules, setMin, setMax, setMult func(float64)) error {
	if r.minStr != "" {
		v, err := strconv.ParseFloat(r.minStr, 64)
		if err != nil {
			return fmt.Errorf("min: %v", err)
		}
		setMin(v)
	}
	if r.maxStr != "" {
		v, err := strconv.ParseFloat(r.maxStr, 64)
		if err != nil {
			return fmt.Errorf("max: %v", err)
		}
		setMax(v)
	}
	if r.multiple != "" {
		v, err := strconv.ParseFloat(r.multiple, 64)
		if err != nil {
			return fmt.Errorf("multiple: %v", err)
		}
		if v == 0 {
			return fmt.Errorf("multiple must be non-zero")
		}
		setMult(v)
	}
	return nil
}

func applySign[T any](r tagRules, positive, nonNeg, negative func() T) {
	if r.positive {
		positive()
	}
	if r.nonNeg {
		nonNeg()
	}
	if r.negative {
		negative()
	}
}

func withDefault(v Validator[string], r tagRules) Validator[string] {
	if r.defaultVal == "" {
		return v
	}
	return Default(v, r.defaultVal)
}

func withDefaultFunc[T any](r tagRules, parse func() (T, error), v Validator[T]) (Validator[T], error) {
	if r.defaultVal == "" {
		return v, nil
	}
	d, err := parse()
	if err != nil {
		return nil, fmt.Errorf("default: %v", err)
	}
	return Default(v, d), nil
}

func hasAnyTag(r tagRules, fields ...string) bool {
	for _, f := range fields {
		var set bool
		switch f {
		case "format":
			set = r.format != ""
		case "pattern":
			set = r.pattern != ""
		case "prefix":
			set = r.prefix != ""
		case "suffix":
			set = r.suffix != ""
		case "nonEmpty":
			set = r.nonEmpty
		case "trim":
			set = r.trim
		case "lower":
			set = r.lower
		case "upper":
			set = r.upper
		case "minStr":
			set = r.minStr != ""
		case "maxStr":
			set = r.maxStr != ""
		case "lenStr":
			set = r.lenStr != ""
		case "multiple":
			set = r.multiple != ""
		default:
			return true
		}
		if set {
			return true
		}
	}
	return false
}

func atoiTag(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func widthBounds(ft reflect.Type) [2]int64 {
	switch ft.Kind() {
	case reflect.Int8:
		return [2]int64{math.MinInt8, math.MaxInt8}
	case reflect.Int16:
		return [2]int64{math.MinInt16, math.MaxInt16}
	case reflect.Int32:
		return [2]int64{math.MinInt32, math.MaxInt32}
	case reflect.Uint:
		return [2]int64{0, math.MaxInt64} // ParseInt-based bounds stay in int64 range
	case reflect.Uint8:
		return [2]int64{0, math.MaxUint8}
	case reflect.Uint16:
		return [2]int64{0, math.MaxUint16}
	case reflect.Uint32:
		return [2]int64{0, math.MaxUint32}
	}
	return [2]int64{math.MinInt64, math.MaxInt64}
}
