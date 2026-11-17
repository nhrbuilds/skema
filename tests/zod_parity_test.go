package skema_test

import (
	"strings"
	"testing"
	"time"

	sk "github.com/nhrbuilds/skema"
)

func mustMap(t *testing.T, v interface {
	JSONSchemaMap(...sk.SchemaOption) (map[string]any, error)
}) map[string]any {
	t.Helper()
	m, err := v.JSONSchemaMap()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func issuesOf(t *testing.T, err error) []sk.Issue {
	t.Helper()
	ve, ok := err.(*sk.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	return ve.Issues
}

func TestUnionVariants(t *testing.T) {
	u := sk.Union(sk.Lift(sk.Int()), sk.Lift(sk.String().UUID()))

	if v, err := u.ParseString(`5`); err != nil || v.(int) != 5 {
		t.Fatalf("int variant: %v %v", v, err)
	}
	if v, err := u.ParseString(`"3d1f1e5c-7a2e-4a7b-9c1d-8e2f3a4b5c6d"`); err != nil || v.(string) != "3d1f1e5c-7a2e-4a7b-9c1d-8e2f3a4b5c6d" {
		t.Fatalf("uuid variant: %v %v", v, err)
	}

	// Failure collects every variant's issues at the union path.
	_, err := u.ParseString(`"neither"`)
	iss := issuesOf(t, err)
	if len(iss) != 2 {
		t.Fatalf("expected 2 union issues, got %d: %v", len(iss), iss)
	}

	// Homogeneous union of structs with the same target type.
	type filter struct {
		Eq string `json:"eq"`
		In string `json:"in"`
	}
	eq := sk.Struct(func(s *sk.ObjectBuilder[filter]) {
		sk.Field(s, &s.Target.Eq, sk.String())
		sk.Field(s, &s.Target.In, sk.OptionalValue(sk.String()))
	})
	in := sk.Struct(func(s *sk.ObjectBuilder[filter]) {
		sk.Field(s, &s.Target.Eq, sk.OptionalValue(sk.String()))
		sk.Field(s, &s.Target.In, sk.String().NonEmpty())
	})
	fu := sk.Union[filter](eq, in)
	if f, err := fu.ParseString(`{"in":"a,b"}`); err != nil || f.In != "a,b" {
		t.Fatalf("struct union: %+v %v", f, err)
	}
}

func TestUnionJSONSchema(t *testing.T) {
	m := mustMap(t, sk.Union(sk.Lift(sk.Int()), sk.Lift(sk.String())))
	anyOf, ok := m["anyOf"].([]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("missing anyOf: %v", m)
	}
}

func TestTuple(t *testing.T) {
	tp := sk.Tuple(sk.Lift(sk.Int()), sk.Lift(sk.String()))
	out, err := tp.ParseString(`[1,"x"]`)
	if err != nil || len(out) != 2 || out[0].(int) != 1 || out[1].(string) != "x" {
		t.Fatalf("tuple parse: %v %v", out, err)
	}
	if _, err := tp.ParseString(`[1]`); err == nil {
		t.Fatal("too few items must fail")
	}
	if _, err := tp.ParseString(`[1,"x",true]`); err == nil {
		t.Fatal("too many items must fail without Rest")
	}

	rp := sk.Tuple(sk.Lift(sk.Int())).Rest(sk.Lift(sk.String()))
	out, err = rp.ParseString(`[1,"a","b"]`)
	if err != nil || len(out) != 3 || out[2].(string) != "b" {
		t.Fatalf("rest tuple: %v %v", out, err)
	}

	m := mustMap(t, tp)
	pi, ok := m["prefixItems"].([]any)
	if !ok || len(pi) != 2 || m["items"] != false {
		t.Fatalf("tuple emission: %v", m)
	}
	m = mustMap(t, rp)
	if _, ok := m["items"].(map[string]any); !ok {
		t.Fatalf("rest emission missing items schema: %v", m)
	}
}

func TestAnyAndLift(t *testing.T) {
	if v, err := sk.Any().ParseString(`[1,"a",null]`); err != nil || v == nil {
		t.Fatalf("any: %v %v", v, err)
	}
	m := mustMap(t, sk.Any())
	if len(m) != 1 || m["$schema"] == nil {
		t.Fatalf("Any must emit only $schema: %v", m)
	}
	if _, err := sk.Lift(sk.Int()).ParseString(`7`); err != nil {
		t.Fatalf("lift: %v", err)
	}
}

func TestCatchAllStruct(t *testing.T) {
	type meta struct {
		Name string `json:"name"`
	}
	s := sk.Struct(func(b *sk.ObjectBuilder[meta]) {
		sk.Field(b, &b.Target.Name, sk.String())
	}).CatchAll(sk.Lift(sk.Int()))

	if u, err := s.ParseString(`{"name":"x","level":3}`); err != nil || u.Name != "x" {
		t.Fatalf("catchall ok: %+v %v", u, err)
	}
	_, err := s.ParseString(`{"name":"x","level":"no"}`)
	iss := issuesOf(t, err)
	if len(iss) != 1 || iss[0].Path != "level" {
		t.Fatalf("catchall failure: %v", iss)
	}

	// Strict mode rejects unknown keys outright; catchAll stays dormant.
	if _, err := s.Strict().ParseString(`{"name":"x","level":3}`); err == nil {
		t.Fatal("strict must reject unknown keys")
	}

	// Emission: additionalProperties becomes the catchAll schema.
	m := mustMap(t, s)
	ap, ok := m["additionalProperties"].(map[string]any)
	if !ok || ap["type"] != "integer" {
		t.Fatalf("catchall emission: %v", m)
	}
}

func TestRecordKeyValidation(t *testing.T) {
	m := sk.Map(sk.Int()).KeyPattern(`^[a-z_]+$`)
	if _, err := m.ParseString(`{"good":1}`); err != nil {
		t.Fatalf("valid key: %v", err)
	}
	_, err := m.ParseString(`{"BadKey":1}`)
	iss := issuesOf(t, err)
	if len(iss) != 1 || iss[0].Path != "BadKey" {
		t.Fatalf("bad key: %v", iss)
	}

	withKeys := sk.Map(sk.Int()).Keys(sk.String().Min(3))
	if _, err := withKeys.ParseString(`{"abc":1}`); err != nil {
		t.Fatalf("keys min ok: %v", err)
	}
	if _, err := withKeys.ParseString(`{"ab":1}`); err == nil {
		t.Fatal("short key must fail")
	}

	em := mustMap(t, m)
	pn, ok := em["propertyNames"].(map[string]any)
	if !ok || pn["pattern"] != `^[a-z_]+$` {
		t.Fatalf("propertyNames emission: %v", em)
	}
}

func TestTimeSchema(t *testing.T) {
	ts := sk.Time().After(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)).
		Before(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))

	got, err := ts.ParseString(`"2021-06-15T10:00:00Z"`)
	if err != nil || !got.Equal(time.Date(2021, 6, 15, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("time parse: %v %v", got, err)
	}
	if _, err := ts.ParseString(`"2019-01-01T00:00:00Z"`); err == nil {
		t.Fatal("before-after lower bound must fail")
	}
	if _, err := ts.ParseString(`"2031-01-01T00:00:00Z"`); err == nil {
		t.Fatal("before-after upper bound must fail")
	}
	if _, err := ts.ParseString(`"garbage"`); err == nil {
		t.Fatal("garbage must fail")
	}
	if _, err := ts.ParseString(`5`); err == nil {
		t.Fatal("non-string must fail")
	}

	type event struct {
		When time.Time `json:"when"`
	}
	es := sk.Struct(func(b *sk.ObjectBuilder[event]) {
		sk.Field(b, &b.Target.When, sk.Time())
	})
	if _, err := es.ParseString(`{"when":"2021-06-15T10:00:00Z"}`); err != nil {
		t.Fatalf("time field: %v", err)
	}

	em := mustMap(t, sk.Time())
	if em["type"] != "string" || em["format"] != "date-time" {
		t.Fatalf("time emission: %v", em)
	}
}

func TestNumericWidths(t *testing.T) {
	if _, err := sk.Int8().ParseAny(127); err != nil {
		t.Fatalf("int8 max: %v", err)
	}
	if _, err := sk.Int8().ParseAny(200); err == nil {
		t.Fatal("int8 overflow must fail")
	}
	if _, err := sk.Int32().ParseAny(2147483647); err != nil {
		t.Fatalf("int32 max: %v", err)
	}
	if _, err := sk.Int32().ParseAny(2147483648); err == nil {
		t.Fatal("int32 overflow must fail")
	}
	if _, err := sk.Uint8().ParseAny(255); err != nil {
		t.Fatalf("uint8 max: %v", err)
	}
	if _, err := sk.Uint8().ParseAny(-1); err == nil {
		t.Fatal("negative must fail for uint")
	}
	if v, err := sk.Uint64().ParseString("18446744073709551615"); err != nil || v != 18446744073709551615 {
		t.Fatalf("uint64 max: %v %v", v, err)
	}
	if _, err := sk.Uint64().ParseString("18446744073709551616"); err == nil {
		t.Fatal("uint64 overflow must fail")
	}
	if _, err := sk.Float32().ParseString("1e70"); err == nil {
		t.Fatal("float32 overflow must fail")
	}
	if v, err := sk.Int16().Coerce().ParseString(`"42"`); err != nil || v != 42 {
		t.Fatalf("int16 coerce: %v %v", v, err)
	}
	if _, err := sk.Int16().ParseString(`"42"`); err == nil {
		t.Fatal("int16 without coerce must reject strings")
	}
	if _, err := sk.Uint32().ParseString(`"-3"`); err == nil {
		t.Fatal("negative string must fail for uint32")
	}
	if _, err := sk.Float32().MultipleOf(0.5).ParseAny(2.5); err != nil {
		t.Fatalf("float32 multipleOf: %v", err)
	}

	// Fast-path parity for width-typed fields.
	type sized struct {
		A int32  `json:"a"`
		B uint16 `json:"b"`
	}
	ss := sk.Struct(func(b *sk.ObjectBuilder[sized]) {
		sk.Field(b, &b.Target.A, sk.Int32().Coerce())
		sk.Field(b, &b.Target.B, sk.Uint16().Coerce())
	})
	raw := `{"a":"-5","b":"65535"}`
	fast, err := ss.ParseString(raw)
	if err != nil {
		t.Fatalf("fast path: %v", err)
	}
	slow, err := ss.ParseAny(decodeTree(t, raw))
	if err != nil || fast != slow {
		t.Fatalf("parity: %+v vs %+v (%v)", fast, slow, err)
	}
}

func TestCatch(t *testing.T) {
	c := sk.Catch(sk.String().Min(5), "fallback")
	if v, err := c.ParseString(`"hi"`); err != nil || v != "fallback" {
		t.Fatalf("catch fallback: %q %v", v, err)
	}
	if v, err := c.ParseString(`"long enough"`); err != nil || v != "long enough" {
		t.Fatalf("catch pass-through: %q %v", v, err)
	}

	type cfg struct {
		Retries int `json:"retries"`
	}
	cs := sk.Struct(func(b *sk.ObjectBuilder[cfg]) {
		sk.Field(b, &b.Target.Retries, sk.Catch(sk.Int().Min(1), 3))
	})
	if u, err := cs.ParseString(`{"retries":0}`); err != nil || u.Retries != 3 {
		t.Fatalf("catch field: %+v %v", u, err)
	}
	if u, err := cs.ParseString(`{"retries":5}`); err != nil || u.Retries != 5 {
		t.Fatalf("catch field pass: %+v %v", u, err)
	}
}

func TestPreprocess(t *testing.T) {
	pp := sk.Preprocess(func(raw any) any {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
		return raw
	}, sk.String().NonEmpty())
	if v, err := pp.ParseString(`"  x  "`); err != nil || v != "x" {
		t.Fatalf("preprocess: %q %v", v, err)
	}
	if _, err := pp.ParseString(`"   "`); err == nil {
		t.Fatal("preprocessed empty must fail")
	}
}

func TestPipe(t *testing.T) {
	p := sk.Pipe(sk.String(), sk.Int().Coerce())
	if v, err := p.ParseString(`"25"`); err != nil || v != 25 {
		t.Fatalf("pipe str->int: %v %v", v, err)
	}
	if _, err := p.ParseString(`"abc"`); err == nil {
		t.Fatal("pipe must fail when second stage fails")
	}

	pt := sk.Pipe(sk.String(), sk.Time())
	got, err := pt.ParseString(`"2021-06-15T10:00:00Z"`)
	if err != nil || got.IsZero() {
		t.Fatalf("pipe str->time: %v %v", got, err)
	}
}

func TestMessages(t *testing.T) {
	s := sk.Messages(sk.String().Min(5), map[string]string{sk.CodeTooSmall: "custom too small"})
	_, err := s.ParseString(`"ab"`)
	iss := issuesOf(t, err)
	if len(iss) != 1 || iss[0].Message != "custom too small" || iss[0].Code != sk.CodeTooSmall {
		t.Fatalf("messages: %v", iss)
	}

	m := sk.Message(sk.String().Email(), "invalid email address")
	_, err = m.ParseString(`"nope"`)
	iss = issuesOf(t, err)
	if len(iss) != 1 || iss[0].Message != "invalid email address" {
		t.Fatalf("message: %v", iss)
	}

	type form struct {
		Email string `json:"email"`
	}
	fs := sk.Struct(func(b *sk.ObjectBuilder[form]) {
		sk.Field(b, &b.Target.Email, sk.Message(sk.String().Email(), "enter a valid email"))
	})
	_, err = fs.ParseString(`{"email":"nope"}`)
	iss = issuesOf(t, err)
	if len(iss) != 1 || iss[0].Message != "enter a valid email" || iss[0].Path != "email" {
		t.Fatalf("message field: %v", iss)
	}
}

func TestSuperRefine(t *testing.T) {
	type range_ struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	rs := sk.Struct(func(b *sk.ObjectBuilder[range_]) {
		sk.Field(b, &b.Target.Start, sk.String())
		sk.Field(b, &b.Target.End, sk.String())
	}).SuperRefine(func(r range_, ic *sk.IssueCollector) {
		if r.End < r.Start {
			ic.Add("end", sk.CodeCustom, "end before start")
			ic.Add("", sk.CodeCustom, "invalid range")
		}
	})
	_, err := rs.ParseString(`{"start":"b","end":"a"}`)
	iss := issuesOf(t, err)
	if len(iss) != 2 || iss[0].Path != "end" || iss[1].Path != "" {
		t.Fatalf("superRefine: %v", iss)
	}
	if _, err := rs.ParseString(`{"start":"a","end":"b"}`); err != nil {
		t.Fatalf("superRefine pass: %v", err)
	}
}

func TestPickOmitKeyOf(t *testing.T) {
	type user struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}
	full := sk.Struct(func(b *sk.ObjectBuilder[user]) {
		sk.Field(b, &b.Target.Name, sk.String())
		sk.Field(b, &b.Target.Email, sk.String().Email())
		sk.Field(b, &b.Target.Age, sk.Int())
	})

	picked := full.Pick([]string{"name", "age"})
	if _, err := picked.ParseString(`{"name":"a","age":1}`); err != nil {
		t.Fatalf("pick parse: %v", err)
	}
	pm := mustMap(t, picked)
	props := pm["properties"].(map[string]any)
	if _, ok := props["email"]; ok {
		t.Fatal("pick must drop email")
	}

	omitted := full.Omit([]string{"age"})
	if _, err := omitted.ParseString(`{"name":"a","email":"a@b.com"}`); err != nil {
		t.Fatalf("omit parse: %v", err)
	}
	om := mustMap(t, omitted)
	props = om["properties"].(map[string]any)
	if _, ok := props["age"]; ok {
		t.Fatal("omit must drop age")
	}

	// The original is untouched.
	if _, err := full.ParseString(`{"name":"a","email":"a@b.com","age":1}`); err != nil {
		t.Fatalf("original after pick/omit: %v", err)
	}

	ko := sk.KeyOf(full)
	if v, err := ko.ParseString(`"email"`); err != nil || v != "email" {
		t.Fatalf("keyof: %v %v", v, err)
	}
	if _, err := ko.ParseString(`"nope"`); err == nil {
		t.Fatal("keyof must reject unknown keys")
	}
}

func TestDeepPartial(t *testing.T) {
	type inner struct {
		Required string `json:"required"`
	}
	type mid struct {
		Inner inner  `json:"inner"`
		Name  string `json:"name"`
	}
	type outer struct {
		Mid mid    `json:"mid"`
		Top string `json:"top"`
	}
	innerSchema := sk.Struct(func(b *sk.ObjectBuilder[inner]) {
		sk.Field(b, &b.Target.Required, sk.String())
	})
	midSchema := sk.Struct(func(b *sk.ObjectBuilder[mid]) {
		sk.Field(b, &b.Target.Inner, innerSchema)
		sk.Field(b, &b.Target.Name, sk.String())
	})
	full := sk.Struct(func(b *sk.ObjectBuilder[outer]) {
		sk.Field(b, &b.Target.Mid, midSchema)
		sk.Field(b, &b.Target.Top, sk.String())
	})

	dp := full.DeepPartial()
	// Everything optional at every depth, via the fast path.
	if _, err := dp.ParseBytes([]byte(`{}`)); err != nil {
		t.Fatalf("deep partial empty: %v", err)
	}
	if _, err := dp.ParseBytes([]byte(`{"mid":{"inner":{}}}`)); err != nil {
		t.Fatalf("deep partial nested: %v", err)
	}
	if _, err := dp.ParseString(`{"top":"x"}`); err != nil {
		t.Fatalf("deep partial top: %v", err)
	}
	// The original stays strict about required fields.
	if _, err := full.ParseString(`{"mid":{"inner":{}}}`); err == nil {
		t.Fatal("original must still require fields")
	}
	// Emission: no required anywhere, and original unchanged.
	dm := mustMap(t, dp)
	if _, ok := dm["required"]; ok {
		t.Fatalf("deep partial must have no required: %v", dm)
	}
	if _, ok := mustMap(t, full)["required"]; !ok {
		t.Fatal("original must keep required")
	}
}

func TestRecursiveDiscriminatedUnion(t *testing.T) {
	type duNode struct {
		Kind  string  `json:"kind"`
		Value string  `json:"value"`
		Child *duNode `json:"child,omitempty"`
	}
	var nodeSchema sk.Validator[duNode]
	leaf := sk.Struct(func(b *sk.ObjectBuilder[duNode]) {
		sk.Field(b, &b.Target.Kind, sk.Literal("leaf"))
		sk.Field(b, &b.Target.Value, sk.String())
	})
	branch := sk.Lazy("Branch", func() sk.Validator[duNode] {
		return sk.Struct(func(b *sk.ObjectBuilder[duNode]) {
			sk.Field(b, &b.Target.Kind, sk.Literal("branch"))
			sk.Field(b, &b.Target.Child, sk.OptionalNull(nodeSchema))
		})
	})
	nodeSchema = sk.DiscriminatedUnion[duNode]("kind",
		sk.UnionVariant[duNode]{Value: "leaf", Schema: leaf},
		sk.UnionVariant[duNode]{Value: "branch", Schema: branch},
	)

	deep := `{"kind":"branch","child":{"kind":"branch","child":{"kind":"leaf","value":"end"}}}`
	n, err := nodeSchema.ParseString(deep)
	if err != nil || n.Kind != "branch" || n.Child == nil {
		t.Fatalf("recursive union parse: %+v %v", n, err)
	}

	m := mustMap(t, nodeSchema)
	if _, ok := m["oneOf"]; !ok {
		t.Fatalf("missing oneOf: %v", m)
	}
	if _, ok := m["discriminator"]; !ok {
		t.Fatalf("missing discriminator: %v", m)
	}
	defs := defsOf(t, m)
	if _, ok := defs["Branch"]; !ok {
		t.Fatalf("$defs missing Branch:\n%s", schemaJSON(t, m))
	}
}
