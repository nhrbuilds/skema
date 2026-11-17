package skema

import (
	"strconv"
	"strings"
)

type boolSchema struct {
	*baseValidator[bool]
	node   *schemaNode
	coerce bool
}

// Bool validates booleans.
func Bool() *boolSchema {
	s := &boolSchema{node: &schemaNode{types: []string{"boolean"}}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *boolSchema) Coerce() *boolSchema { s.coerce = true; return s }

func (s *boolSchema) parse(ctx *parseContext, raw any) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		if !s.coerce {
			failType(ctx, "boolean")
			return false, nil
		}
		return parseBoolString(ctx, v)
	case float64:
		if !s.coerce {
			failType(ctx, "boolean")
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
		if v == 0 {
			return false, nil
		}
		failType(ctx, "boolean")
		return false, nil
	case int:
		if !s.coerce {
			failType(ctx, "boolean")
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
		if v == 0 {
			return false, nil
		}
		failType(ctx, "boolean")
		return false, nil
	default:
		failType(ctx, "boolean")
		return false, nil
	}
}

func parseBoolString(ctx *parseContext, v string) (bool, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		if b, err := strconv.ParseBool(v); err == nil {
			return b, nil
		}
		failType(ctx, "boolean")
		return false, nil
	}
}
