package skema

import "regexp"

type mapSchema[V any] struct {
	*baseValidator[map[string]V]
	node   *schemaNode
	values Validator[V]
	keys   Validator[string]
	min    *int
	max    *int
}

// Map validates JSON objects with string keys.
func Map[V any](values Validator[V]) *mapSchema[V] {
	node := &schemaNode{
		types:      []string{"object"},
		properties: map[string]*schemaNode{},
	}
	additional := true
	node.additional = &additional
	node.additionalItem = values.schemaNode()
	s := &mapSchema[V]{node: node, values: values}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *mapSchema[V]) MinKeys(n int) *mapSchema[V] {
	s.min = &n
	s.node.minProperties = &n
	return s
}
func (s *mapSchema[V]) MaxKeys(n int) *mapSchema[V] {
	s.max = &n
	s.node.maxProperties = &n
	return s
}

// Keys validates each object key with a string validator (e.g. a pattern or
// length constraint). Emitted as JSON Schema propertyNames.
func (s *mapSchema[V]) Keys(kv Validator[string]) *mapSchema[V] {
	s.keys = kv
	s.node.propertyNames = kv.schemaNode()
	return s
}

// KeyPattern validates each object key against a regular expression.
// Panics at schema build time if the pattern is invalid.
func (s *mapSchema[V]) KeyPattern(pattern string) *mapSchema[V] {
	re := regexp.MustCompile(pattern)
	node := &schemaNode{types: []string{"string"}, pattern: pattern}
	keySchema := bindValidator(node, func(ctx *parseContext, raw any) (string, error) {
		str, ok := raw.(string)
		if !ok {
			failType(ctx, "string")
			return "", nil
		}
		if !re.MatchString(str) {
			ctx.add(ctx.path, CodeInvalidFormat, "key pattern mismatch")
		}
		return str, nil
	})
	return s.Keys(keySchema)
}

func (s *mapSchema[V]) parse(ctx *parseContext, raw any) (map[string]V, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		failType(ctx, "object")
		return nil, nil
	}
	if s.min != nil && len(obj) < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum keys")
	}
	if s.max != nil && len(obj) > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum keys")
	}
	out := make(map[string]V, len(obj))
	for k, v := range obj {
		if s.keys != nil {
			attempt := &parseContext{path: joinPath(ctx.path, k), issues: newValidationError()}
			_, _ = s.keys.parse(attempt, k)
			if len(attempt.issues.Issues) > 0 {
				ctx.issues.Issues = append(ctx.issues.Issues, attempt.issues.Issues...)
				continue
			}
		}
		child := ctx.child(k)
		parsed, err := s.values.parse(child, v)
		if err != nil {
			return nil, err
		}
		if len(child.issues.Issues) > 0 {
			continue
		}
		out[k] = parsed
	}
	return out, nil
}
