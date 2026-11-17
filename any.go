package skema

// Any accepts any JSON value (including null) and validates nothing. It
// emits an empty JSON Schema ({}, i.e. anything).
func Any() Validator[any] {
	node := &schemaNode{}
	return bindValidator(node, func(ctx *parseContext, raw any) (any, error) {
		return raw, nil
	})
}

// Lift adapts a typed validator to Validator[any] so heterogeneous
// containers such as Tuple, Union[any] and CatchAll can hold validators of
// different concrete types.
func Lift[T any](v Validator[T]) Validator[any] {
	node := v.schemaNode().cloneShallow()
	return bindValidator(node, func(ctx *parseContext, raw any) (any, error) {
		return v.parse(ctx, raw)
	})
}
