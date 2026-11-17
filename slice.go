package skema

type sliceSchema[T any] struct {
	*baseValidator[[]T]
	node     *schemaNode
	elem     Validator[T]
	min      *int
	max      *int
	nonEmpty bool
	unique   bool
}

// Slice validates JSON arrays.
func Slice[T any](elem Validator[T]) *sliceSchema[T] {
	s := &sliceSchema[T]{
		node: &schemaNode{types: []string{"array"}, items: elem.schemaNode()},
		elem: elem,
	}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *sliceSchema[T]) Min(n int) *sliceSchema[T] {
	s.min = &n
	s.node.minItems = &n
	return s
}
func (s *sliceSchema[T]) Max(n int) *sliceSchema[T] {
	s.max = &n
	s.node.maxItems = &n
	return s
}
func (s *sliceSchema[T]) NonEmpty() *sliceSchema[T] {
	s.nonEmpty = true
	n := 1
	s.node.minItems = &n
	return s
}
func (s *sliceSchema[T]) Unique() *sliceSchema[T] {
	s.unique = true
	s.node.uniqueItems = true
	return s
}
func (s *sliceSchema[T]) Optional() Validator[[]T] {
	node := s.node.cloneShallow()
	node.optional = true
	return bindValidator(node, s.parse)
}
func (s *sliceSchema[T]) Describe(text string) Validator[[]T] { return Describe(s, text) }

func (s *sliceSchema[T]) parse(ctx *parseContext, raw any) ([]T, error) {
	arr, ok := raw.([]any)
	if !ok {
		failType(ctx, "array")
		return nil, nil
	}
	if s.min != nil && len(arr) < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum items")
	}
	if s.max != nil && len(arr) > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum items")
	}
	if s.nonEmpty && len(arr) == 0 {
		ctx.add(ctx.path, CodeTooSmall, "must not be empty")
	}
	out := make([]T, len(arr))
	seen := make(map[string]struct{})
	for i, item := range arr {
		child := ctx.child("[" + itoa(i) + "]")
		v, err := s.elem.parse(child, item)
		if err != nil {
			return nil, err
		}
		if len(child.issues.Issues) > 0 {
			continue
		}
		out[i] = v
		if s.unique {
			key := uniqueKeyJSON(v)
			if _, dup := seen[key]; dup {
				ctx.add(child.path, CodeInvalidFormat, "items must be unique")
			}
			seen[key] = struct{}{}
		}
	}
	return out, nil
}
