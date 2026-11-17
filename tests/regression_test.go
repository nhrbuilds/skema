package skema_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	sk "github.com/nhrbuilds/skema"
)

func decodeTree(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func schemaJSON(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// defsOf round-trips through JSON so nested map types can be asserted uniformly.
func defsOf(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	var generic map[string]any
	if err := json.Unmarshal(schemaJSON(t, m), &generic); err != nil {
		t.Fatal(err)
	}
	defs, ok := generic["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("missing $defs: %v", m)
	}
	return defs
}

// Regression: Strict/StripUnknown/Partial/Refine must never mutate the node of
// the validator they were derived from.
func TestDerivedSchemasDoNotMutateOriginal(t *testing.T) {
	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	full := sk.Struct(func(s *sk.ObjectBuilder[User]) {
		sk.Field(s, &s.Target.Name, sk.String().NonEmpty())
		sk.Field(s, &s.Target.Age, sk.Int().Min(0))
	})

	before := schemaJSON(t, mustSchemaMap(t, full))
	_ = full.Strict()
	_ = full.StripUnknown()
	_ = full.Partial()
	_ = full.Refine(func(User) bool { return true }, "x")
	after := schemaJSON(t, mustSchemaMap(t, full))

	if !bytes.Equal(before, after) {
		t.Fatalf("original schema mutated:\nbefore: %s\nafter:  %s", before, after)
	}
	// Parse behavior unchanged: unknown keys still accepted by the original.
	if _, err := full.ParseString(`{"name":"a","age":1,"extra":true}`); err != nil {
		t.Fatalf("original became strict: %v", err)
	}
}

func mustSchemaMap(t *testing.T, v interface {
	JSONSchemaMap(...sk.SchemaOption) (map[string]any, error)
}) map[string]any {
	t.Helper()
	m, err := v.JSONSchemaMap()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStrictAndStripUnknownRoundTrip(t *testing.T) {
	type User struct {
		Name string `json:"name"`
	}
	base := sk.Struct(func(s *sk.ObjectBuilder[User]) {
		sk.Field(s, &s.Target.Name, sk.String())
	})
	strict := base.Strict()
	if _, err := strict.ParseString(`{"name":"a","extra":1}`); err == nil {
		t.Fatal("strict must reject unknown keys")
	}

	// StripUnknown returns to lenient parsing, even after Strict.
	loose := strict.StripUnknown()
	u, err := loose.ParseString(`{"name":"a","extra":1}`)
	if err != nil {
		t.Fatalf("strip unknown must ignore extra keys: %v", err)
	}
	if u.Name != "a" {
		t.Fatalf("got %+v", u)
	}
	// The strict schema itself is untouched by deriving a lenient copy.
	if _, err := strict.ParseString(`{"name":"a","extra":1}`); err == nil {
		t.Fatal("strict schema lost its strictness")
	}
}

func TestPartialKeepsOriginalRequired(t *testing.T) {
	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	full := sk.Struct(func(s *sk.ObjectBuilder[User]) {
		sk.Field(s, &s.Target.Name, sk.String())
		sk.Field(s, &s.Target.Age, sk.Int())
	})
	patch := full.Partial()
	if _, err := patch.ParseString(`{"name":"x"}`); err != nil {
		t.Fatalf("patch must allow missing fields: %v", err)
	}
	if _, err := full.ParseString(`{"name":"x"}`); err == nil {
		t.Fatal("original must still require age")
	}
	before := schemaJSON(t, mustSchemaMap(t, full))
	_ = full.Partial()
	after := schemaJSON(t, mustSchemaMap(t, full))
	if !bytes.Equal(before, after) {
		t.Fatalf("Partial mutated original JSON Schema:\n%s\n%s", before, after)
	}
}

// Regression: the struct fast path (ParseBytes/ParseString) must coerce
// strings exactly like ParseAny.
func TestCoerceFastPathParity(t *testing.T) {
	type Coerced struct {
		Age   int     `json:"age"`
		Score float64 `json:"score"`
	}
	schema := sk.Struct(func(s *sk.ObjectBuilder[Coerced]) {
		sk.Field(s, &s.Target.Age, sk.Int().Coerce())
		sk.Field(s, &s.Target.Score, sk.Float64().Coerce())
	})
	raw := `{"age":"25","score":"3.5"}`

	fast, err := schema.ParseString(raw)
	if err != nil {
		t.Fatalf("fast path coerce failed: %v", err)
	}
	slow, err := schema.ParseAny(decodeTree(t, raw))
	if err != nil {
		t.Fatalf("ParseAny coerce failed: %v", err)
	}
	if fast.Age != 25 || fast.Score != 3.5 || fast != slow {
		t.Fatalf("parity mismatch: fast %+v slow %+v", fast, slow)
	}

	// Top-level coerce parity as well.
	if v, err := sk.Int().Coerce().ParseString(`"25"`); err != nil || v != 25 {
		t.Fatalf("top-level coerce: %d %v", v, err)
	}

	// Without Coerce, strings fail identically on both paths.
	strictSchema := sk.Struct(func(s *sk.ObjectBuilder[Coerced]) {
		sk.Field(s, &s.Target.Age, sk.Int())
	})
	if _, err := strictSchema.ParseString(`{"age":"25"}`); err == nil {
		t.Fatal("fast path must reject string without coerce")
	}
	if _, err := strictSchema.ParseAny(decodeTree(t, `{"age":"25"}`)); err == nil {
		t.Fatal("ParseAny must reject string without coerce")
	}
}

// lazyInner/lazyOuter are package-level because Go function-local type
// declarations cannot forward-reference each other (no mutual recursion).
type lazyInner struct {
	Name  string    `json:"name"`
	Outer lazyOuter `json:"outer"`
}

type lazyOuter struct {
	ID     int         `json:"id"`
	Inners []lazyInner `json:"inners"`
}

// Regression: Lazy definitions registered while compiling other definitions
// (nested or indirect recursion) must all appear in $defs.
func TestNestedLazyDefsMaterialize(t *testing.T) {
	var outer sk.Validator[lazyOuter]
	inner := sk.Lazy("Inner", func() sk.Validator[lazyInner] {
		return sk.Struct(func(s *sk.ObjectBuilder[lazyInner]) {
			sk.Field(s, &s.Target.Name, sk.String())
			sk.Field(s, &s.Target.Outer, outer)
		})
	})
	outer = sk.Lazy("Outer", func() sk.Validator[lazyOuter] {
		return sk.Struct(func(s *sk.ObjectBuilder[lazyOuter]) {
			sk.Field(s, &s.Target.ID, sk.Int())
			sk.Field(s, &s.Target.Inners, sk.Slice(inner))
		})
	})

	// Parse through the mutually recursive pair.
	v, err := outer.ParseString(`{"id":1,"inners":[{"name":"a","outer":{"id":2,"inners":[]}}]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v.ID != 1 || len(v.Inners) != 1 {
		t.Fatalf("parse result %+v", v)
	}

	m := mustSchemaMap(t, outer)
	defs := defsOf(t, m)
	for _, name := range []string{"Outer", "Inner"} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("$defs missing %q:\n%s", name, schemaJSON(t, m))
		}
	}
}

// Regression: a Lazy schema referenced from a plain struct field must be
// materialized too (it is not the root definition).
func TestLazyFieldInStructEmitsDef(t *testing.T) {
	type node struct {
		Value    int    `json:"value"`
		Children []node `json:"children,omitempty"`
	}
	var nodeSchema sk.Validator[node]
	nodeSchema = sk.Lazy("Node", func() sk.Validator[node] {
		return sk.Struct(func(s *sk.ObjectBuilder[node]) {
			sk.Field(s, &s.Target.Value, sk.Int())
			sk.Field(s, &s.Target.Children, sk.Slice(nodeSchema).Optional())
		})
	})
	type holder struct {
		Tree *node `json:"tree,omitempty"`
	}
	holderSchema := sk.Struct(func(s *sk.ObjectBuilder[holder]) {
		sk.Field(s, &s.Target.Tree, sk.OptionalNull(nodeSchema))
	})

	m := mustSchemaMap(t, holderSchema)
	defs := defsOf(t, m)
	if _, ok := defs["Node"]; !ok {
		t.Fatalf("$defs missing Node:\n%s", schemaJSON(t, m))
	}

	h, err := holderSchema.ParseString(`{"tree":{"value":1,"children":[{"value":2}]}}`)
	if err != nil || h.Tree == nil || len(h.Tree.Children) != 1 {
		t.Fatalf("parse: %+v %v", h, err)
	}
	if _, err := holderSchema.ParseString(`{"tree":null}`); err != nil {
		t.Fatalf("OptionalNull must accept null: %v", err)
	}
}

func TestNullSemantics(t *testing.T) {
	// Optional rejects JSON null on both paths.
	if _, err := sk.String().Optional().ParseString(`null`); err == nil {
		t.Fatal("Optional must reject null")
	}

	type User struct {
		Nickname *string `json:"nickname"`
		Bio      *string `json:"bio"`
		Company  *string `json:"company"`
		Plan     string  `json:"plan"`
		OnlyNull *string `json:"only_null"`
	}
	schema := sk.Struct(func(s *sk.ObjectBuilder[User]) {
		sk.Field(s, &s.Target.Nickname, sk.String().Min(1).Optional())      // missing ok, null rejected
		sk.Field(s, &s.Target.Bio, sk.String().Nullable())                  // null ok, missing required
		sk.Field(s, &s.Target.Company, sk.String().OptionalNull())          // both ok
		sk.Field(s, &s.Target.Plan, sk.Enum("free", "pro").Default("free")) // missing or null -> default
		sk.Field(s, &s.Target.OnlyNull, sk.String().OptionalNull())
	})

	// Missing optional fields stay nil; null bio is accepted; defaults applied.
	u, err := schema.ParseString(`{"bio":null,"only_null":null}`)
	if err != nil {
		t.Fatalf("valid null usage rejected: %v", err)
	}
	if u.Nickname != nil || u.Company != nil || u.OnlyNull != nil || u.Bio != nil {
		t.Fatalf("pointers must be nil: %+v", u)
	}
	if u.Plan != "free" {
		t.Fatalf("default not applied: %+v", u)
	}

	// null on an Optional field is now rejected.
	if _, err := schema.ParseString(`{"nickname":null,"bio":null}`); err == nil {
		t.Fatal("Optional field must reject null")
	}

	// Missing Nullable field is still required.
	_, errMissing := schema.ParseString(`{}`)
	if errMissing == nil {
		t.Fatal("Nullable field missing must be required")
	}
	ve, ok := errMissing.(*sk.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", errMissing)
	}
	foundRequired := false
	for _, iss := range ve.Issues {
		if iss.Path == "bio" && iss.Code == sk.CodeRequired {
			foundRequired = true
		}
	}
	if !foundRequired {
		t.Fatalf("expected required issue for bio, got %v", ve.Issues)
	}

	// Default applies on missing AND null (documented behavior).
	if _, err := schema.ParseString(`{"plan":null,"bio":null}`); err != nil {
		t.Fatalf("null plan should fall back to default: %v", err)
	}
}

func TestWriteValidationErrorWrappedErrors(t *testing.T) {
	type User struct {
		Name string `json:"name"`
	}
	schema := sk.Struct(func(s *sk.ObjectBuilder[User]) {
		sk.Field(s, &s.Target.Name, sk.String().Min(2))
	})
	_, err := schema.ParseString(`{"name":"a"}`)
	if err == nil {
		t.Fatal("expected validation error")
	}

	// Wrapped *ValidationError still maps to 422.
	rr := httptest.NewRecorder()
	sk.WriteValidationError(rr, fmt.Errorf("handler: %w", err))
	if rr.Code != 422 {
		t.Fatalf("wrapped ValidationError: got %d", rr.Code)
	}

	// Wrapped *HTTPError keeps its status.
	rr = httptest.NewRecorder()
	sk.WriteValidationError(rr, fmt.Errorf("ctx: %w", &sk.HTTPError{Status: 413, Code: "payload_too_large", Message: "big"}))
	if rr.Code != 413 {
		t.Fatalf("wrapped HTTPError: got %d", rr.Code)
	}
}
