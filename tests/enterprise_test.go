package skema_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sk "github.com/nhrbuilds/skema"
)

func TestHTTP413(t *testing.T) {
	schema := sk.String()
	payload := []byte(`"` + string(bytes.Repeat([]byte("a"), 4096)) + `"`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	_, err := sk.DecodeHTTP(req, schema, sk.WithMaxBodyBytes(1024))
	if err == nil {
		t.Fatal("expected 413")
	}
	he, ok := err.(*sk.HTTPError)
	if !ok || he.Status != 413 {
		t.Fatalf("%v", err)
	}
}

func TestHTTP415(t *testing.T) {
	schema := sk.String()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`"x"`))
	req.Header.Set("Content-Type", "text/plain")
	_, err := sk.DecodeHTTP(req, schema)
	if err == nil {
		t.Fatal("expected 415")
	}
}

func TestIntOverflow(t *testing.T) {
	s := sk.Int()
	_, err := s.ParseString("9223372036854775808")
	if err == nil {
		t.Fatal("expected range error")
	}
}

func TestLiteralJSONNumber(t *testing.T) {
	s := sk.Literal(1)
	v, err := s.ParseAny(float64(1))
	if err != nil || any(v) != 1 {
		t.Fatalf("%v %v", v, err)
	}
}

func TestLazyConcurrent(t *testing.T) {
	type Node struct {
		Value    int    `json:"value"`
		Children []Node `json:"children,omitempty"`
	}
	var recursive sk.Validator[Node]
	recursive = sk.Lazy[Node]("Node", func() sk.Validator[Node] {
		return sk.Struct(func(s *sk.ObjectBuilder[Node]) {
			sk.Field(s, &s.Target.Value, sk.Int())
			sk.Field(s, &s.Target.Children, sk.Slice(recursive).Optional())
		})
	})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = recursive.ParseString(`{"value":1}`)
		}()
	}
	wg.Wait()
	m, err := recursive.JSONSchemaMap()
	if err != nil || m["$defs"] == nil {
		t.Fatalf("missing defs: %v %v", m, err)
	}
}

func TestFastParseBytesMatchesParseAny(t *testing.T) {
	raw := `{"email":"user@example.com","age":25,"plan":"pro","tags":["go"],"address":{"city":"Dhaka","country":"BD"}}`
	a, err := SignupSchema.ParseBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	b, err := SignupSchema.ParseAny(tree)
	if err != nil {
		t.Fatal(err)
	}
	if a.Email != b.Email || a.Age != b.Age || a.Plan != b.Plan || a.Address.City != b.Address.City {
		t.Fatalf("mismatch %+v %+v", a, b)
	}
}

func TestPartialPATCH(t *testing.T) {
	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	full := sk.Struct(func(s *sk.ObjectBuilder[User]) {
		sk.Field(s, &s.Target.Name, sk.String().NonEmpty())
		sk.Field(s, &s.Target.Age, sk.Int().Min(0))
	})
	patch := full.Partial()
	u, err := patch.ParseString(`{"name":"x"}`)
	if err != nil || u.Name != "x" {
		t.Fatalf("%+v %v", u, err)
	}
}
