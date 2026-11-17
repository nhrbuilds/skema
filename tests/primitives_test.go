package skema_test

import (
	"testing"

	sk "github.com/nhrbuilds/skema"
)

func TestStringEmailTrim(t *testing.T) {
	s := sk.String().Trim().Lower().Email()
	out, err := s.ParseString(`"  Me@Example.COM  "`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "me@example.com" {
		t.Fatalf("got %q", out)
	}
}

func TestIntCoerceRejectFloat(t *testing.T) {
	s := sk.Int()
	_, err := s.ParseAny(3.5)
	if err == nil {
		t.Fatal("expected error")
	}
	ve := err.(*sk.ValidationError)
	if len(ve.Issues) == 0 {
		t.Fatal("expected issues")
	}
}

func TestBoolCoerce(t *testing.T) {
	s := sk.Bool().Coerce()
	v, err := s.ParseAny("1")
	if err != nil || !v {
		t.Fatalf("got %v err %v", v, err)
	}
}

func TestEnum(t *testing.T) {
	s := sk.Enum("a", "b")
	_, err := s.ParseAny("c")
	if err == nil {
		t.Fatal("expected error")
	}
}
