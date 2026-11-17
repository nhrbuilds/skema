package skema_test

import (
	"testing"

	sk "github.com/nhrbuilds/skema"
)

type Event struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func TestDiscriminatedUnion(t *testing.T) {
	click := sk.Struct(func(s *sk.ObjectBuilder[Event]) {
		sk.Field(s, &s.Target.Type, sk.Literal("click"))
		sk.Field(s, &s.Target.Name, sk.String().NonEmpty())
	})
	view := sk.Struct(func(s *sk.ObjectBuilder[Event]) {
		sk.Field(s, &s.Target.Type, sk.Literal("view"))
		sk.Field(s, &s.Target.Name, sk.String())
	})
	u := sk.DiscriminatedUnion[Event]("type",
		sk.UnionVariant[Event]{Value: "click", Schema: click},
		sk.UnionVariant[Event]{Value: "view", Schema: view},
	)
	_, err := u.ParseString(`{"type":"click","name":""}`)
	if err == nil {
		t.Fatal("expected name error")
	}
	ev, err := u.ParseString(`{"type":"view","name":"home"}`)
	if err != nil || ev.Type != "view" {
		t.Fatalf("%+v %v", ev, err)
	}
}
