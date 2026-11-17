package skema

import (
	"encoding/base64"
	"encoding/json"
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
	uuidVer  int
	base64   bool
	cidr     bool
	jwt      bool
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
	s.uuidVer = 0
	s.node.format = "uuid"
	return s
}

// UUIDv4 requires a version-4 (random) UUID.
func (s *stringSchema) UUIDv4() *stringSchema {
	s.uuid = true
	s.uuidVer = 4
	s.node.format = "uuid"
	return s
}

// UUIDv6 requires a version-6 UUID.
func (s *stringSchema) UUIDv6() *stringSchema {
	s.uuid = true
	s.uuidVer = 6
	s.node.format = "uuid"
	return s
}

// UUIDv7 requires a version-7 (time-ordered) UUID.
func (s *stringSchema) UUIDv7() *stringSchema {
	s.uuid = true
	s.uuidVer = 7
	s.node.format = "uuid"
	return s
}

// Base64 requires standard (padded or raw) base64 content.
func (s *stringSchema) Base64() *stringSchema {
	s.base64 = true
	s.node.format = "base64"
	return s
}

// CIDR requires an IP prefix in CIDR notation (e.g. 10.0.0.0/8).
func (s *stringSchema) CIDR() *stringSchema {
	s.cidr = true
	s.node.format = "cidr"
	return s
}

// JWT requires a structurally valid JSON Web Token (three base64url
// segments with JSON-object header and payload). No signature verification.
func (s *stringSchema) JWT() *stringSchema {
	s.jwt = true
	s.node.format = "jwt"
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
	if s.uuid && s.uuidVer > 0 && validUUID(str) && !uuidVersionIs(str, s.uuidVer) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid uuid version")
	}
	if s.base64 && !validBase64(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid base64")
	}
	if s.cidr && !validCIDR(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid cidr")
	}
	if s.jwt && !validJWT(str) {
		ctx.add(ctx.path, CodeInvalidFormat, "invalid jwt")
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

// uuidVersionIs checks the version nibble (third group's first hex char).
func uuidVersionIs(s string, v int) bool {
	if len(s) < 15 {
		return false
	}
	b := s[14]
	var n int
	switch {
	case b >= '0' && b <= '9':
		n = int(b - '0')
	case b >= 'a' && b <= 'f':
		n = int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		n = int(b-'A') + 10
	default:
		return false
	}
	return n == v
}

func validBase64(s string) bool {
	if _, err := base64.StdEncoding.DecodeString(s); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(s)
	return err == nil
}

func validCIDR(s string) bool {
	_, err := netip.ParsePrefix(s)
	return err == nil
}

func validJWT(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for i, p := range parts {
		if p == "" {
			return false
		}
		decoded, err := base64.RawURLEncoding.DecodeString(p)
		if err != nil {
			return false
		}
		if i < 2 {
			var header map[string]any
			if json.Unmarshal(decoded, &header) != nil {
				return false
			}
		}
	}
	return true
}

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
