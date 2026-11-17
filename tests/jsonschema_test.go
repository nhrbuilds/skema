package skema_test

import (
	"strings"
	"testing"

	sk "github.com/nhrbuilds/skema"
)

func TestJSONSchemaDraft(t *testing.T) {
	s := sk.Int().Min(1).Max(10)
	m, err := s.JSONSchemaMap(sk.WithDraft202012(), sk.WithTitle("Age"))
	if err != nil {
		t.Fatal(err)
	}
	if m["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("%v", m)
	}
}

func TestLLMStrictMode(t *testing.T) {
	type Item struct {
		Name string `json:"name"`
	}
	schema := sk.Struct(func(s *sk.ObjectBuilder[Item]) {
		sk.Field(s, &s.Target.Name, sk.OptionalValue(sk.String()))
	})
	m, err := schema.JSONSchemaMap(sk.WithLLMStrictMode())
	if err != nil {
		t.Fatal(err)
	}
	req, ok := m["required"].([]string)
	if !ok || len(req) != 1 {
		t.Fatalf("%v", m["required"])
	}
	ap, ok := m["additionalProperties"].(bool)
	if !ok || ap {
		t.Fatalf("%v", m["additionalProperties"])
	}
	props := m["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if desc, ok := name["description"].(string); ok && strings.Contains(desc, "null") {
		t.Fatalf("unexpected %s", desc)
	}
	if name["type"] == nil {
		t.Fatalf("%v", name)
	}
}
