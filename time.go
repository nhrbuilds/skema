package skema

import "time"

type timeSchema struct {
	*baseValidator[time.Time]
	node   *schemaNode
	after  *time.Time
	before *time.Time
}

// Time parses an RFC 3339 date-time string into a time.Time.
func Time() *timeSchema {
	s := &timeSchema{node: &schemaNode{types: []string{"string"}, format: "date-time"}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

// After requires the parsed time to be strictly after t.
func (s *timeSchema) After(t time.Time) *timeSchema {
	s.after = &t
	return s
}

// Before requires the parsed time to be strictly before t.
func (s *timeSchema) Before(t time.Time) *timeSchema {
	s.before = &t
	return s
}

func (s *timeSchema) Optional() Validator[*time.Time]     { return Optional(s) }
func (s *timeSchema) Nullable() Validator[*time.Time]     { return Nullable(s) }
func (s *timeSchema) OptionalNull() Validator[*time.Time] { return OptionalNull(s) }
func (s *timeSchema) Default(v time.Time) Validator[time.Time] {
	return Default(s, v)
}
func (s *timeSchema) Describe(text string) Validator[time.Time] { return Describe(s, text) }
func (s *timeSchema) Refine(fn func(time.Time) bool, msg string) Validator[time.Time] {
	return Refine(s, fn, msg)
}

func (s *timeSchema) parse(ctx *parseContext, raw any) (time.Time, error) {
	var zero time.Time
	str, ok := raw.(string)
	if !ok {
		failType(ctx, "string")
		return zero, nil
	}
	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid date-time")
		return zero, nil
	}
	if s.after != nil && !t.After(*s.after) {
		ctx.add(ctx.path, CodeTooSmall, "must be after "+s.after.Format(time.RFC3339))
	}
	if s.before != nil && !t.Before(*s.before) {
		ctx.add(ctx.path, CodeTooBig, "must be before "+s.before.Format(time.RFC3339))
	}
	return t, nil
}
