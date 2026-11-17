package skema_test

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sk "github.com/nhrbuilds/skema"
)

func TestCustom(t *testing.T) {
	even := sk.Custom(func(raw any, report func(path, code, msg string)) (int, error) {
		n, ok := raw.(int)
		if !ok {
			report("", "custom", "want integer")
			return 0, nil
		}
		if n%2 != 0 {
			report("", "custom", "want even number")
			return 0, nil
		}
		return n, nil
	})
	if v, err := even.ParseAny(4); err != nil || v != 4 {
		t.Fatalf("custom ok: %v %v", v, err)
	}
	if _, err := even.ParseAny(3); err == nil {
		t.Fatal("custom report must fail")
	}
	if _, err := even.ParseAny("x"); err == nil {
		t.Fatal("custom type report must fail")
	}
}

func TestIntersect(t *testing.T) {
	both := sk.Intersect(
		sk.Lift(sk.String().Min(3)),
		sk.Lift(sk.String().Max(5)),
	)
	if v, err := both.ParseString(`"abcd"`); err != nil || v.(string) != "abcd" {
		t.Fatalf("intersect ok: %v %v", v, err)
	}
	_, failErr := both.ParseString(`"ab"`)
	if failErr == nil {
		t.Fatal("intersect must fail on too-small")
	}
	iss := issuesOf(t, failErr)
	if len(iss) == 0 || iss[0].Code != sk.CodeTooSmall {
		t.Fatalf("intersect too small: %v", iss)
	}
	m := mustMap(t, both)
	if _, ok := m["allOf"].([]any); !ok {
		t.Fatalf("missing allOf: %v", m)
	}
}

func TestErrorShaping(t *testing.T) {
	type form struct {
		Email string `json:"email"`
		Age   int    `json:"age"`
	}
	s := sk.Struct(func(b *sk.ObjectBuilder[form]) {
		sk.Field(b, &b.Target.Email, sk.String().Email())
		sk.Field(b, &b.Target.Age, sk.Int().Min(18))
	})
	_, err := s.ParseString(`{"email":"nope","age":5}`)
	ve := err.(*sk.ValidationError)

	flat := ve.Flatten()
	if msgs := flat["email"]; len(msgs) != 1 {
		t.Fatalf("flatten email: %v", flat)
	}
	if msgs := flat["age"]; len(msgs) != 1 {
		t.Fatalf("flatten age: %v", flat)
	}
	if _, ok := flat[""]; ok {
		t.Fatalf("no root issues expected: %v", flat)
	}

	pre := ve.Prettify()
	if !strings.Contains(pre, "email: ") || !strings.Contains(pre, "age: ") || !strings.Contains(pre, "\n") {
		t.Fatalf("prettify: %q", pre)
	}
}

func TestParseEnv(t *testing.T) {
	type cfg struct {
		Port  int     `json:"PORT"`
		Debug bool    `json:"DEBUG"`
		Host  *string `json:"HOST"`
	}
	s := sk.Struct(func(b *sk.ObjectBuilder[cfg]) {
		sk.Field(b, &b.Target.Port, sk.Int().Coerce())
		sk.Field(b, &b.Target.Debug, sk.Bool().Coerce())
		sk.Field(b, &b.Target.Host, sk.String().OptionalNull())
	})
	c, err := sk.ParseEnv(s, []string{"PORT=8080", "DEBUG=true", "PATH=/bin"})
	if err != nil || c.Port != 8080 || !c.Debug || c.Host != nil {
		t.Fatalf("parse env: %+v %v", c, err)
	}
	if _, err := sk.ParseEnv(s, []string{"PORT=abc", "DEBUG=true"}); err == nil {
		t.Fatal("bad env must fail")
	}

	m, err := sk.ParseEnvMap(s, map[string]string{"PORT": "9090", "DEBUG": "1"})
	if err != nil || m.Port != 9090 || !m.Debug {
		t.Fatalf("parse env map: %+v %v", m, err)
	}
}

func TestDecodeFormAndQuery(t *testing.T) {
	type q struct {
		Page int    `json:"page"`
		Tag  string `json:"tag"`
	}
	s := sk.Struct(func(b *sk.ObjectBuilder[q]) {
		sk.Field(b, &b.Target.Page, sk.Int().Coerce().Min(1))
		sk.Field(b, &b.Target.Tag, sk.String().NonEmpty())
	})

	req := httptest.NewRequest("POST", "/search", strings.NewReader("page=3&tag=go"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	got, err := sk.DecodeForm(req, s)
	if err != nil || got.Page != 3 || got.Tag != "go" {
		t.Fatalf("decode form: %+v %v", got, err)
	}

	bad := httptest.NewRequest("POST", "/search", strings.NewReader("page=0&tag=go"))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := sk.DecodeForm(bad, s); err == nil {
		t.Fatal("bad form must fail")
	}

	greq := httptest.NewRequest("GET", "/search?page=2&tag=x", nil)
	got, err = sk.DecodeQuery(greq, s)
	if err != nil || got.Page != 2 || got.Tag != "x" {
		t.Fatalf("decode query: %+v %v", got, err)
	}
}

func TestStringFormatsV11(t *testing.T) {
	v4 := "3d1f1e5c-7a2e-4a7b-9c1d-8e2f3a4b5c6d" // version 4
	v7 := "0190b4a0-5e1d-7b3a-9c2d-3e4f5a6b7c8d" // version 7
	if _, err := sk.String().UUIDv4().ParseString(`"` + v4 + `"`); err != nil {
		t.Fatalf("uuidv4 ok: %v", err)
	}
	if _, err := sk.String().UUIDv4().ParseString(`"` + v7 + `"`); err == nil {
		t.Fatal("uuidv4 must reject v7")
	}
	if _, err := sk.String().UUIDv7().ParseString(`"` + v7 + `"`); err != nil {
		t.Fatalf("uuidv7 ok: %v", err)
	}

	if _, err := sk.String().Base64().ParseString(`"aGVsbG8="`); err != nil {
		t.Fatalf("base64 ok: %v", err)
	}
	if _, err := sk.String().Base64().ParseString(`"not base64!!"`); err == nil {
		t.Fatal("bad base64 must fail")
	}

	if _, err := sk.String().CIDR().ParseString(`"10.0.0.0/8"`); err != nil {
		t.Fatalf("cidr ok: %v", err)
	}
	if _, err := sk.String().CIDR().ParseString(`"10.0.0.0"`); err == nil {
		t.Fatal("bare ip must fail cidr")
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"1"}`))
	jwt := header + "." + payload + "." + base64.RawURLEncoding.EncodeToString([]byte("sig"))
	if _, err := sk.String().JWT().ParseString(`"` + jwt + `"`); err != nil {
		t.Fatalf("jwt ok: %v", err)
	}
	if _, err := sk.String().JWT().ParseString(`"not.a-jwt"`); err == nil {
		t.Fatal("bad jwt must fail")
	}
}

func TestBigInt(t *testing.T) {
	const huge = "123456789012345678901234567890"
	v, err := sk.BigInt().ParseString(huge)
	if err != nil {
		t.Fatalf("bigint: %v", err)
	}
	if v.String() != huge {
		t.Fatalf("bigint value: %s", v.String())
	}
	if _, err := sk.BigInt().ParseString(`"123"`); err == nil {
		t.Fatal("string needs coerce")
	}
	if v, err := sk.BigInt().Coerce().ParseString(`"98765432109876543210"`); err != nil || v.String() != "98765432109876543210" {
		t.Fatalf("bigint coerce: %v %v", v, err)
	}
	if _, err := sk.BigInt().ParseString("1.5"); err == nil {
		t.Fatal("fraction must fail")
	}

	type wallet struct {
		Balance big.Int `json:"balance"`
	}
	ws := sk.Struct(func(b *sk.ObjectBuilder[wallet]) {
		sk.Field(b, &b.Target.Balance, sk.BigInt())
	})
	w, err := ws.ParseString(`{"balance":-99999999999999999999999999}`)
	if err != nil || w.Balance.Sign() >= 0 {
		t.Fatalf("bigint field: %v %v", w.Balance.String(), err)
	}
}

func TestTitle(t *testing.T) {
	m := mustMap(t, sk.Title(sk.Int().Min(1), "Age"))
	if m["title"] != "Age" {
		t.Fatalf("title: %v", m)
	}
}

func TestObject(t *testing.T) {
	o := sk.Object(map[string]sk.Validator[any]{
		"name":  sk.Lift(sk.String().NonEmpty()),
		"level": sk.Lift(sk.Int()),
	}, "name")

	out, err := o.ParseString(`{"name":"x","level":2,"extra":true}`)
	if err != nil {
		t.Fatalf("object parse: %v", err)
	}
	if out["name"] != "x" || out["level"].((int)) != 2 {
		t.Fatalf("object fields: %v", out)
	}
	if out["extra"] != true {
		t.Fatal("unknown keys must pass through")
	}

	if _, err := o.ParseString(`{"level":2}`); err == nil {
		t.Fatal("missing required must fail")
	}
	if _, err := o.Strict().ParseString(`{"name":"x","extra":1}`); err == nil {
		t.Fatal("strict must reject unknown")
	}

	ca := o.CatchAll(sk.Lift(sk.Int()))
	out, err = ca.ParseString(`{"name":"x","count":7}`)
	if err != nil || out["count"] != 7 {
		t.Fatalf("catchall: %v %v", out, err)
	}
	if _, err := ca.ParseString(`{"name":"x","count":"no"}`); err == nil {
		t.Fatal("catchall must validate unknown keys")
	}

	mk := sk.Object(nil).MinKeys(1).MaxKeys(2)
	if _, err := mk.ParseString(`{}`); err == nil {
		t.Fatal("minKeys must fail on empty")
	}

	m := mustMap(t, o)
	if _, ok := m["properties"]; !ok {
		t.Fatal("object emission missing properties")
	}
}

func TestFromJSONSchemaImport(t *testing.T) {
	doc := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "minLength": float64(2)},
			"age":  map[string]any{"type": "integer", "minimum": float64(18), "maximum": float64(120)},
			"plan": map[string]any{"enum": []any{"free", "pro"}, "default": "free"},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": float64(3)},
			"meta": map[string]any{"type": []any{"string", "null"}},
		},
		"required":             []any{"name", "age"},
		"additionalProperties": false,
	}
	v, err := sk.FromJSONSchema(doc)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	out, err := v.ParseString(`{"name":"abe","age":30,"plan":null,"tags":["a"]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "abe" {
		t.Fatalf("name: %v", m["name"])
	}
	if n, ok := m["age"].(int64); !ok || n != 30 {
		t.Fatalf("age: %v", m["age"])
	}
	if _, err := v.ParseString(`{"name":"a","age":30}`); err == nil {
		t.Fatal("minLength must be enforced")
	}
	if _, err := v.ParseString(`{"name":"abe","age":17}`); err == nil {
		t.Fatal("minimum must be enforced")
	}
	if _, err := v.ParseString(`{"name":"abe","age":30,"unknown":1}`); err == nil {
		t.Fatal("additionalProperties false must reject")
	}

	// refs + defs with recursion
	recursive := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value":    map[string]any{"type": "integer"},
			"children": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/Node"}},
		},
		"$defs": map[string]any{
			"Node": map[string]any{"$ref": "#"},
		},
	}
	// "#/$defs/Node" refers to itself through "#": compile must not hang.
	rv, err := sk.FromJSONSchema(recursive)
	if err != nil {
		t.Fatalf("recursive import: %v", err)
	}
	if _, err := rv.ParseString(`{"value":1,"children":[{"value":2}]}`); err != nil {
		t.Fatalf("recursive parse: %v", err)
	}

	// fail-closed on unsupported keywords
	if _, err := sk.FromJSONSchema(map[string]any{"not": map[string]any{}}); err == nil {
		t.Fatal("unsupported keyword must fail")
	}
}

func TestJSONSchemaRoundTrip(t *testing.T) {
	m, err := SignupSchema.JSONSchemaMap()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := sk.FromJSONSchemaBytes(doc)
	if err != nil {
		t.Fatalf("round-trip import: %v", err)
	}
	raw := `{"email":"user@example.com","age":25,"plan":"pro","tags":["go"],"address":{"city":"Dhaka","country":"BD"}}`
	out, err := imported.ParseString(raw)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	o := out.(map[string]any)
	if o["email"] != "user@example.com" {
		t.Fatalf("email: %v", o["email"])
	}
	if o["plan"] != "pro" {
		t.Fatalf("plan: %v", o["plan"])
	}
	addr := o["address"].(map[string]any)
	if addr["city"] != "Dhaka" {
		t.Fatalf("nested: %v", addr)
	}
	if _, err := imported.ParseString(`{"email":"nope","age":25,"address":{"city":"x","country":"US"}}`); err == nil {
		t.Fatal("round-trip must enforce constraints")
	}
}

type taggedAddress struct {
	City    string `json:"city" sk:"required,trim,nonempty,min=1"`
	Country string `json:"country" sk:"required,upper,len=2"`
}

type taggedSignup struct {
	Email    string        `json:"email" sk:"required,trim,lower,email"`
	Age      int           `json:"age" sk:"required,coerce,min=18,max=120"`
	Plan     string        `json:"plan" sk:"enum=free|pro|enterprise,default=free"`
	Nickname *string       `json:"nickname" sk:"optional,min=2,max=30"`
	Bio      *string       `json:"bio" sk:"nullable,min=1"`
	Comment  *string       `json:"comment" sk:"optional,nullable,min=1"`
	Tags     []string      `json:"tags" sk:"optional,unique,max=5"`
	Count    uint8         `json:"count" sk:"coerce,max=200"`
	When     time.Time     `json:"when" sk:"required"`
	Address  taggedAddress `json:"address"`
}

func init() {
	sk.RegisterTagSchema(sk.FromStruct[taggedAddress]())
}

func TestFromStruct(t *testing.T) {
	s := sk.FromStruct[taggedSignup]()

	raw := `{"email":" USER@Example.COM ","age":"30","plan":"pro","nickname":"nhr","bio":"go dev","tags":["a","b"],"count":"42","when":"2026-01-02T03:04:05Z","address":{"city":"Dhaka","country":"bd"}}`
	u, err := s.ParseString(raw)
	if err != nil {
		t.Fatalf("fromstruct parse: %v", err)
	}
	if u.Email != "user@example.com" || u.Age != 30 || u.Plan != "pro" {
		t.Fatalf("fields: %+v", u)
	}
	if u.Nickname == nil || *u.Nickname != "nhr" {
		t.Fatalf("nickname: %v", u.Nickname)
	}
	if u.Count != 42 {
		t.Fatalf("count: %d", u.Count)
	}
	if u.When.Year() != 2026 {
		t.Fatalf("when: %v", u.When)
	}
	if u.Address.City != "Dhaka" || u.Address.Country != "BD" {
		t.Fatalf("nested: %+v", u.Address)
	}

	// plan default applies when missing
	u2, err := s.ParseString(`{"email":"a@b.com","age":30,"count":0,"bio":null,"when":"2026-01-02T03:04:05Z","address":{"city":"x","country":"US"}}`)
	if err != nil || u2.Plan != "free" {
		t.Fatalf("default: %+v %v", u2, err)
	}

	// optional pointer: null rejected; nullable pointer: null accepted
	if _, err := s.ParseString(`{"email":"a@b.com","age":30,"nickname":null,"when":"2026-01-02T03:04:05Z","address":{"city":"x","country":"US"}}`); err == nil {
		t.Fatal("optional pointer must reject null")
	}
	u3, err := s.ParseString(`{"email":"a@b.com","age":30,"count":0,"bio":null,"when":"2026-01-02T03:04:05Z","address":{"city":"x","country":"US"}}`)
	if err != nil || u3.Bio != nil {
		t.Fatalf("nullable pointer: %+v %v", u3, err)
	}

	// width bounds
	if _, err := s.ParseString(`{"email":"a@b.com","age":30,"count":201,"when":"2026-01-02T03:04:05Z","address":{"city":"x","country":"US"}}`); err == nil {
		t.Fatal("uint8 max must be enforced")
	}

	// unknown tag tokens panic at build time
	defer func() {
		if recover() == nil {
			t.Fatal("unknown tag token must panic")
		}
	}()
	type badTag struct {
		Name string `json:"name" sk:"wat=1"`
	}
	_ = sk.FromStruct[badTag]()
}

func TestFromJSONSchemaBytesTrailing(t *testing.T) {
	// guards that FromJSONSchemaBytes rejects trailing JSON
	if _, err := sk.FromJSONSchemaBytes([]byte(`{"type":"string"} {"type":"number"}`)); err == nil {
		t.Fatal("trailing json must fail")
	}
}
