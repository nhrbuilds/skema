package skema

import (
	"encoding/json"
	"unsafe"
)

func fieldParseRaw[F any](jsonName string, offset uintptr, v Validator[F], fb *fieldBinding) {
	if sv, ok := any(v).(*structValidator[F]); ok {
		inner := sv.schema
		fb.parseRaw = func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
			child := ctx.child(jsonName)
			m, err := decodeJSONObjectRawFromMessage(raw)
			if err != nil {
				child.add(child.path, CodeInvalidFormat, "invalid object")
				return
			}
			var dest F
			inner.parseObject(child, m, &dest)
			if len(child.issues.Issues) > 0 {
				return
			}
			storeField(base, offset, dest)
		}
		fb.deepPartial = func() *fieldBinding {
			dp := sv.schema.deepPartialSchema()
			nb := *fb
			nb.optional = true
			node := dp.node.cloneShallow()
			node.optional = true
			nb.node = node
			nb.parse = func(ctx *parseContext, raw any, base unsafe.Pointer) {
				dest := (*F)(unsafe.Pointer(uintptr(base) + offset))
				child := ctx.child(jsonName)
				if raw == nil {
					return
				}
				obj, ok := raw.(map[string]any)
				if !ok {
					failType(child, "object")
					return
				}
				rawObj := make(map[string]json.RawMessage, len(obj))
				for k, vv := range obj {
					b, err := json.Marshal(vv)
					if err != nil {
						failType(child, "object")
						return
					}
					rawObj[k] = b
				}
				var out F
				dp.parseObject(child, rawObj, &out)
				if len(child.issues.Issues) > 0 {
					return
				}
				*dest = out
			}
			nb.parseRaw = func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
				child := ctx.child(jsonName)
				m, err := decodeJSONObjectRawFromMessage(raw)
				if err != nil {
					child.add(child.path, CodeInvalidFormat, "invalid object")
					return
				}
				var dest F
				dp.parseObject(child, m, &dest)
				if len(child.issues.Issues) > 0 {
					return
				}
				storeField(base, offset, dest)
			}
			return &nb
		}
		return
	}
	if ss, ok := any(v).(*stringSchema); ok {
		fb.parseRaw = func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
			child := ctx.child(jsonName)
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				failType(child, "string")
				return
			}
			out, err := ss.parse(child, s)
			if err != nil || len(child.issues.Issues) > 0 {
				return
			}
			storeField(base, offset, out)
		}
		return
	}
	if is, ok := any(v).(*intSchema); ok {
		fb.parseRaw = func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
			child := ctx.child(jsonName)
			// Numeric literals decode straight into json.Number; string
			// literals are kept as strings and other values fall back to a
			// full decode, so the fast path stays behaviorally identical to
			// ParseAny (including Coerce semantics).
			var val any
			if firstJSONByte(raw) == '"' {
				var str string
				if err := json.Unmarshal(raw, &str); err != nil {
					failType(child, "integer")
					return
				}
				val = str
			} else {
				var num json.Number
				if err := json.Unmarshal(raw, &num); err == nil {
					val = num
				} else {
					decoded, err := anyFromJSON(raw)
					if err != nil {
						failType(child, "integer")
						return
					}
					val = decoded
				}
			}
			out, err := is.parse(child, val)
			if err != nil || len(child.issues.Issues) > 0 {
				return
			}
			storeField(base, offset, out)
		}
		return
	}
	if sl, ok := any(v).(interface {
		bindRaw(string, uintptr, func(*parseContext, any, unsafe.Pointer)) func(*parseContext, json.RawMessage, unsafe.Pointer)
	}); ok {
		fb.parseRaw = sl.bindRaw(jsonName, offset, fb.parse)
		return
	}
	fb.parseRaw = func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
		child := ctx.child(jsonName)
		val, err := anyFromJSON(raw)
		if err != nil {
			child.add(child.path, CodeInvalidFormat, "invalid json")
			return
		}
		fb.parse(ctx, val, base)
	}
}

func storeField[F any](base unsafe.Pointer, offset uintptr, val F) {
	*(*F)(unsafe.Pointer(uintptr(base) + offset)) = val
}

func (s *sliceSchema[T]) bindRaw(jsonName string, offset uintptr, fallback func(ctx *parseContext, raw any, base unsafe.Pointer)) func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
	if _, ok := any(s.elem).(*stringSchema); ok && reflectElemIsString[T]() {
		return func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
			child := ctx.child(jsonName)
			var items []string
			if err := json.Unmarshal(raw, &items); err != nil {
				failType(child, "array")
				return
			}
			arr := make([]any, len(items))
			for i := range items {
				arr[i] = items[i]
			}
			fallback(ctx, arr, base)
		}
	}
	return func(ctx *parseContext, raw json.RawMessage, base unsafe.Pointer) {
		child := ctx.child(jsonName)
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			val, err2 := anyFromJSON(raw)
			if err2 != nil {
				failType(child, "array")
				return
			}
			fallback(ctx, val, base)
			return
		}
		arr := make([]any, len(items))
		for i, item := range items {
			v, err := anyFromJSON(item)
			if err != nil {
				child.add(child.path, CodeInvalidFormat, "invalid array item")
				return
			}
			arr[i] = v
		}
		fallback(ctx, arr, base)
	}
}

func reflectElemIsString[T any]() bool {
	var z T
	switch any(z).(type) {
	case []string:
		return true
	case *[]string:
		return true
	default:
		return false
	}
}
