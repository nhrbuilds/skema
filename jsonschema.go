package skema

import (
	"strconv"
	"strings"
)

const draft202012 = "https://json-schema.org/draft/2020-12/schema"

type schemaCompiler struct {
	draft202012 bool
	llmStrict   bool
	title       string
	description string
	defs        map[string]map[string]any
	pending     []defEntry
}

type defEntry struct {
	name string
	node *schemaNode
}

func newSchemaCompiler(opts ...SchemaOption) *schemaCompiler {
	c := &schemaCompiler{
		draft202012: true,
		defs:        map[string]map[string]any{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *schemaCompiler) registerDef(name string, node *schemaNode) {
	if name == "" || node == nil {
		return
	}
	for _, d := range c.pending {
		if d.name == name {
			return
		}
	}
	c.pending = append(c.pending, defEntry{name: name, node: node})
}

func (c *schemaCompiler) materializeDefs() {
	// Worklist loop: a definition's subtree may reference further Lazy types
	// that were registered during compilation and still need materializing.
	for len(c.pending) > 0 {
		batch := c.pending
		c.pending = nil
		for _, d := range batch {
			if _, ok := c.defs[d.name]; ok {
				continue
			}
			c.defs[d.name] = c.compileNode(d.node)
		}
	}
}

// WithDraft202012 emits Draft 2020-12 JSON Schema (default).
func WithDraft202012() SchemaOption {
	return func(c *schemaCompiler) { c.draft202012 = true }
}

// WithLLMStrictMode adapts schemas for LLM structured output APIs.
func WithLLMStrictMode() SchemaOption {
	return func(c *schemaCompiler) { c.llmStrict = true }
}

// WithTitle sets the root schema title.
func WithTitle(title string) SchemaOption {
	return func(c *schemaCompiler) { c.title = title }
}

// WithDescription sets the root schema description.
func WithDescription(desc string) SchemaOption {
	return func(c *schemaCompiler) { c.description = desc }
}

func (c *schemaCompiler) compile(node *schemaNode) map[string]any {
	// Traverse first: compileNode registers every Lazy definition it reaches.
	out := c.compileNode(node)
	// Then drain the worklist — compiling one definition can register more.
	c.materializeDefs()
	if c.draft202012 {
		out["$schema"] = draft202012
	}
	if c.title != "" {
		out["title"] = c.title
	}
	if c.description != "" {
		out["description"] = c.description
	}
	if len(c.defs) > 0 {
		out["$defs"] = c.defs
	}
	return out
}

func (c *schemaCompiler) compileNode(node *schemaNode) map[string]any {
	if node == nil {
		return map[string]any{}
	}
	if node.defName != "" && node.resolveDef != nil {
		c.registerDef(node.defName, node.resolveDef())
	}
	if node.ref != "" {
		return map[string]any{"$ref": node.ref}
	}
	m := map[string]any{}
	if node.title != "" {
		m["title"] = node.title
	}
	desc := node.description
	if c.llmStrict {
		desc = c.appendLLMHints(node, desc)
	}
	if desc != "" {
		m["description"] = desc
	}
	if node.constVal != nil {
		m["const"] = node.constVal
	}
	if len(node.enum) > 0 {
		m["enum"] = node.enum
	}
	if node.defaultVal != nil {
		m["default"] = node.defaultVal
	}
	types := node.types
	if node.nullable && len(types) > 0 && !sliceContains(types, "null") {
		types = append(append([]string{}, types...), "null")
	}
	if c.llmStrict && node.optional && len(types) > 0 && !sliceContains(types, "null") {
		types = append(append([]string{}, types...), "null")
	}
	if len(types) == 1 {
		m["type"] = types[0]
	} else if len(types) > 1 {
		m["type"] = types
	}
	if !c.llmStrict || node.format == "email" || node.format == "date" || node.format == "date-time" {
		if node.format != "" {
			m["format"] = node.format
		}
	}
	if !c.llmStrict {
		emitNumericAndLengthKeywords(node, m)
	}
	if len(node.properties) > 0 {
		props := map[string]any{}
		req := node.required
		for name, child := range node.properties {
			props[name] = c.compileNode(child)
		}
		m["properties"] = props
		if c.llmStrict {
			all := make([]string, 0, len(node.properties))
			for name := range node.properties {
				all = append(all, name)
			}
			m["required"] = all
			ap := false
			m["additionalProperties"] = ap
		} else {
			if len(req) > 0 {
				m["required"] = req
			}
			if node.additional != nil {
				m["additionalProperties"] = *node.additional
			}
		}
	} else if c.llmStrict && len(node.types) > 0 && node.types[0] == "object" {
		ap := false
		m["additionalProperties"] = ap
	}
	if node.items != nil {
		m["items"] = c.compileNode(node.items)
	}
	if len(node.prefixItems) > 0 {
		pi := make([]any, len(node.prefixItems))
		for i, child := range node.prefixItems {
			pi[i] = c.compileNode(child)
		}
		m["prefixItems"] = pi
		if node.items == nil {
			m["items"] = false
		}
	}
	if node.propertyNames != nil {
		m["propertyNames"] = c.compileNode(node.propertyNames)
	}
	if node.additionalItem != nil {
		m["additionalProperties"] = c.compileNode(node.additionalItem)
	}
	if len(node.oneOf) > 0 {
		one := make([]any, len(node.oneOf))
		for i, child := range node.oneOf {
			one[i] = c.compileNode(child)
		}
		m["oneOf"] = one
	}
	if len(node.anyOf) > 0 {
		variants := make([]any, len(node.anyOf))
		for i, child := range node.anyOf {
			variants[i] = c.compileNode(child)
		}
		m["anyOf"] = variants
	}
	if len(node.allOf) > 0 {
		inter := make([]any, len(node.allOf))
		for i, child := range node.allOf {
			inter[i] = c.compileNode(child)
		}
		m["allOf"] = inter
	}
	if node.discriminator != nil {
		if prop, ok := node.discriminator["propertyName"]; ok && prop != "" {
			m["discriminator"] = map[string]any{"propertyName": prop}
		}
	}
	return m
}

func emitNumericAndLengthKeywords(node *schemaNode, m map[string]any) {
	if node.pattern != "" {
		m["pattern"] = node.pattern
	}
	if node.minLength != nil {
		m["minLength"] = *node.minLength
	}
	if node.maxLength != nil {
		m["maxLength"] = *node.maxLength
	}
	if node.minimum != nil {
		m["minimum"] = *node.minimum
	}
	if node.maximum != nil {
		m["maximum"] = *node.maximum
	}
	if node.multipleOf != nil {
		m["multipleOf"] = *node.multipleOf
	}
	if node.minItems != nil {
		m["minItems"] = *node.minItems
	}
	if node.maxItems != nil {
		m["maxItems"] = *node.maxItems
	}
	if node.uniqueItems {
		m["uniqueItems"] = true
	}
	if node.minProperties != nil {
		m["minProperties"] = *node.minProperties
	}
	if node.maxProperties != nil {
		m["maxProperties"] = *node.maxProperties
	}
}

func sliceContains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}

func (c *schemaCompiler) appendLLMHints(node *schemaNode, desc string) string {
	var hints []string
	if node.minimum != nil || node.maximum != nil {
		hint := "("
		if node.minimum != nil {
			hint += "min: " + floatString(*node.minimum)
		}
		if node.maximum != nil {
			if node.minimum != nil {
				hint += ", "
			}
			hint += "max: " + floatString(*node.maximum)
		}
		hint += ")"
		hints = append(hints, hint)
	}
	if node.minLength != nil || node.maxLength != nil {
		hint := "("
		if node.minLength != nil {
			hint += "minLen: " + itoa(*node.minLength)
		}
		if node.maxLength != nil {
			if node.minLength != nil {
				hint += ", "
			}
			hint += "maxLen: " + itoa(*node.maxLength)
		}
		hint += ")"
		hints = append(hints, hint)
	}
	if len(hints) == 0 {
		return desc
	}
	joined := strings.Join(hints, " ")
	if desc == "" {
		return joined
	}
	return desc + " " + joined
}

func floatString(f float64) string {
	s := strings.TrimRight(strings.TrimRight(fmtFloat(f), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
