package skema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type parseContext struct {
	path   string
	issues *ValidationError
}

var parseCtxPool sync.Pool

func newParseContext(path string) *parseContext {
	v, ok := parseCtxPool.Get().(*parseContext)
	if !ok {
		v = &parseContext{issues: &ValidationError{Issues: make([]Issue, 0, 8)}}
	}
	v.path = path
	v.issues.Issues = v.issues.Issues[:0]
	return v
}

func releaseParseContext(c *parseContext) {
	if c == nil {
		return
	}
	parseCtxPool.Put(c)
}

func (c *parseContext) child(path string) *parseContext {
	return &parseContext{
		path:   joinPath(c.path, path),
		issues: c.issues,
	}
}

func (c *parseContext) add(path, code, msg string) {
	if path == "" {
		path = c.path
	}
	c.issues.Add(path, code, msg)
}

func decodeJSONValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if err := ensureNoTrailingJSON(dec); err != nil {
		return nil, err
	}
	return raw, nil
}

func decodeJSONReader(r io.Reader) (any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if err := ensureNoTrailingJSON(dec); err != nil {
		return nil, err
	}
	return raw, nil
}

func ensureNoTrailingJSON(dec *json.Decoder) error {
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("trailing json: %w", err)
	}
	return fmt.Errorf("trailing json after root value")
}
