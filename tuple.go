package skema

// Tuple validates a JSON array positionally. Element validators are
// Validator[any] — wrap typed validators with Lift. Without Rest, the array
// length must match the number of elements exactly; with Rest, additional
// trailing items are validated against the rest schema.
func Tuple(elems ...Validator[any]) *tupleSchema {
	nodes := make([]*schemaNode, len(elems))
	for i, e := range elems {
		nodes[i] = e.schemaNode()
	}
	node := &schemaNode{types: []string{"array"}, prefixItems: nodes}
	s := &tupleSchema{node: node, elems: elems}
	s.baseValidator = bindValidator(node, s.parse)
	return s
}

type tupleSchema struct {
	*baseValidator[[]any]
	node  *schemaNode
	elems []Validator[any]
	rest  Validator[any]
}

// Rest validates trailing items beyond the fixed prefix.
func (s *tupleSchema) Rest(v Validator[any]) *tupleSchema {
	s.rest = v
	s.node.items = v.schemaNode()
	return s
}

func (s *tupleSchema) parse(ctx *parseContext, raw any) ([]any, error) {
	arr, ok := raw.([]any)
	if !ok {
		failType(ctx, "array")
		return nil, nil
	}
	if len(arr) < len(s.elems) {
		ctx.add(ctx.path, CodeTooSmall, "expected at least "+itoa(len(s.elems))+" items")
	}
	if s.rest == nil && len(arr) > len(s.elems) {
		ctx.add(ctx.path, CodeTooBig, "expected exactly "+itoa(len(s.elems))+" items")
	}
	out := make([]any, len(arr))
	for i, item := range arr {
		var ev Validator[any]
		if i < len(s.elems) {
			ev = s.elems[i]
		} else if s.rest != nil {
			ev = s.rest
		} else {
			continue // over-long array already reported
		}
		child := ctx.child("[" + itoa(i) + "]")
		v, err := ev.parse(child, item)
		if err != nil {
			return nil, err
		}
		if len(child.issues.Issues) > 0 {
			continue
		}
		out[i] = v
	}
	return out, nil
}
