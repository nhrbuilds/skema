package skema

// Custom builds a validator from a user-supplied function — the extension
// point for schema types skema does not ship. The function receives the raw
// input and a reporter; report issues with a path relative to the current
// one ("" for the value itself). Return (value, nil) with no reported issues
// for success; if fn returns an error without reporting anything, it becomes
// a single CodeCustom issue at the current path. The emitted JSON Schema is
// empty ({}) — combine with Describe/Title to document it.
func Custom[T any](fn func(raw any, report func(path, code, msg string)) (T, error)) Validator[T] {
	node := &schemaNode{}
	return bindValidator(node, func(ctx *parseContext, raw any) (T, error) {
		var zero T
		reported := 0
		out, err := fn(raw, func(path, code, msg string) {
			reported++
			ctx.add(joinPath(ctx.path, path), code, msg)
		})
		if reported > 0 {
			return zero, nil
		}
		if err != nil {
			ctx.add(ctx.path, CodeCustom, err.Error())
			return zero, nil
		}
		return out, nil
	})
}
