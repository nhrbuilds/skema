package skema

// UnionVariant maps a discriminator value to a variant schema.
type UnionVariant[T any] struct {
	Value  string
	Schema Validator[T]
}

// Union tries each variant in order and returns the first successful parse.
// On failure it reports every variant's issues at the union's path. Wrap
// differently-typed validators with Lift to build Union[any].
func Union[T any](variants ...Validator[T]) Validator[T] {
	if len(variants) == 0 {
		panic("skema.Union: at least one variant is required")
	}
	nodes := make([]*schemaNode, len(variants))
	for i, v := range variants {
		nodes[i] = v.schemaNode()
	}
	node := &schemaNode{anyOf: nodes}
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		var zero T
		var failed []Issue
		for _, v := range variants {
			attempt := &parseContext{path: ctx.path, issues: newValidationError()}
			out, err := v.parse(attempt, raw)
			if err == nil && len(attempt.issues.Issues) == 0 {
				return out, nil
			}
			failed = append(failed, attempt.issues.Issues...)
		}
		// Report variant issues only when the union as a whole fails; on
		// success the shared issue list must stay untouched.
		if len(failed) == 0 {
			ctx.add(ctx.path, CodeInvalidUnion, "no union variant matched")
		} else {
			ctx.issues.Issues = append(ctx.issues.Issues, failed...)
		}
		return zero, nil
	}, node)
}

type discriminatedUnion[T any] struct {
	*baseValidator[T]
	key      string
	variants map[string]Validator[T]
}

// DiscriminatedUnion routes objects by a discriminator field.
func DiscriminatedUnion[T any](discriminatorKey string, variants ...UnionVariant[T]) Validator[T] {
	m := make(map[string]Validator[T], len(variants))
	nodes := make([]*schemaNode, 0, len(variants))
	for _, v := range variants {
		m[v.Value] = v.Schema
		nodes = append(nodes, v.Schema.schemaNode())
	}
	node := &schemaNode{
		types: []string{"object"},
		oneOf: nodes,
		discriminator: map[string]string{
			"propertyName": discriminatorKey,
		},
	}
	u := &discriminatedUnion[T]{key: discriminatorKey, variants: m}
	u.baseValidator = bindValidator(node, u.parse)
	return u
}

func (u *discriminatedUnion[T]) parse(ctx *parseContext, raw any) (T, error) {
	var zero T
	obj, ok := raw.(map[string]any)
	if !ok {
		failType(ctx, "object")
		return zero, nil
	}
	tag, ok := obj[u.key]
	if !ok {
		ctx.add(joinPath(ctx.path, u.key), CodeRequired, "discriminator required")
		return zero, nil
	}
	tagStr, ok := tag.(string)
	if !ok {
		ctx.add(joinPath(ctx.path, u.key), CodeInvalidType, "discriminator must be string")
		return zero, nil
	}
	schema, ok := u.variants[tagStr]
	if !ok {
		ctx.add(joinPath(ctx.path, u.key), CodeInvalidEnum, "unknown discriminator")
		return zero, nil
	}
	return schema.parse(ctx, raw)
}
