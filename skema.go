package skema

import (
	"encoding/json"
	"io"
)

// Validator is a type-safe schema that parses and emits JSON Schema.
type Validator[T any] interface {
	ParseBytes(data []byte) (T, error)
	ParseString(s string) (T, error)
	ParseReader(r io.Reader) (T, error)
	ParseAny(v any) (T, error)
	JSONSchema(opts ...SchemaOption) ([]byte, error)
	JSONSchemaMap(opts ...SchemaOption) (map[string]any, error)

	parse(ctx *parseContext, raw any) (T, error)
	schemaNode() *schemaNode
}

// SchemaOption configures JSON Schema emission.
type SchemaOption func(*schemaCompiler)

type schemaNode struct {
	typeName       string
	description    string
	title          string
	types          []string
	enum           []any
	constVal       any
	defaultVal     any
	nullable       bool
	optional       bool
	minLength      *int
	maxLength      *int
	lenExact       *int
	pattern        string
	format         string
	minimum        *float64
	maximum        *float64
	exclusiveMin   bool
	exclusiveMax   bool
	multipleOf     *float64
	minItems       *int
	maxItems       *int
	uniqueItems    bool
	minProperties  *int
	maxProperties  *int
	properties     map[string]*schemaNode
	required       []string
	additional     *bool
	items          *schemaNode
	prefixItems    []*schemaNode
	additionalItem *schemaNode
	propertyNames  *schemaNode
	oneOf          []*schemaNode
	anyOf          []*schemaNode
	allOf          []*schemaNode
	ref            string
	defName        string
	resolveDef     func() *schemaNode
	discriminator  map[string]string
}

func (n *schemaNode) cloneShallow() *schemaNode {
	if n == nil {
		return nil
	}
	c := *n
	if n.properties != nil {
		c.properties = make(map[string]*schemaNode, len(n.properties))
		for k, v := range n.properties {
			c.properties[k] = v
		}
	}
	return &c
}

// baseValidator implements common parse entrypoints (allocate once per schema).
type baseValidator[T any] struct {
	inner func(ctx *parseContext, raw any) (T, error)
	node  *schemaNode
}

func bindValidator[T any](node *schemaNode, parse func(ctx *parseContext, raw any) (T, error)) *baseValidator[T] {
	return &baseValidator[T]{node: node, inner: parse}
}

func (b *baseValidator[T]) ParseBytes(data []byte) (T, error) {
	var zero T
	raw, err := decodeJSONValue(data)
	if err != nil {
		return zero, err
	}
	return b.ParseAny(raw)
}

func (b *baseValidator[T]) ParseString(s string) (T, error) {
	return b.ParseBytes([]byte(s))
}

func (b *baseValidator[T]) ParseReader(r io.Reader) (T, error) {
	var zero T
	raw, err := decodeJSONReader(r)
	if err != nil {
		return zero, err
	}
	return b.ParseAny(raw)
}

func (b *baseValidator[T]) ParseAny(v any) (T, error) {
	var zero T
	ctx := newParseContext("")
	out, err := b.parse(ctx, v)
	if err != nil {
		releaseParseContext(ctx)
		return zero, err
	}
	if len(ctx.issues.Issues) > 0 {
		ve := &ValidationError{Issues: append([]Issue(nil), ctx.issues.Issues...)}
		releaseParseContext(ctx)
		return zero, ve
	}
	releaseParseContext(ctx)
	return out, nil
}

func (b *baseValidator[T]) parse(ctx *parseContext, raw any) (T, error) {
	return b.inner(ctx, raw)
}

func (b *baseValidator[T]) schemaNode() *schemaNode {
	return b.node
}

func (b *baseValidator[T]) JSONSchema(opts ...SchemaOption) ([]byte, error) {
	m, err := b.JSONSchemaMap(opts...)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

func (b *baseValidator[T]) JSONSchemaMap(opts ...SchemaOption) (map[string]any, error) {
	comp := newSchemaCompiler(opts...)
	root := b.node
	if root != nil && root.defName != "" && root.resolveDef != nil {
		comp.registerDef(root.defName, root.resolveDef())
	}
	return comp.compile(root), nil
}

func wrapValidator[T any](inner func(ctx *parseContext, raw any) (T, error), node *schemaNode) Validator[T] {
	return bindValidator(node, inner)
}

func isMissing(raw any) bool {
	return raw == nil
}

func failType(ctx *parseContext, expected string) {
	ctx.add(ctx.path, CodeInvalidType, "expected "+expected)
}
