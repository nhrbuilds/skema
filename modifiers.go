package skema

// Optional allows a missing key or omitted value; JSON null is rejected and
// present invalid values still error. Use Nullable or OptionalNull to accept null.
func Optional[T any](v Validator[T]) Validator[*T] {
	base := v.schemaNode().cloneShallow()
	base.optional = true
	return wrapValidator[*T](func(ctx *parseContext, raw any) (*T, error) {
		if raw == nil {
			ctx.add(ctx.path, CodeInvalidType, "unexpected null")
			return nil, nil
		}
		out, err := v.parse(ctx, raw)
		if err != nil {
			return nil, err
		}
		if len(ctx.issues.Issues) > 0 {
			return nil, nil
		}
		return &out, nil
	}, base)
}

// OptionalValue keeps Validator[T] while marking the field optional in objects.
func OptionalValue[T any](v Validator[T]) Validator[T] {
	base := v.schemaNode().cloneShallow()
	base.optional = true
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		if isMissing(raw) {
			var zero T
			return zero, nil
		}
		return v.parse(ctx, raw)
	}, base)
}

// OptionalNull allows missing keys and JSON null for pointer fields.
func OptionalNull[T any](v Validator[T]) Validator[*T] {
	base := v.schemaNode().cloneShallow()
	base.optional = true
	base.nullable = true
	return wrapValidator[*T](func(ctx *parseContext, raw any) (*T, error) {
		if isMissing(raw) || raw == nil {
			return nil, nil
		}
		out, err := v.parse(ctx, raw)
		if err != nil {
			return nil, err
		}
		if len(ctx.issues.Issues) > 0 {
			return nil, nil
		}
		return &out, nil
	}, base)
}

// Nullable allows JSON null.
func Nullable[T any](v Validator[T]) Validator[*T] {
	base := v.schemaNode().cloneShallow()
	base.nullable = true
	return wrapValidator[*T](func(ctx *parseContext, raw any) (*T, error) {
		if raw == nil {
			return nil, nil
		}
		out, err := v.parse(ctx, raw)
		if err != nil {
			return nil, err
		}
		if len(ctx.issues.Issues) > 0 {
			return nil, nil
		}
		return &out, nil
	}, base)
}

// Default supplies a value when input is missing or null.
func Default[T any](v Validator[T], val T) Validator[T] {
	base := v.schemaNode().cloneShallow()
	base.defaultVal = val
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		if isMissing(raw) || raw == nil {
			return val, nil
		}
		return v.parse(ctx, raw)
	}, base)
}

// Describe attaches documentation to the JSON Schema.
func Describe[T any](v Validator[T], text string) Validator[T] {
	base := v.schemaNode().cloneShallow()
	if base.description != "" {
		base.description = base.description + " " + text
	} else {
		base.description = text
	}
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		return v.parse(ctx, raw)
	}, base)
}

// Refine runs a predicate after base validation succeeds.
func Refine[T any](v Validator[T], fn func(T) bool, message string) Validator[T] {
	base := v.schemaNode().cloneShallow()
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		out, err := v.parse(ctx, raw)
		if err != nil {
			return out, err
		}
		if len(ctx.issues.Issues) > 0 {
			return out, nil
		}
		if !fn(out) {
			ctx.add(ctx.path, CodeCustom, message)
		}
		return out, nil
	}, base)
}

// Transform parses with inner then maps to another type.
func Transform[A, B any](inner Validator[A], fn func(A) (B, error)) Validator[B] {
	innerNode := inner.schemaNode()
	node := innerNode.cloneShallow()
	if node == nil {
		node = &schemaNode{}
	}
	return wrapValidator[B](func(ctx *parseContext, raw any) (B, error) {
		var zero B
		a, err := inner.parse(ctx, raw)
		if err != nil {
			return zero, err
		}
		if len(ctx.issues.Issues) > 0 {
			return zero, nil
		}
		b, err := fn(a)
		if err != nil {
			ctx.add(ctx.path, CodeCustom, err.Error())
			return zero, nil
		}
		return b, nil
	}, node)
}

// Catch replaces a failed parse with a fallback value instead of an error.
func Catch[T any](v Validator[T], fallback T) Validator[T] {
	base := v.schemaNode().cloneShallow()
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		attempt := &parseContext{path: ctx.path, issues: newValidationError()}
		out, err := v.parse(attempt, raw)
		if err != nil || len(attempt.issues.Issues) > 0 {
			return fallback, nil
		}
		return out, nil
	}, base)
}

// Preprocess applies fn to the raw input before validation.
func Preprocess[T any](fn func(any) any, v Validator[T]) Validator[T] {
	base := v.schemaNode().cloneShallow()
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		return v.parse(ctx, fn(raw))
	}, base)
}

// Pipe parses with a, then parses a's output with b. The emitted JSON Schema
// describes b (the output side).
func Pipe[A, B any](a Validator[A], b Validator[B]) Validator[B] {
	node := b.schemaNode().cloneShallow()
	return wrapValidator[B](func(ctx *parseContext, raw any) (B, error) {
		var zero B
		inter, err := a.parse(ctx, raw)
		if err != nil {
			return zero, err
		}
		if len(ctx.issues.Issues) > 0 {
			return zero, nil
		}
		return b.parse(ctx, inter)
	}, node)
}

// Messages overrides issue messages by stable code for everything inside v,
// e.g. map[string]string{sk.CodeTooSmall: "too short"}. Issues from failed
// union variants keep their original messages unless their code matches.
func Messages[T any](v Validator[T], byCode map[string]string) Validator[T] {
	base := v.schemaNode().cloneShallow()
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		attempt := &parseContext{path: ctx.path, issues: newValidationError()}
		out, err := v.parse(attempt, raw)
		if len(attempt.issues.Issues) > 0 {
			for i := range attempt.issues.Issues {
				if m, ok := byCode[attempt.issues.Issues[i].Code]; ok {
					attempt.issues.Issues[i].Message = m
				}
			}
			ctx.issues.Issues = append(ctx.issues.Issues, attempt.issues.Issues...)
		}
		return out, err
	}, base)
}

// Message replaces the message of every issue produced by v with a single
// string (use Messages for per-code control).
func Message[T any](v Validator[T], msg string) Validator[T] {
	base := v.schemaNode().cloneShallow()
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		attempt := &parseContext{path: ctx.path, issues: newValidationError()}
		out, err := v.parse(attempt, raw)
		if len(attempt.issues.Issues) > 0 {
			for i := range attempt.issues.Issues {
				attempt.issues.Issues[i].Message = msg
			}
			ctx.issues.Issues = append(ctx.issues.Issues, attempt.issues.Issues...)
		}
		return out, err
	}, base)
}

// SuperRefine runs fn after v validates; fn may add any number of issues at
// arbitrary paths via the IssueCollector.
func SuperRefine[T any](v Validator[T], fn func(T, *IssueCollector)) Validator[T] {
	base := v.schemaNode().cloneShallow()
	return wrapValidator[T](func(ctx *parseContext, raw any) (T, error) {
		out, err := v.parse(ctx, raw)
		if err != nil {
			return out, err
		}
		if len(ctx.issues.Issues) > 0 {
			return out, nil
		}
		fn(out, &IssueCollector{path: ctx.path, ve: ctx.issues})
		return out, nil
	}, base)
}
