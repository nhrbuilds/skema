# Changelog

## 1.1.0 — 2026-11-18

Zog/gozod parity release. Everything is additive; no breaking changes to the
1.0.0 API.

### Added

- `Custom(fn)` — user-supplied validators on the same issue-reporting
  pipeline (previously `Validator[T]` was unimplementable outside the
  package).
- `Intersect` — allOf semantics, all issues collected, emitted as
  `allOf`.
- `Object(props, required...)` — dynamic object schemas without a Go struct,
  with `Strict`, `CatchAll` (validated unknown keys are copied to the output),
  `MinKeys`/`MaxKeys`.
- `FromJSONSchema` / `FromJSONSchemaBytes` — compile a JSON Schema document
  (Draft 2020-12 subset: types, enum/const, string/number/array/object
  keywords, oneOf/anyOf/allOf, internal $ref with $defs, cycle-safe) into a
  validator. Fails closed on unsupported keywords.
- `FromStruct[T]` — schema generation from `sk:"..."` struct tags (formats,
  bounds, enum, literal, default, coerce, optional/nullable/required-pointer
  semantics), with `RegisterTagSchema` for nested struct fields.
- `ParseEnv` / `ParseEnvMap` — validate environ slices / maps against a
  schema (Zog zenv parity).
- `DecodeForm` / `DecodeQuery` — validate urlencoded forms and query params
  (Zog zhttp parity; JSON bodies remain DecodeHTTP).
- String formats: `UUIDv4`/`UUIDv6`/`UUIDv7` (version-nibble checks),
  `Base64`, `CIDR`, `JWT` (structural).
- `BigInt` — arbitrary-precision JSON integers into `math/big.Int`.
- `Title` — JSON Schema title per node.
- `ValidationError.Flatten()` / `.Prettify()` — error shaping for forms and
  logs.

### Fixed during development

- `objectSchema.Strict`/`CatchAll` returned copies sharing the source's bound
  parse closure, silently ignoring the derived flags.
- JSON Schema import resolved `#/$defs/Name` to the wrong segment.

### Quality gates

- `go test -race ./...` green (36 → 49 tests), `go vet` and `gofmt` clean.
- 30 s fuzz: ~99k execs, no crashers.
- Benchmarks unchanged vs 1.0.0 (same allocation counts on existing paths).

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
