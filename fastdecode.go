package skema

import (
	"bytes"
	"encoding/json"
)

func decodeJSONObjectRaw(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if err := ensureNoTrailingJSON(dec); err != nil {
		return nil, err
	}
	return obj, nil
}

func decodeJSONObjectRawFromMessage(raw json.RawMessage) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func isJSONNull(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return b == 'n' && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		}
	}
	return true
}

// firstJSONByte returns the first non-whitespace byte of a raw JSON value
// (0 for empty input). Used to distinguish string literals from numbers,
// since json.Number also unmarshals numeric strings.
func firstJSONByte(raw json.RawMessage) byte {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return b
		}
	}
	return 0
}

func anyFromJSON(raw json.RawMessage) (any, error) {
	if isJSONNull(raw) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func finishParse[T any](ctx *parseContext, out T) (T, error) {
	var zero T
	if len(ctx.issues.Issues) > 0 {
		ve := &ValidationError{Issues: append([]Issue(nil), ctx.issues.Issues...)}
		releaseParseContext(ctx)
		return zero, ve
	}
	releaseParseContext(ctx)
	return out, nil
}
