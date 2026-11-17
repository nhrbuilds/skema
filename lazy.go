package skema

import "sync"

type lazySchema[T any] struct {
	*baseValidator[T]
	name string
	fn   func() Validator[T]
	once sync.Once
	res  Validator[T]
}

// Lazy defers schema construction for recursive types and emits $ref/$defs.
func Lazy[T any](name string, fn func() Validator[T]) Validator[T] {
	l := &lazySchema[T]{name: name, fn: fn}
	refNode := &schemaNode{
		ref:     "#/$defs/" + name,
		defName: name,
	}
	refNode.resolveDef = func() *schemaNode {
		l.once.Do(func() {
			l.res = fn()
		})
		return l.res.schemaNode()
	}
	l.baseValidator = bindValidator[T](refNode, l.parse)
	return l
}

func (l *lazySchema[T]) parse(ctx *parseContext, raw any) (T, error) {
	var zero T
	l.once.Do(func() {
		l.res = l.fn()
	})
	out, err := l.res.parse(ctx, raw)
	if err != nil {
		return zero, err
	}
	return out, nil
}
