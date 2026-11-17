package skema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// FromJSONSchemaBytes decodes and compiles a JSON Schema document into a
// skema validator over any.
func FromJSONSchemaBytes(data []byte) (Validator[any], error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("skema.FromJSONSchema: %w", err)
	}
	if err := ensureNoTrailingJSON(dec); err != nil {
		return nil, fmt.Errorf("skema.FromJSONSchema: %w", err)
	}
	return FromJSONSchema(doc)
}

// FromJSONSchema compiles a JSON Schema document into a validator over any.
// Supported: type (incl. [T, "null"]), enum, const, string/number/array/
// object keywords, oneOf/anyOf/allOf, $defs/definitions with $ref (internal
// refs only, cycle-safe), title/description/default. Unsupported keywords
// (not, if/then/else, patternProperties, external $refs, …) fail closed
// with an error so nothing silently passes.
func FromJSONSchema(doc map[string]any) (Validator[any], error) {
	imp := &schemaImporter{
		defs: map[string]map[string]any{},
		root: doc,
	}
	for _, key := range []string{"$defs", "definitions"} {
		if raw, ok := doc[key].(map[string]any); ok {
			for name, d := range raw {
				if dm, ok := d.(map[string]any); ok {
					imp.defs[name] = dm
				}
			}
		}
	}
	return imp.build(doc)
}

type schemaImporter struct {
	defs map[string]map[string]any
	root map[string]any
}

func (imp *schemaImporter) build(node map[string]any) (Validator[any], error) {
	if node == nil {
		return Any(), nil
	}
	for _, key := range []string{"not", "if", "then", "else", "patternProperties", "dependentRequired", "dependentSchemas"} {
		if _, ok := node[key]; ok {
			return nil, fmt.Errorf("skema.FromJSONSchema: unsupported keyword %q", key)
		}
	}
	if ref, ok := node["$ref"].(string); ok {
		return imp.resolveRef(ref)
	}

	var parts []Validator[any]
	base, err := imp.buildBase(node)
	if err != nil {
		return nil, err
	}
	if base != nil {
		parts = append(parts, base)
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if raw, ok := node[key].([]any); ok {
			variants, err := imp.buildAll(raw)
			if err != nil {
				return nil, err
			}
			if len(variants) > 0 {
				parts = append(parts, Union(variants...))
			}
		}
	}
	if raw, ok := node["allOf"].([]any); ok {
		sub, err := imp.buildAll(raw)
		if err != nil {
			return nil, err
		}
		parts = append(parts, sub...)
	}

	var out Validator[any]
	switch {
	case len(parts) == 0:
		out = Any()
	case len(parts) == 1:
		out = parts[0]
	default:
		out = Intersect(parts...)
	}
	if t, ok := node["title"].(string); ok {
		out = Title(out, t)
	}
	if d, ok := node["description"].(string); ok {
		out = Describe(out, d)
	}
	if def, ok := node["default"]; ok {
		out = Default[any](out, def)
	}
	return out, nil
}

func (imp *schemaImporter) buildAll(raw []any) ([]Validator[any], error) {
	out := make([]Validator[any], 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("skema.FromJSONSchema: schema entries must be objects")
		}
		v, err := imp.build(m)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (imp *schemaImporter) resolveRef(ref string) (Validator[any], error) {
	if ref == "#" {
		return Lazy[any]("#root", func() Validator[any] {
			v, err := imp.build(imp.root)
			if err != nil {
				panic(fmt.Sprintf("skema.FromJSONSchema: root schema: %v", err))
			}
			return v
		}), nil
	}
	path, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return nil, fmt.Errorf("skema.FromJSONSchema: external $ref %q not supported", ref)
	}
	segments := strings.Split(path, "/")
	var name string
	switch {
	case len(segments) >= 2 && (segments[0] == "$defs" || segments[0] == "definitions"):
		name = segments[1]
	default:
		name = segments[0]
	}
	name = strings.ReplaceAll(name, "~1", "/")
	name = strings.ReplaceAll(name, "~0", "~")
	def, ok := imp.defs[name]
	if !ok {
		return nil, fmt.Errorf("skema.FromJSONSchema: unknown $ref %q", ref)
	}
	return Lazy[any](name, func() Validator[any] {
		v, err := imp.build(def)
		if err != nil {
			panic(fmt.Sprintf("skema.FromJSONSchema: $defs/%s: %v", name, err))
		}
		return v
	}), nil
}

// buildBase builds the validator for direct keywords; nil when the node has
// none. Keywords without a "type" apply only to values of the matching kind,
// mirroring JSON Schema's conditional applicability.
func (imp *schemaImporter) buildBase(node map[string]any) (Validator[any], error) {
	if en, ok := node["enum"].([]any); ok && len(en) > 0 {
		variants := make([]Validator[any], 0, len(en))
		for _, v := range en {
			lv, err := imp.literalValidator(v)
			if err != nil {
				return nil, err
			}
			variants = append(variants, lv)
		}
		return Union(variants...), nil
	}
	if c, present := node["const"]; present {
		return imp.literalValidator(c)
	}

	typeVal, hasType := node["type"]
	if !hasType {
		return imp.buildUngated(node)
	}
	switch t := typeVal.(type) {
	case string:
		v, err := imp.buildType(t, node)
		if err != nil {
			return nil, err
		}
		return v, nil
	case []any:
		var nonNull []string
		null := false
		for _, tv := range t {
			ts, _ := tv.(string)
			if ts == "null" {
				null = true
			} else {
				nonNull = append(nonNull, ts)
			}
		}
		if len(nonNull) != 1 {
			return nil, fmt.Errorf("skema.FromJSONSchema: unsupported type array %v", t)
		}
		v, err := imp.buildType(nonNull[0], node)
		if err != nil {
			return nil, err
		}
		if null {
			v = Lift(Nullable(v))
		}
		return v, nil
	default:
		return nil, fmt.Errorf("skema.FromJSONSchema: invalid type %v", typeVal)
	}
}

// buildUngated applies keywords gated by the JSON kind they belong to.
func (imp *schemaImporter) buildUngated(node map[string]any) (Validator[any], error) {
	var parts []Validator[any]
	stringly := hasAny(node, "minLength", "maxLength", "pattern", "format")
	numeric := hasAny(node, "minimum", "maximum", "multipleOf")
	arrayish := hasAny(node, "items", "prefixItems", "minItems", "maxItems", "uniqueItems")
	objectish := hasAny(node, "properties", "required", "additionalProperties", "minProperties", "maxProperties")

	if stringly {
		s, err := imp.buildStringFrom(node)
		if err != nil {
			return nil, err
		}
		parts = append(parts, gateKind(isStringKind, Lift(s)))
	}
	if numeric {
		n, err := imp.buildNumberFrom(node)
		if err != nil {
			return nil, err
		}
		parts = append(parts, gateKind(isNumberKind, Lift(n)))
	}
	if arrayish {
		a, err := imp.buildArrayFrom(node)
		if err != nil {
			return nil, err
		}
		parts = append(parts, gateKind(isArrayKind, Lift(a)))
	}
	if objectish {
		o, err := imp.buildObjectFrom(node)
		if err != nil {
			return nil, err
		}
		parts = append(parts, gateKind(isObjectKind, Lift(o)))
	}
	switch len(parts) {
	case 0:
		return nil, nil
	case 1:
		return parts[0], nil
	default:
		return Intersect(parts...), nil
	}
}

func (imp *schemaImporter) buildType(t string, node map[string]any) (Validator[any], error) {
	switch t {
	case "string":
		s, err := imp.buildStringFrom(node)
		if err != nil {
			return nil, err
		}
		return Lift(s), nil
	case "integer":
		n, err := imp.buildIntFrom(node)
		if err != nil {
			return nil, err
		}
		return Lift(n), nil
	case "number":
		n, err := imp.buildNumberFrom(node)
		if err != nil {
			return nil, err
		}
		return Lift(n), nil
	case "boolean":
		return Lift(Bool()), nil
	case "array":
		a, err := imp.buildArrayFrom(node)
		if err != nil {
			return nil, err
		}
		return Lift(a), nil
	case "object":
		o, err := imp.buildObjectFrom(node)
		if err != nil {
			return nil, err
		}
		return Lift(o), nil
	case "null":
		return nullValidator(), nil
	default:
		return nil, fmt.Errorf("skema.FromJSONSchema: unsupported type %q", t)
	}
}

func (imp *schemaImporter) buildStringFrom(node map[string]any) (*stringSchema, error) {
	s := String()
	if f, ok := node["minLength"].(float64); ok {
		s.Min(int(f))
	}
	if f, ok := node["maxLength"].(float64); ok {
		s.Max(int(f))
	}
	if p, ok := node["pattern"].(string); ok {
		if _, err := regexp.Compile(p); err != nil {
			return nil, fmt.Errorf("skema.FromJSONSchema: invalid pattern: %w", err)
		}
		s.Regex(p)
	}
	if f, ok := node["format"].(string); ok {
		switch f {
		case "email":
			s.Email()
		case "uri", "url":
			s.URL()
		case "uuid":
			s.UUID()
		case "date-time":
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
			// Unknown formats are annotations in JSON Schema — ignore.
		}
	}
	return s, nil
}

func (imp *schemaImporter) buildIntFrom(node map[string]any) (*int64Schema, error) {
	n := Int64()
	minimum, hasMin := node["minimum"].(float64)
	if hasMin && (!isIntegral(minimum) || minimum > math.MaxInt64 || minimum < math.MinInt64) {
		return nil, fmt.Errorf("skema.FromJSONSchema: non-integral minimum for integer type")
	}
	maximum, hasMax := node["maximum"].(float64)
	if hasMax && (!isIntegral(maximum) || maximum > math.MaxInt64 || maximum < math.MinInt64) {
		return nil, fmt.Errorf("skema.FromJSONSchema: non-integral maximum for integer type")
	}
	if hasMin {
		n.Min(int64(minimum))
	}
	if hasMax {
		n.Max(int64(maximum))
	}
	if f, ok := node["multipleOf"].(float64); ok && isIntegral(f) && f != 0 {
		n.MultipleOf(int64(f))
	}
	return n, nil
}

func (imp *schemaImporter) buildNumberFrom(node map[string]any) (*float64Schema, error) {
	n := Float64()
	if f, ok := node["minimum"].(float64); ok {
		n.Min(f)
	}
	if f, ok := node["maximum"].(float64); ok {
		n.Max(f)
	}
	if f, ok := node["multipleOf"].(float64); ok && f != 0 {
		n.MultipleOf(f)
	}
	return n, nil
}

func (imp *schemaImporter) buildArrayFrom(node map[string]any) (Validator[[]any], error) {
	prefixRaw, hasPrefix := node["prefixItems"].([]any)
	itemsRaw, hasItems := node["items"]

	if hasPrefix && len(prefixRaw) > 0 {
		elems, err := imp.buildAll(prefixRaw)
		if err != nil {
			return nil, err
		}
		t := Tuple(elems...)
		if hasItems {
			rest, err := imp.itemsValidator(itemsRaw)
			if err != nil {
				return nil, err
			}
			if rest != nil {
				t.Rest(rest)
			}
		}
		return t, nil
	}
	if hasItems {
		rest, err := imp.itemsValidator(itemsRaw)
		if err != nil {
			return nil, err
		}
		if rest == nil { // items: false
			return Slice(Any()).Max(0), nil
		}
		out := Slice(rest)
		imp.applyArrayBounds(out, node)
		return out, nil
	}
	out := Slice(Any())
	imp.applyArrayBounds(out, node)
	return out, nil
}

func (imp *schemaImporter) applyArrayBounds(out *sliceSchema[any], node map[string]any) {
	if f, ok := node["minItems"].(float64); ok {
		out.Min(int(f))
	}
	if f, ok := node["maxItems"].(float64); ok {
		out.Max(int(f))
	}
	if b, ok := node["uniqueItems"].(bool); ok && b {
		out.Unique()
	}
}

// itemsValidator returns nil for items:false, Any() for items:true/absent.
func (imp *schemaImporter) itemsValidator(itemsRaw any) (Validator[any], error) {
	switch items := itemsRaw.(type) {
	case bool:
		if !items {
			return nil, nil
		}
		return Any(), nil
	case map[string]any:
		return imp.build(items)
	default:
		return Any(), nil
	}
}

func (imp *schemaImporter) buildObjectFrom(node map[string]any) (*objectSchema, error) {
	props := map[string]Validator[any]{}
	if raw, ok := node["properties"].(map[string]any); ok {
		for name, p := range raw {
			pm, ok := p.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("skema.FromJSONSchema: property %q must be a schema object", name)
			}
			v, err := imp.build(pm)
			if err != nil {
				return nil, err
			}
			props[name] = v
		}
	}
	var required []string
	if raw, ok := node["required"].([]any); ok {
		for _, r := range raw {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}
	out := Object(props, required...)
	switch ap := node["additionalProperties"].(type) {
	case bool:
		if !ap {
			out = out.Strict()
		}
	case map[string]any:
		v, err := imp.build(ap)
		if err != nil {
			return nil, err
		}
		out = out.CatchAll(v)
	}
	if f, ok := node["minProperties"].(float64); ok {
		out.MinKeys(int(f))
	}
	if f, ok := node["maxProperties"].(float64); ok {
		out.MaxKeys(int(f))
	}
	return out, nil
}

func (imp *schemaImporter) literalValidator(v any) (Validator[any], error) {
	if v == nil {
		return nullValidator(), nil
	}
	switch v.(type) {
	case string:
		return Lift(Literal(v.(string))), nil
	case float64:
		return Lift(Literal(v.(float64))), nil
	case bool:
		return Lift(Literal(v.(bool))), nil
	default:
		return nil, fmt.Errorf("skema.FromJSONSchema: unsupported const/enum value %v (%T)", v, v)
	}
}

func nullValidator() Validator[any] {
	return Custom(func(raw any, report func(path, code, msg string)) (any, error) {
		if raw != nil {
			report("", CodeInvalidType, "expected null")
			return nil, nil
		}
		return raw, nil
	})
}

func gateKind(is func(any) bool, inner Validator[any]) Validator[any] {
	node := inner.schemaNode().cloneShallow()
	return bindValidator(node, func(ctx *parseContext, raw any) (any, error) {
		if !is(raw) {
			return raw, nil
		}
		attempt := &parseContext{path: ctx.path, issues: newValidationError()}
		out, err := inner.parse(attempt, raw)
		ctx.issues.Issues = append(ctx.issues.Issues, attempt.issues.Issues...)
		if len(attempt.issues.Issues) > 0 {
			return nil, nil
		}
		return out, err
	})
}

func isStringKind(raw any) bool { _, ok := raw.(string); return ok }
func isNumberKind(raw any) bool {
	switch raw.(type) {
	case json.Number, float64, int, int64:
		return true
	}
	return false
}
func isArrayKind(raw any) bool  { _, ok := raw.([]any); return ok }
func isObjectKind(raw any) bool { _, ok := raw.(map[string]any); return ok }

func hasAny(node map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := node[k]; ok {
			return true
		}
	}
	return false
}

func isIntegral(f float64) bool { return math.Trunc(f) == f }
