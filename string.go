package skema

import (
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type stringSchema struct {
	*baseValidator[string]
	node     *schemaNode
	trim     bool
	lower    bool
	upper    bool
	min      *int
	max      *int
	exactLen *int
	nonEmpty bool
	prefix   string
	suffix   string
	re       *regexp.Regexp
	email    bool
	asURL    bool
	uuid     bool
	dateTime bool
	date     bool
	ipv4     bool
	ipv6     bool
}

// String builds a string validator.
func String() *stringSchema {
	s := &stringSchema{node: &schemaNode{types: []string{"string"}}}
	s.baseValidator = bindValidator(s.node, s.parse)
	return s
}

func (s *stringSchema) Trim() *stringSchema  { s.trim = true; return s }
func (s *stringSchema) Lower() *stringSchema { s.lower = true; return s }
func (s *stringSchema) Upper() *stringSchema { s.upper = true; return s }
func (s *stringSchema) Min(n int) *stringSchema {
	s.min = &n
	s.node.minLength = &n
	return s
}
func (s *stringSchema) Max(n int) *stringSchema {
	s.max = &n
	s.node.maxLength = &n
	return s
}
func (s *stringSchema) Len(n int) *stringSchema {
	s.exactLen = &n
	s.node.lenExact = &n
	s.node.minLength = &n
	s.node.maxLength = &n
	return s
}
func (s *stringSchema) NonEmpty() *stringSchema { s.nonEmpty = true; return s }
func (s *stringSchema) Prefix(p string) *stringSchema {
	s.prefix = p
	return s
}
func (s *stringSchema) Suffix(p string) *stringSchema {
	s.suffix = p
	return s
}
func (s *stringSchema) Regex(pattern string) *stringSchema {
	s.re = regexp.MustCompile(pattern)
	s.node.pattern = pattern
	return s
}
func (s *stringSchema) Email() *stringSchema {
	s.email = true
	s.node.format = "email"
	return s
}
func (s *stringSchema) URL() *stringSchema {
	s.asURL = true
	s.node.format = "uri"
	return s
}
func (s *stringSchema) UUID() *stringSchema {
	s.uuid = true
	s.node.format = "uuid"
	return s
}
func (s *stringSchema) DateTime() *stringSchema {
	s.dateTime = true
	s.node.format = "date-time"
	return s
}
func (s *stringSchema) Date() *stringSchema {
	s.date = true
	s.node.format = "date"
	return s
}
func (s *stringSchema) IPv4() *stringSchema {
	s.ipv4 = true
	s.node.format = "ipv4"
	return s
}
func (s *stringSchema) IPv6() *stringSchema {
	s.ipv6 = true
	s.node.format = "ipv6"
	return s
}
func (s *stringSchema) Describe(text string) Validator[string] { return Describe(s, text) }
func (s *stringSchema) Optional() Validator[*string]           { return Optional(s) }
func (s *stringSchema) Nullable() Validator[*string]           { return Nullable(s) }
func (s *stringSchema) OptionalNull() Validator[*string]       { return OptionalNull(s) }
func (s *stringSchema) Default(v string) Validator[string]     { return Default(s, v) }
func (s *stringSchema) Refine(fn func(string) bool, msg string) Validator[string] {
	return Refine(s, fn, msg)
}

func (s *stringSchema) schemaNode() *schemaNode { return s.node }

func (s *stringSchema) parse(ctx *parseContext, raw any) (string, error) {
	var zero string
	str, ok := raw.(string)
	if !ok {
		failType(ctx, "string")
		return zero, nil
	}
	if s.trim {
		str = strings.TrimSpace(str)
	}
	if s.lower {
		str = strings.ToLower(str)
	}
	if s.upper {
		str = strings.ToUpper(str)
	}
	runes := utf8.RuneCountInString(str)
	if s.nonEmpty && runes == 0 {
		ctx.add(ctx.path, CodeTooSmall, "must not be empty")
	}
	if s.exactLen != nil && runes != *s.exactLen {
		ctx.add(ctx.path, CodeInvalidFormat, "length must be exactly "+itoa(*s.exactLen))
	}
	if s.min != nil && runes < *s.min {
		ctx.add(ctx.path, CodeTooSmall, "minimum length "+itoa(*s.min))
	}
	if s.max != nil && runes > *s.max {
		ctx.add(ctx.path, CodeTooBig, "maximum length "+itoa(*s.max))
	}
	if s.prefix != "" && !strings.HasPrefix(str, s.prefix) {
		ctx.add(ctx.path, CodeInvalidFormat, "must start with "+s.prefix)
	}
	if s.suffix != "" && !strings.HasSuffix(str, s.suffix) {
		ctx.add(ctx.path, CodeInvalidFormat, "must end with "+s.suffix)
	}
	if s.re != nil && !s.re.MatchString(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "pattern mismatch")
	}
	if s.email && !validEmail(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid email")
	}
	if s.asURL && !validURL(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid url")
	}
	if s.uuid && !validUUID(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid uuid")
	}
	if s.dateTime && !validDateTime(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid date-time")
	}
	if s.date && !validDate(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid date")
	}
	if s.ipv4 && !validIPv4(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid ipv4")
	}
	if s.ipv6 && !validIPv6(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid ipv6")
	}
	return str, nil
}

func validEmail(s string) bool {
	if strings.ContainsAny(s, " \t\r\n<>\"") {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 || !strings.Contains(parts[1], ".") {
		return false
	}
	return addr.Address == s
}

func validURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validUUID(s string) bool { return uuidRe.MatchString(s) }

func validDateTime(s string) bool {
	if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func validIPv4(s string) bool {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return false
	}
	return addr.Is4()
}

func validIPv6(s string) bool {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return false
	}
	return addr.Is6()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
