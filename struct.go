package skema

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unsafe"
)

// ObjectBuilder collects struct field bindings during Struct construction.
type ObjectBuilder[T any] struct {
	Target T
	schema *structSchema[T]
}

type fieldBinding struct {
	jsonName    string
	offset      uintptr
	optional    bool
	nullable    bool
	node        *schemaNode
	parse       func(ctx *parseContext, raw any, base unsafe.Pointer)
	parseRaw    func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer)
	deepPartial func() *fieldBinding
}

type structSchema[T any] struct {
	node         *schemaNode
	fields       []fieldBinding
	strict       bool
	catchAll     Validator[any]
	knownKeys    map[string]struct{}
	refines      []refineRule[T]
	superRefines []superRefineRule[T]
}

type refineRule[T any] struct {
	fn  func(T) bool
	msg string
}

type superRefineRule[T any] struct {
	fn func(T, *IssueCollector)
}

type structValidator[T any] struct {
	*baseValidator[T]
	schema *structSchema[T]
}

// Struct builds a struct validator with compile-time field binding.
func Struct[T any](fn func(*ObjectBuilder[T])) *structValidator[T] {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt.Kind() != reflect.Struct {
		panic(fmt.Sprintf("skema.Struct: %T must be a struct", zero))
	}

	s := &structSchema[T]{
		node: &schemaNode{
			types:      []string{"object"},
			properties: map[string]*schemaNode{},
		},
	}
	b := &ObjectBuilder[T]{schema: s}
	fn(b)
	s.rebuildNode()
	return &structValidator[T]{
		baseValidator: bindValidator[T](s.node, s.parse),
		schema:        s,
	}
}

// Field binds a struct field to a validator; only matching types compile.
func Field[T any, F any](b *ObjectBuilder[T], fieldPtr *F, v Validator[F]) {
	if b == nil || b.schema == nil {
		panic("skema.Field must be called inside skema.Struct")
	}
	s := b.schema

	target := uintptr(unsafe.Pointer(&b.Target))
	fp := uintptr(unsafe.Pointer(fieldPtr))
	offset := fp - target

	jsonName := jsonNameFromOffset[T](offset)
	node := v.schemaNode()
	fb := fieldBinding{
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
	fieldParseRaw(jsonName, offset, v, &fb)
	s.fields = append(s.fields, fb)
}

func (s *structSchema[T]) rebuildNode() {
	s.node.properties = make(map[string]*schemaNode, len(s.fields))
	required := make([]string, 0, len(s.fields))
	s.knownKeys = make(map[string]struct{}, len(s.fields))
	for _, f := range s.fields {
		s.node.properties[f.jsonName] = f.node
		s.knownKeys[f.jsonName] = struct{}{}
		if !f.optional {
			required = append(required, f.jsonName)
		}
	}
	s.node.required = required
	if s.strict {
		ap := false
		s.node.additional = &ap
	} else {
		ap := true
		s.node.additional = &ap
	}
}

func (s *structSchema[T]) parse(ctx *parseContext, raw any) (T, error) {
	var out T
	if raw == nil {
		ctx.add(ctx.path, CodeRequired, "expected object")
		return out, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		failType(ctx, "object")
		return out, nil
	}
	rawObj := make(map[string]json.RawMessage, len(obj))
	for k, v := range obj {
		b, err := json.Marshal(v)
		if err != nil {
			failType(ctx, "object")
			return out, nil
		}
		rawObj[k] = b
	}
	s.parseObject(ctx, rawObj, &out)
	return out, nil
}

func (s *structSchema[T]) parseObject(ctx *parseContext, obj map[string]json.RawMessage, out *T) {
	if s.strict {
		for k := range obj {
			if _, ok := s.knownKeys[k]; !ok {
				ctx.add(joinPath(ctx.path, k), CodeUnrecognizedKey, "unrecognized key")
			}
		}
	}

	base := unsafe.Pointer(out)
	for _, f := range s.fields {
		rawMsg, present := obj[f.jsonName]
		if !present {
			if f.optional {
				continue
			}
			if f.node.defaultVal != nil {
				f.parse(ctx, f.node.defaultVal, base)
				continue
			}
			ctx.add(joinPath(ctx.path, f.jsonName), CodeRequired, "required")
			continue
		}
		if isJSONNull(rawMsg) {
			if f.nullable || f.optional {
				f.parse(ctx, nil, base)
				continue
			}
			if f.node.defaultVal != nil {
				f.parse(ctx, f.node.defaultVal, base)
				continue
			}
			ctx.add(joinPath(ctx.path, f.jsonName), CodeInvalidType, "must not be null")
			continue
		}
		f.parseRaw(ctx, rawMsg, base)
	}

	s.validateCatchAll(ctx, obj)

	if len(ctx.issues.Issues) > 0 {
		return
	}
	for _, r := range s.refines {
		if !r.fn(*out) {
			ctx.add(ctx.path, CodeCustom, r.msg)
		}
	}
	for _, r := range s.superRefines {
		r.fn(*out, &IssueCollector{path: ctx.path, ve: ctx.issues})
	}
}

// validateCatchAll checks unknown keys' values against the catchAll schema.
// Values are validated but never stored; disabled in strict mode, which
// rejects unknown keys outright.
func (s *structSchema[T]) validateCatchAll(ctx *parseContext, obj map[string]json.RawMessage) {
	if s.catchAll == nil || s.strict {
		return
	}
	for k, rawMsg := range obj {
		if _, known := s.knownKeys[k]; known {
			continue
		}
		child := ctx.child(k)
		val, err := anyFromJSON(rawMsg)
		if err != nil {
			child.add(child.path, CodeInvalidFormat, "invalid json")
			continue
		}
		_, _ = s.catchAll.parse(child, val)
	}
}

// ParseBytes uses a single-pass JSON object decode (faster than map[string]any trees).
func (v *structValidator[T]) ParseBytes(data []byte) (T, error) {
	var zero T
	obj, err := decodeJSONObjectRaw(data)
	if err != nil {
		return zero, err
	}
	ctx := newParseContext("")
	var out T
	v.schema.parseObject(ctx, obj, &out)
	return finishParse(ctx, out)
}

// ParseString parses JSON text into T.
func (v *structValidator[T]) ParseString(s string) (T, error) {
	return v.ParseBytes([]byte(s))
}

// ParseReader reads and parses JSON from r.
func (v *structValidator[T]) ParseReader(r io.Reader) (T, error) {
	var zero T
	data, err := io.ReadAll(r)
	if err != nil {
		return zero, err
	}
	return v.ParseBytes(data)
}

// Strict rejects unknown object keys with a CodeUnrecognizedKey issue.
func (v *structValidator[T]) Strict() *structValidator[T] {
	return v.derive(true)
}

// StripUnknown restores lenient parsing: unknown keys are ignored and never
// copied into the result (the inverse of Strict).
func (v *structValidator[T]) StripUnknown() *structValidator[T] {
	return v.derive(false)
}

// derive returns a validator over a fresh schemaNode so derived schemas never
// mutate the node of the validator they were built from.
func (v *structValidator[T]) derive(strict bool) *structValidator[T] {
	cp := *v.schema
	cp.node = cp.node.cloneShallow()
	cp.strict = strict
	cp.rebuildNode()
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        &cp,
	}
}

// Partial makes every field optional (e.g. PATCH).
func (v *structValidator[T]) Partial() *structValidator[T] {
	cp := *v.schema
	cp.node = cp.node.cloneShallow()
	cp.fields = make([]fieldBinding, len(v.schema.fields))
	copy(cp.fields, v.schema.fields)
	for i := range cp.fields {
		cp.fields[i].optional = true
		n := cp.fields[i].node.cloneShallow()
		n.optional = true
		cp.fields[i].node = n
	}
	cp.rebuildNode()
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        &cp,
	}
}

// Refine adds a cross-field predicate.
func (v *structValidator[T]) Refine(fn func(T) bool, message string) *structValidator[T] {
	cp := *v.schema
	cp.refines = append(append([]refineRule[T]{}, v.schema.refines...), refineRule[T]{fn: fn, msg: message})
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        &cp,
	}
}

// SuperRefine adds a cross-field check that may report multiple issues at
// arbitrary paths via the IssueCollector.
func (v *structValidator[T]) SuperRefine(fn func(T, *IssueCollector)) *structValidator[T] {
	cp := *v.schema
	cp.superRefines = append(append([]superRefineRule[T]{}, v.schema.superRefines...), superRefineRule[T]{fn: fn})
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        &cp,
	}
}

// CatchAll validates unknown keys' values against cv. Values are validated
// but never stored (a Go struct has nowhere to put them). No effect in
// strict mode, which rejects unknown keys. Emitted as additionalProperties.
func (v *structValidator[T]) CatchAll(cv Validator[any]) *structValidator[T] {
	cp := *v.schema
	cp.node = cp.node.cloneShallow()
	cp.catchAll = cv
	cp.node.additionalItem = cv.schemaNode()
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        &cp,
	}
}

// Pick returns a schema keeping only the named JSON keys. Cross-field
// refinements are dropped since they may reference omitted fields.
func (v *structValidator[T]) Pick(keys []string) *structValidator[T] {
	return v.selectFields(keys, nil)
}

// Omit returns a schema without the named JSON keys.
func (v *structValidator[T]) Omit(keys []string) *structValidator[T] {
	return v.selectFields(nil, keys)
}

func (v *structValidator[T]) selectFields(pick, omit []string) *structValidator[T] {
	pickSet := make(map[string]struct{}, len(pick))
	for _, k := range pick {
		pickSet[k] = struct{}{}
	}
	omitSet := make(map[string]struct{}, len(omit))
	for _, k := range omit {
		omitSet[k] = struct{}{}
	}
	cp := *v.schema
	cp.node = cp.node.cloneShallow()
	cp.fields = make([]fieldBinding, 0, len(v.schema.fields))
	for _, f := range v.schema.fields {
		if _, drop := omitSet[f.jsonName]; drop {
			continue
		}
		if len(pickSet) > 0 {
			if _, ok := pickSet[f.jsonName]; !ok {
				continue
			}
		}
		cp.fields = append(cp.fields, f)
	}
	cp.refines = nil
	cp.superRefines = nil
	cp.rebuildNode()
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        &cp,
	}
}

// DeepPartial makes every field optional, recursively through directly-bound
// nested struct schemas (wrappers such as Optional around nested structs only
// get their node marked). Cross-field refinements are dropped.
func (v *structValidator[T]) DeepPartial() *structValidator[T] {
	cp := v.schema.deepPartialSchema()
	return &structValidator[T]{
		baseValidator: bindValidator[T](cp.node, cp.parse),
		schema:        cp,
	}
}

func (s *structSchema[T]) deepPartialSchema() *structSchema[T] {
	cp := *s
	cp.node = s.node.cloneShallow()
	cp.fields = make([]fieldBinding, len(s.fields))
	copy(cp.fields, s.fields)
	for i := range cp.fields {
		f := &cp.fields[i]
		f.optional = true
		if f.deepPartial != nil {
			*f = *f.deepPartial()
		} else {
			n := f.node.cloneShallow()
			n.optional = true
			f.node = n
		}
	}
	cp.refines = nil
	cp.superRefines = nil
	cp.rebuildNode()
	return &cp
}

// KeyOf returns a validator that parses one of v's JSON keys (emits an enum).
func KeyOf[T any](v *structValidator[T]) Validator[string] {
	names := make([]string, 0, len(v.schema.knownKeys))
	for name := range v.schema.knownKeys {
		names = append(names, name)
	}
	slices.Sort(names)
	return Enum(names...)
}

func jsonNameFromOffset[T any](offset uintptr) string {
	var zero T
	rt := reflect.TypeOf(zero)
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Offset == offset {
			name := jsonTagName(f)
			if name == "" {
				panic("skema.Field: field " + f.Name + " is skipped by json:\"-\"")
			}
			return name
		}
	}
	panic(fmt.Sprintf("skema.Field: unknown field offset %d on %T", offset, zero))
}

func jsonTagName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag == "" {
		return toSnakeCase(f.Name)
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return toSnakeCase(f.Name)
	}
	return name
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}
