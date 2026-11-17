# skema

Type-safe, fluent validation for Go—Zod-style ergonomics with JSON Schema Draft 2020-12, HTTP helpers, and LLM strict mode. **Zero third-party dependencies.**

## Quick start

```go
import sk "github.com/nhrbuilds/skema"

schema := sk.Struct(func(s *sk.ObjectBuilder[User]) {
    sk.Field(s, &s.Target.Email, sk.String().Trim().Lower().Email())
    sk.Field(s, &s.Target.Age, sk.Int().Coerce().Min(18))
})

user, err := schema.ParseString(`{"email":"me@example.com","age":30}`)
```

Field names come from the `json` tag; untagged fields use snake_case, and `json:"-"` fields panic at schema build time.

## Features

- Stable issue codes (`sk.CodeRequired`, `CodeInvalidType`, `CodeInvalidUnion`, …)
- Collect-all `ValidationError` with JSON paths and a JSON envelope via `MarshalJSON`
- `DecodeHTTP` + `WriteValidationError` (413/415/422, body-size limits, `errors.As` unwrapping)
- `Lazy` + `$defs` / `$ref` for recursive schemas (`sync.Once`, race-safe, nested definitions materialize)
- `DiscriminatedUnion` and non-discriminated `Union` (first-match, all variant issues on failure)
- `Tuple` with positional validation and `Rest` for trailing items
- `Time()` parsing into `time.Time` with `After`/`Before` bounds
- Fixed-width numerics: `Int8`/`Int16`/`Int32`, `Uint`/`Uint8`/`Uint16`/`Uint32`/`Uint64`, `Float32`
- Record key validation: `Map().Keys(...)` / `KeyPattern(...)` (emitted as `propertyNames`)
- `CatchAll` on structs (validates unknown keys, emitted as `additionalProperties`)
- `Catch`, `Preprocess`, `Pipe`, `Any`/`Lift` adapters
- Per-schema custom messages: `Message(v, "msg")` / `Messages(v, map[code]string)`
- `SuperRefine` with `IssueCollector` for multi-issue cross-field rules
- Object surgery: `Pick`, `Omit`, `KeyOf`, `Partial`, `DeepPartial` (recurses into nested structs)
- `WithLLMStrictMode()` for structured-output APIs
- `Struct().Strict()` rejects unknown keys; `StripUnknown()` ignores them
- Derivatives never mutate the schema they were built from

## Null semantics

| Modifier | Key missing | JSON `null` | Result type |
|---|---|---|---|
| `Optional()` | accepted | **rejected** | `*T` |
| `Nullable()` | required | accepted | `*T` |
| `OptionalNull()` | accepted | accepted | `*T` |
| `Default(v)` | → `v` | → `v` | `T` |

## Error message customization

Per-schema overrides:

```go
sk.Field(b, &b.Target.Email, sk.Message(sk.String().Email(), "enter a valid email"))

s := sk.Messages(sk.String().Min(5), map[string]string{
    sk.CodeTooSmall: "custom too small",
})
```

Or remap stable codes downstream for i18n:

```go
var issueMessages = map[string]string{
    sk.CodeRequired:      "This field is required.",
    sk.CodeInvalidFormat: "This field is malformed.",
}
for i := range ve.Issues {
    if msg, ok := issueMessages[ve.Issues[i].Code]; ok {
        ve.Issues[i].Message = msg
    }
}
```

## What's intentionally not here

`bigint`/`symbol`/`void`/`never` schemas, `z.function`/`z.promise`, branded types, and object `.extend` are static-type concerns Go's compiler already owns, or impossible without struct-type merging. Dates parse into `time.Time`; there is no string-only date *target* other than that.

## How skema compares

skema is Zod-*inspired*, not the first — [Zog](https://github.com/Oudwins/zog) is the most established Zod-like Go library and a great choice for forms/env/i18n-heavy validation. Feature comparison as documented at time of writing (Nov 2026):

| Library | Style | JSON Schema out | LLM strict mode | HTTP helpers | Deps |
|---|---|---|---|---|---|
| **skema** | Zod-style | ✓ Draft 2020-12 | ✓ | ✓ JSON bodies (413/415/422) | 0 |
| [Zog](https://github.com/Oudwins/zog) | Zod-inspired | — | — | forms & query params (`zhttp`) | 0 |
| [gozod](https://github.com/kaptinlin/gozod) | Zod v4-inspired | ✓ Draft 2020-12 (+ import) | — | — | several |
| [zod-go](https://github.com/aymaneallaoui/zod-go) | Zod-inspired | — | — | — | not stated |
| [go-playground/validator](https://github.com/go-playground/validator) | struct tags | — | — | — | 0 |
| [ozzo-validation](https://github.com/go-ozzo/ozzo-validation) | rules-based | — | — | — | 0 |

When to pick what: **Zog** for the most mature Zod-like API with i18n and env/form helpers; **gozod** if you want Zod v4 semantics, a two-way JSON Schema bridge, or struct-tag/codegen integration; **skema** if JSON Schema emission plus LLM structured-output strict mode, plain-struct fast-path parsing, or HTTP JSON-body validation with 422 semantics are on your critical path; **go-playground/validator** or **ozzo-validation** for ubiquitous struct-tag/rule-style validation.

## Performance

Struct `ParseBytes` / `ParseString` use a **single-pass** decode into `map[string]json.RawMessage`, with typed fast paths for strings, ints, nested structs, and slices. Use these entry points in HTTP handlers and hot paths instead of `ParseAny` when parsing JSON bytes. Both paths are behaviorally identical (including `Coerce`); tests enforce parity.

Measured with `go test -bench=. -benchmem -run='^$' ./tests/` (Intel i5-8350U, Go 1.27, linux/amd64):

| Benchmark | Result |
|---|---|
| 5-field struct w/ email format validation | ~15 µs/op, 3.4 KB, 55 allocs |
| Concurrent parse (all cores) | ~7 µs/op |
| 200-item nested-struct array (25 KB) | ~2.0 ms/op, 3.2 MB/s |
| JSON Schema emission | ~7 µs/op |
| Email format validation | ~4.2 µs/op |

vs. raw `encoding/json` `Unmarshal` (no validation) at ~3.2 µs/op for the same struct. Recursive `Lazy` schemas parse through the generic path (~0.15 MB/s at 64 levels deep) — avoid deep recursion in hot loops or flatten it.

## Development

```bash
make test    # go test -race -v ./...
make vet
make bench   # all benchmarks incl. large arrays, deep nesting, concurrency
make fuzz    # trailing-JSON fuzz (30s)
```

## Author

Built by [Nowshad Hossain Rahat](https://nhrdev.com)

- Portfolio: [nhrdev.com](https://nhrdev.com)
- X: [@nhrdev](https://x.com/nhrdev)
- GitHub: [@nowshad-hossain-rahat](https://github.com/nowshad-hossain-rahat)

## License

MIT
