package skema_test

import (
	"testing"

	sk "github.com/nhrbuilds/skema"
)

type Address struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type SignupInput struct {
	Email    string   `json:"email"`
	Age      int      `json:"age"`
	Plan     string   `json:"plan"`
	Tags     []string `json:"tags,omitempty"`
	Nickname *string  `json:"nickname,omitempty"`
	Address  Address  `json:"address"`
}

var AddressSchema = sk.Struct(func(s *sk.ObjectBuilder[Address]) {
	sk.Field(s, &s.Target.City, sk.String().Trim().NonEmpty())
	sk.Field(s, &s.Target.Country, sk.String().Trim().Upper().Len(2))
})

var SignupSchema = sk.Struct(func(s *sk.ObjectBuilder[SignupInput]) {
	sk.Field(s, &s.Target.Email, sk.String().Trim().Lower().Email())
	sk.Field(s, &s.Target.Age, sk.Int().Coerce().Min(18).Max(120))
	sk.Field(s, &s.Target.Plan, sk.Enum("free", "pro", "enterprise").Default("free"))
	sk.Field(s, &s.Target.Tags, sk.Slice(sk.String().Min(2)).Max(5).Optional())
	sk.Field(s, &s.Target.Nickname, sk.String().Min(2).Max(30).OptionalNull())
	sk.Field(s, &s.Target.Address, AddressSchema)
}).Strict().Refine(func(in SignupInput) bool {
	return !(in.Plan == "enterprise" && in.Age < 21)
}, "enterprise plan requires age >= 21")

func TestSignupValid(t *testing.T) {
	raw := `{
		"email":"user@example.com",
		"age":25,
		"plan":"pro",
		"tags":["go","rust"],
		"nickname":"nhr",
		"address":{"city":"Dhaka","country":"BD"}
	}`
	in, err := SignupSchema.ParseString(raw)
	if err != nil {
		t.Fatal(err)
	}
	if in.Email != "user@example.com" || in.Plan != "pro" {
		t.Fatalf("%+v", in)
	}
}

func TestSignupStrictUnknownKey(t *testing.T) {
	raw := `{"email":"u@e.com","age":22,"plan":"free","address":{"city":"x","country":"US"},"extra":1}`
	_, err := SignupSchema.ParseString(raw)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSignupRefineEnterpriseAge(t *testing.T) {
	raw := `{"email":"u@e.com","age":20,"plan":"enterprise","address":{"city":"x","country":"US"}}`
	_, err := SignupSchema.ParseString(raw)
	if err == nil {
		t.Fatal("expected refine failure")
	}
}
