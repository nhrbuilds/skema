// Package skema provides type-safe, fluent schema validation for Go with
// Zod-style ergonomics, JSON Schema Draft 2020-12 emission, HTTP helpers and
// LLM structured-output strict mode. It has zero third-party dependencies.
//
// Build a schema once and reuse it; schemas are immutable after construction
// and safe for concurrent use:
//
//	import sk "github.com/nhrbuilds/skema"
//
//	type User struct {
//	    Email string `json:"email"`
//	    Age   int    `json:"age"`
//	}
//
//	schema := sk.Struct(func(s *sk.ObjectBuilder[User]) {
//	    sk.Field(s, &s.Target.Email, sk.String().Trim().Lower().Email())
//	    sk.Field(s, &s.Target.Age, sk.Int().Coerce().Min(18))
//	})
//
//	user, err := schema.ParseString(`{"email":"me@example.com","age":30}`)
//
// Parsing collects every validation issue into a *ValidationError with stable
// machine-readable codes (CodeRequired, CodeInvalidType, ...). Derivatives
// such as Strict, StripUnknown, Partial, Refine and the modifier wrappers
// never mutate the schema they were derived from.
//
// Nullability: Optional accepts a missing key but rejects JSON null; Nullable
// accepts null but the key remains required; OptionalNull accepts both; and
// Default applies its value when the key is missing or null.
//
// In HTTP handlers use DecodeHTTP with WriteValidationError to answer with
// 413/415/422 responses, and prefer ParseBytes/ParseString over ParseAny in
// hot paths — struct schemas use a single-pass decode there.
//
// Beyond structs the package offers non-discriminated Union, positional
// Tuple, record key validation, Time parsing into time.Time, fixed-width
// integers (Int8..Uint64, Float32), Catch/Preprocess/Pipe combinators,
// custom issue messages, and object surgery via Pick/Omit/KeyOf/DeepPartial.
package skema
