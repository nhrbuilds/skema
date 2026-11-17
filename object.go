package skema

import "slices"

// Object validates a JSON object with heterogeneous per-key validators — a
// dynamic shape that needs no Go struct. Wrap differently-typed validators
// with Lift. Unknown keys pass through into the output unless the schema is
// strict or a CatchAll schema is set.
func Object(properties map[string]Validator[any], required ...string) *objectSchema {
	if properties == nil {
		properties = map[string]Validator[any]{}
	}
	s := &objectSchema{
		props:    properties,
		required: make(map[string]struct{}, len(required)),
		node: &schemaNode{
			types:      []string{"object"},
			properties: map[string]*schemaNode{},
		},
	}
	for _, r := range required {
		s.required[r] = struct{}{}
	}
	s.rebuildNode()
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

type objectSchema struct {
	*baseValidator[map[string]any]
	node     *schemaNode
	props    map[string]Validator[any]
	required map[string]struct{}
	catchAll Validator[any]
	strict   bool
	minKeys  *int
	maxKeys  *int
}

func (s *objectSchema) rebuildNode() {
	s.node.properties = make(map[string]*schemaNode, len(s.props))
	req := make([]string, 0, len(s.required))
	for name := range s.required {
		req = append(req, name)
	}
	slices.Sort(req)
	s.node.required = req
	for name, v := range s.props {
		s.node.properties[name] = v.schemaNode()
	}
	ap := !s.strict && s.catchAll == nil
	s.node.additional = &ap
	if s.catchAll != nil {
		s.node.additionalItem = s.catchAll.schemaNode()
	}
}

// Strict rejects unknown keys instead of passing them through.
func (s *objectSchema) Strict() *objectSchema {
	return s.derive(true, s.catchAll)
}

// CatchAll validates unknown keys' values against v and copies the validated
// values into the output map.
func (s *objectSchema) CatchAll(v Validator[any]) *objectSchema {
	return s.derive(s.strict, v)
}

// derive returns a validator over a fresh schemaNode and a freshly bound
// parse closure so derivatives never mutate — or stay bound to — the source.
func (s *objectSchema) derive(strict bool, catchAll Validator[any]) *objectSchema {
	cp := *s
	cp.node = cp.node.cloneShallow()
	cp.strict = strict
	cp.catchAll = catchAll
	cp.rebuildNode()
	return &objectSchema{
		baseValidator: bindValidator(cp.node, cp.parse),
		node:          cp.node,
		props:         cp.props,
		required:      cp.required,
		catchAll:      cp.catchAll,
		strict:        cp.strict,
		minKeys:       cp.minKeys,
		maxKeys:       cp.maxKeys,
	}
}

// MinKeys requires at least n keys.
func (s *objectSchema) MinKeys(n int) *objectSchema {
	s.minKeys = &n
	s.node.minProperties = &n
	return s
}

// MaxKeys allows at most n keys.
func (s *objectSchema) MaxKeys(n int) *objectSchema {
	s.maxKeys = &n
	s.node.maxProperties = &n
	return s
}

func (s *objectSchema) parse(ctx *parseContext, raw any) (map[string]any, error) {
	if raw == nil {
		ctx.add(ctx.path, CodeRequired, "expected object")
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		failType(ctx, "object")
		return nil, nil
	}
	if s.minKeys != nil && len(obj) < *s.minKeys {
		ctx.add(ctx.path, CodeTooSmall, "minimum keys")
	}
	if s.maxKeys != nil && len(obj) > *s.maxKeys {
		ctx.add(ctx.path, CodeTooBig, "maximum keys")
	}
	if s.strict {
		for k := range obj {
			if _, known := s.props[k]; !known {
				ctx.add(joinPath(ctx.path, k), CodeUnrecognizedKey, "unrecognized key")
			}
		}
	}
	for name := range s.required {
		if _, present := obj[name]; !present {
			ctx.add(joinPath(ctx.path, name), CodeRequired, "required")
		}
	}
	out := make(map[string]any, len(obj))
	for name, v := range s.props {
		val, present := obj[name]
		if !present {
			continue
		}
		child := ctx.child(name)
		parsed, err := v.parse(child, val)
		if err != nil {
			continue
		}
		if len(child.issues.Issues) > 0 {
			continue
		}
		out[name] = parsed
	}
	if !s.strict {
		for k, val := range obj {
			if _, known := s.props[k]; known {
				continue
			}
			if s.catchAll == nil {
				out[k] = val // pass through
				continue
			}
			child := ctx.child(k)
			parsed, err := s.catchAll.parse(child, val)
			if err != nil {
				continue
			}
			if len(child.issues.Issues) > 0 {
				continue
			}
			out[k] = parsed
		}
	}
	return out, nil
}
