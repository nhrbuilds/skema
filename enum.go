package skema

type enumSchema[T ~string] struct {
	*baseValidator[T]
	node   *schemaNode
	values []T
	set    map[T]struct{}
}

// Enum validates string membership.
func Enum[T ~string](vals ...T) *enumSchema[T] {
	set := make(map[T]struct{}, len(vals))
	anyVals := make([]any, len(vals))
	for i, v := range vals {
		set[v] = struct{}{}
		anyVals[i] = string(v)
	}
	node := &schemaNode{types: []string{"string"}, enum: anyVals}
	s := &enumSchema[T]{node: node, values: vals, set: set}
	s.baseValidator = bindValidator(node, s.parse)
	return s
}

func (s *enumSchema[T]) Default(val T) Validator[T] { return Default(s, val) }

func (s *enumSchema[T]) parse(ctx *parseContext, raw any) (T, error) {
	var zero T
	str, ok := raw.(string)
	if !ok {
		failType(ctx, "string")
		return zero, nil
	}
	v := T(str)
	if _, ok := s.set[v]; !ok {
		ctx.add(ctx.path, CodeInvalidEnum, "invalid enum value")
	}
	return v, nil
}

// Literal validates an exact constant.
func Literal[T comparable](val T) Validator[T] {
	node := &schemaNode{constVal: val}
	switch any(val).(type) {
	case string:
		node.types = []string{"string"}
	case bool:
		node.types = []string{"boolean"}
	case int, int64, float64:
		node.types = []string{"number"}
	default:
		node.types = []string{"string"}
	}
	return bindValidator(node, func(ctx *parseContext, raw any) (T, error) {
		var zero T
		if !literalsEqual(val, raw) {
			ctx.add(ctx.path, CodeInvalidFormat, "expected literal value")
			return zero, nil
		}
		return val, nil
	})
}
