# Changelog

## 1.0.0 — 2026-11-18

Initial release: type-safe, fluent validation for Go with Zod-style
ergonomics, JSON Schema Draft 2020-12 emission, HTTP helpers (413/415/422,
body-size limits, errors.As unwrapping) and LLM structured-output strict
mode. Zero third-party dependencies.

### Core

- Primitives: String (trim/case transforms, email/url/uuid/date/ipv4/ipv6/
  regex/prefix/suffix formats), Bool, Enum, Literal, Any
- Numbers: Int, Int64, Float64 with overflow guards, plus fixed-width
  Int8/Int16/Int32, Uint/Uint8/Uint16/Uint32/Uint64 (full 0..2^64-1 range)
  and Float32 via a shared generic core; Coerce everywhere
- Struct with compile-time field binding (json-tag names, snake_case
  fallback, build-time panics for json:"-"), Strict / StripUnknown /
  Partial / DeepPartial / Pick / Omit / KeyOf / CatchAll / Refine /
  SuperRefine
- Containers: Slice (min/max/nonEmpty/unique), Tuple with Rest,
  Map with Keys/KeyPattern record-key validation (propertyNames emission)
- Unions: DiscriminatedUnion (oneOf + discriminator) and non-discriminated
  Union (anyOf, first match, all variant issues on failure)
- Lazy recursive schemas with $defs/$ref materialization (sync.Once,
  race-safe, nested definitions materialize)
- Time() parsing into time.Time with After/Before bounds
- Modifiers: Optional / Nullable / OptionalNull / Default / Describe /
  Refine / Transform / Catch / Preprocess / Pipe / Message / Messages /
  SuperRefine, with Any()/Lift[T] adapters for heterogeneous containers
- Stable issue codes (required, invalid_type, too_small, too_big,
  invalid_format, invalid_enum, unrecognized_key, invalid_union, custom)
- Collect-all ValidationError with JSON paths and a JSON envelope via
  MarshalJSON
- JSON Schema Draft 2020-12 emission incl. WithLLMStrictMode for
  structured-output APIs (all-required, additionalProperties:false,
  constraints in descriptions)
- Null semantics: Optional rejects null, Nullable accepts it,
  OptionalNull accepts both, Default applies on missing or null

### Quality gates

- go test -race green (36 tests incl. fast-path/ParseAny parity and a
  null-semantics matrix), go vet and gofmt clean
- Concurrent stress: 32 goroutines × 2000 parses interleaved with schema
  derivation and JSON Schema emission under -race
- 30 s fuzz over the trailing-JSON decode path: no crashers
- Benchmarks documented in README (single-pass struct decode fast path)
