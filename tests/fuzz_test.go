package skema_test

import (
	"testing"

	sk "github.com/nhrbuilds/skema"
)

func FuzzDecodeJSONTrailing(f *testing.F) {
	f.Add([]byte(`{"name":"x"}`))
	f.Add([]byte(`{"name":"x"} `))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		schema := sk.Struct(func(s *sk.ObjectBuilder[fuzzUser]) {
			sk.Field(s, &s.Target.Name, sk.String())
		})
		_, _ = schema.ParseBytes(data)
	})
}

type fuzzUser struct {
	Name string `json:"name"`
}
